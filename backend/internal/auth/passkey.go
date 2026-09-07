package auth

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"
	"github.com/ydfk/measure-trail/backend/internal/config"
	"gorm.io/gorm"
)

const (
	passkeySessionLifetime = 5 * time.Minute
	passkeyMaxSessions     = 1024
)

type PasskeyCredential struct {
	ID         string
	UserID     string
	Name       string
	CreatedAt  time.Time
	UpdatedAt  time.Time
	LastUsedAt *time.Time
}

type passkeyService struct {
	db       *gorm.DB
	auth     *Service
	webAuthn *webauthn.WebAuthn
	aead     cipher.AEAD
	mu       sync.Mutex
	sessions map[string]passkeySession
}

type passkeySession struct {
	kind      string
	userID    string
	name      string
	data      webauthn.SessionData
	expiresAt time.Time
}

type passkeyUser struct {
	id          string
	username    string
	credentials []webauthn.Credential
}

func newPasskeyService(db *gorm.DB, authService *Service, settings config.Passkey) (*passkeyService, error) {
	if db == nil || authService == nil {
		return nil, fmt.Errorf("Passkey 依赖尚未初始化")
	}
	key, err := decodePasskeyEncryptionKey(settings.CredentialEncryptionKey)
	if err != nil || len(key) != 32 {
		return nil, fmt.Errorf("Passkey 凭据加密密钥未配置")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	webAuthn, err := webauthn.New(&webauthn.Config{
		RPID:          settings.RPID,
		RPDisplayName: settings.RPName,
		RPOrigins:     settings.Origins,
		AuthenticatorSelection: protocol.AuthenticatorSelection{
			ResidentKey:      protocol.ResidentKeyRequirementRequired,
			UserVerification: protocol.VerificationRequired,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("Passkey RP 配置无效: %w", err)
	}
	return &passkeyService{db: db, auth: authService, webAuthn: webAuthn, aead: aead, sessions: map[string]passkeySession{}}, nil
}

func decodePasskeyEncryptionKey(value string) ([]byte, error) {
	key, err := base64.RawStdEncoding.DecodeString(value)
	if err == nil {
		return key, nil
	}
	return base64.StdEncoding.DecodeString(value)
}

func (user *passkeyUser) WebAuthnID() []byte                         { return []byte(user.id) }
func (user *passkeyUser) WebAuthnName() string                       { return user.username }
func (user *passkeyUser) WebAuthnDisplayName() string                { return user.username }
func (user *passkeyUser) WebAuthnCredentials() []webauthn.Credential { return user.credentials }

func (service *passkeyService) beginRegistration(ctx context.Context, userID, name string) (*protocol.CredentialCreation, string, error) {
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > 128 {
		return nil, "", fmt.Errorf("Passkey 名称必须为 1 至 128 个字符")
	}
	user, err := service.loadUser(ctx, userID)
	if err != nil {
		return nil, "", err
	}
	creation, session, err := service.webAuthn.BeginRegistration(user,
		webauthn.WithResidentKeyRequirement(protocol.ResidentKeyRequirementRequired),
		webauthn.WithExclusions(webauthn.Credentials(user.credentials).CredentialDescriptors()),
	)
	if err != nil {
		return nil, "", fmt.Errorf("创建 Passkey 注册挑战失败: %w", err)
	}
	sessionID, err := service.storeSession(passkeySession{kind: "registration", userID: userID, name: name, data: *session})
	return creation, sessionID, err
}

func (service *passkeyService) finishRegistration(ctx context.Context, userID, sessionID string, response []byte) (PasskeyCredential, error) {
	session, err := service.takeSession(sessionID, "registration", userID)
	if err != nil {
		return PasskeyCredential{}, err
	}
	user, err := service.loadUser(ctx, userID)
	if err != nil {
		return PasskeyCredential{}, err
	}
	parsed, err := protocol.ParseCredentialCreationResponseBytes(response)
	if err != nil {
		return PasskeyCredential{}, fmt.Errorf("解析 Passkey 注册响应失败: %w", err)
	}
	credential, err := service.webAuthn.CreateCredential(user, session.data, parsed)
	if err != nil {
		return PasskeyCredential{}, fmt.Errorf("验证 Passkey 注册响应失败: %w", err)
	}
	return service.saveCredential(ctx, userID, session.name, credential)
}

func (service *passkeyService) beginLogin(ctx context.Context) (*protocol.CredentialAssertion, string, error) {
	var count int64
	if err := service.db.WithContext(ctx).Raw("SELECT COUNT(*) FROM passkey_credentials").Scan(&count).Error; err != nil {
		return nil, "", err
	}
	if count == 0 {
		return nil, "", fmt.Errorf("尚未登记 Passkey，请先使用密码登录并添加")
	}
	assertion, session, err := service.webAuthn.BeginDiscoverableLogin(webauthn.WithUserVerification(protocol.VerificationRequired))
	if err != nil {
		return nil, "", fmt.Errorf("创建 Passkey 登录挑战失败: %w", err)
	}
	sessionID, err := service.storeSession(passkeySession{kind: "login", data: *session})
	return assertion, sessionID, err
}

func (service *passkeyService) finishLogin(ctx context.Context, sessionID string, response []byte, deviceLabel string) (Session, error) {
	session, err := service.takeSession(sessionID, "login", "")
	if err != nil {
		return Session{}, err
	}
	parsed, err := protocol.ParseCredentialRequestResponseBytes(response)
	if err != nil {
		return Session{}, fmt.Errorf("解析 Passkey 登录响应失败: %w", err)
	}
	validatedUser, credential, err := service.webAuthn.ValidatePasskeyLogin(func(rawID, userHandle []byte) (webauthn.User, error) {
		return service.loadDiscoverableUser(ctx, rawID, userHandle)
	}, session.data, parsed)
	if err != nil {
		return Session{}, fmt.Errorf("Passkey 验证失败: %w", err)
	}
	user, ok := validatedUser.(*passkeyUser)
	if !ok {
		return Session{}, fmt.Errorf("Passkey 用户类型无效")
	}
	if err := service.updateCredential(ctx, user.id, credential); err != nil {
		return Session{}, err
	}
	return service.auth.issueSession(user.id, deviceLabel)
}

func (service *passkeyService) list(ctx context.Context, userID string) ([]PasskeyCredential, error) {
	var result []PasskeyCredential
	err := service.db.WithContext(ctx).Raw(`SELECT id, user_id, name, created_at, updated_at, last_used_at
		FROM passkey_credentials WHERE user_id = ? ORDER BY created_at ASC`, userID).Scan(&result).Error
	return result, err
}

func (service *passkeyService) rename(ctx context.Context, userID, credentialID, name string) (PasskeyCredential, error) {
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > 128 {
		return PasskeyCredential{}, fmt.Errorf("Passkey 名称必须为 1 至 128 个字符")
	}
	now := time.Now().UTC()
	result := service.db.WithContext(ctx).Exec("UPDATE passkey_credentials SET name = ?, updated_at = ? WHERE id = ? AND user_id = ?", name, now, credentialID, userID)
	if result.Error != nil {
		return PasskeyCredential{}, result.Error
	}
	if result.RowsAffected == 0 {
		return PasskeyCredential{}, gorm.ErrRecordNotFound
	}
	var credential PasskeyCredential
	err := service.db.WithContext(ctx).Raw(`SELECT id, user_id, name, created_at, updated_at, last_used_at
		FROM passkey_credentials WHERE id = ?`, credentialID).Scan(&credential).Error
	return credential, err
}

func (service *passkeyService) delete(ctx context.Context, userID, credentialID string) error {
	result := service.db.WithContext(ctx).Exec("DELETE FROM passkey_credentials WHERE id = ? AND user_id = ?", credentialID, userID)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (service *passkeyService) loadUser(ctx context.Context, userID string) (*passkeyUser, error) {
	var username string
	if err := service.db.WithContext(ctx).Raw("SELECT username FROM users WHERE id = ? AND status = 'active'", userID).Row().Scan(&username); err != nil {
		return nil, err
	}
	var records []struct{ EncryptedCredential []byte }
	if err := service.db.WithContext(ctx).Raw("SELECT encrypted_credential FROM passkey_credentials WHERE user_id = ?", userID).Scan(&records).Error; err != nil {
		return nil, err
	}
	credentials := make([]webauthn.Credential, 0, len(records))
	for _, record := range records {
		credential, err := service.decryptCredential(userID, record.EncryptedCredential)
		if err != nil {
			return nil, fmt.Errorf("读取 Passkey 凭据失败: %w", err)
		}
		credentials = append(credentials, credential)
	}
	return &passkeyUser{id: userID, username: username, credentials: credentials}, nil
}

func (service *passkeyService) loadDiscoverableUser(ctx context.Context, rawID, userHandle []byte) (webauthn.User, error) {
	userID := string(userHandle)
	if _, err := uuid.Parse(userID); err != nil {
		return nil, fmt.Errorf("Passkey 用户标识无效")
	}
	var count int64
	if err := service.db.WithContext(ctx).Raw("SELECT COUNT(*) FROM passkey_credentials WHERE user_id = ? AND credential_id_hash = ?", userID, hashCredentialID(rawID)).Scan(&count).Error; err != nil {
		return nil, err
	}
	if count != 1 {
		return nil, fmt.Errorf("Passkey 不属于此用户")
	}
	return service.loadUser(ctx, userID)
}

func (service *passkeyService) saveCredential(ctx context.Context, userID, name string, credential *webauthn.Credential) (PasskeyCredential, error) {
	ciphertext, err := service.encryptCredential(userID, credential)
	if err != nil {
		return PasskeyCredential{}, err
	}
	now := time.Now().UTC()
	record := PasskeyCredential{ID: uuid.NewString(), UserID: userID, Name: name, CreatedAt: now, UpdatedAt: now}
	err = service.db.WithContext(ctx).Exec(`INSERT INTO passkey_credentials
		(id, user_id, name, credential_id_hash, encrypted_credential, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, record.ID, userID, name, hashCredentialID(credential.ID), ciphertext, now, now).Error
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return PasskeyCredential{}, fmt.Errorf("此 Passkey 已经登记")
		}
		return PasskeyCredential{}, err
	}
	return record, nil
}

func (service *passkeyService) updateCredential(ctx context.Context, userID string, credential *webauthn.Credential) error {
	ciphertext, err := service.encryptCredential(userID, credential)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	result := service.db.WithContext(ctx).Exec(`UPDATE passkey_credentials SET encrypted_credential = ?, last_used_at = ?, updated_at = ?
		WHERE user_id = ? AND credential_id_hash = ?`, ciphertext, now, now, userID, hashCredentialID(credential.ID))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("找不到已验证的 Passkey")
	}
	return nil
}

func (service *passkeyService) encryptCredential(userID string, credential *webauthn.Credential) ([]byte, error) {
	plaintext, err := json.Marshal(credential)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, service.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return append(nonce, service.aead.Seal(nil, nonce, plaintext, []byte(userID))...), nil
}

func (service *passkeyService) decryptCredential(userID string, ciphertext []byte) (webauthn.Credential, error) {
	if len(ciphertext) < service.aead.NonceSize() {
		return webauthn.Credential{}, fmt.Errorf("凭据密文无效")
	}
	plaintext, err := service.aead.Open(nil, ciphertext[:service.aead.NonceSize()], ciphertext[service.aead.NonceSize():], []byte(userID))
	if err != nil {
		return webauthn.Credential{}, err
	}
	var credential webauthn.Credential
	err = json.Unmarshal(plaintext, &credential)
	return credential, err
}

func (service *passkeyService) storeSession(session passkeySession) (string, error) {
	buffer := make([]byte, 32)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	sessionID := base64.RawURLEncoding.EncodeToString(buffer)
	session.expiresAt = time.Now().Add(passkeySessionLifetime)
	service.mu.Lock()
	defer service.mu.Unlock()
	service.removeExpiredSessionsLocked()
	if len(service.sessions) >= passkeyMaxSessions {
		service.removeOldestSessionLocked()
	}
	service.sessions[sessionID] = session
	return sessionID, nil
}

func (service *passkeyService) takeSession(sessionID, kind, userID string) (passkeySession, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	service.removeExpiredSessionsLocked()
	sessionID = strings.TrimSpace(sessionID)
	session, ok := service.sessions[sessionID]
	if !ok {
		return passkeySession{}, fmt.Errorf("Passkey 会话不存在或已过期，请重试")
	}
	delete(service.sessions, sessionID)
	if session.kind != kind || (userID != "" && session.userID != userID) {
		return passkeySession{}, fmt.Errorf("Passkey 会话与当前操作不匹配")
	}
	return session, nil
}

func (service *passkeyService) removeExpiredSessionsLocked() {
	now := time.Now()
	for id, session := range service.sessions {
		if !session.expiresAt.After(now) {
			delete(service.sessions, id)
		}
	}
}

func (service *passkeyService) removeOldestSessionLocked() {
	var oldestID string
	var oldestExpiry time.Time
	for id, session := range service.sessions {
		if oldestID == "" || session.expiresAt.Before(oldestExpiry) {
			oldestID, oldestExpiry = id, session.expiresAt
		}
	}
	if oldestID != "" {
		delete(service.sessions, oldestID)
	}
}

func hashCredentialID(id []byte) string {
	hash := sha256.Sum256(id)
	return hex.EncodeToString(hash[:])
}

var errPasskeyUnavailable = errors.New("Passkey 尚未配置")
