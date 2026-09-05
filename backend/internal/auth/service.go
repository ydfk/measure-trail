package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/ydfk/measure-trail/backend/internal/config"
	"gorm.io/gorm"
)

var (
	ErrRegistrationClosed = errors.New("暂未开放注册，请使用已有账号登录")
	ErrInvalidCredentials = errors.New("邮箱或密码不正确")
	ErrEmailNotVerified   = errors.New("邮箱尚未验证")
	ErrInvalidToken       = errors.New("token 无效或已过期")
)

type Service struct {
	registrationEnabled bool
	db                  *gorm.DB
	tokens              *tokenManager
	appleCredentials    *appleCredentialStore
	now                 func() time.Time
}

func (service *Service) ConfigureAppleCredentials(encodedKey string) error {
	store, err := newAppleCredentialStore(service.db, encodedKey)
	if err != nil {
		return err
	}
	service.appleCredentials = store
	return nil
}

type Session struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int
}

type SessionInfo struct {
	ID          string
	DeviceLabel string
	CreatedAt   time.Time
	ExpiresAt   time.Time
}

func NewService(db *gorm.DB, authConfig config.Auth) (*Service, error) {
	tokens, err := newTokenManager(authConfig)
	if err != nil {
		return nil, err
	}
	return &Service{db: db, tokens: tokens, now: time.Now, registrationEnabled: authConfig.RegistrationEnabled}, nil
}

func (service *Service) Register(email string, password string) (string, error) {
	if !service.registrationEnabled {
		return "", ErrRegistrationClosed
	}
	email = normalizeEmail(email)
	if email == "" || !passwordIsValid(password) {
		return "", fmt.Errorf("邮箱或密码不符合要求")
	}
	hash, err := hashPassword(password)
	if err != nil {
		return "", err
	}
	verification, verificationHash, err := newOpaqueToken()
	if err != nil {
		return "", err
	}
	now := service.now().UTC()
	err = service.db.Transaction(func(tx *gorm.DB) error {
		userID := uuid.NewString()
		if err := tx.Exec("INSERT INTO users(id, email, created_at, updated_at) VALUES (?, ?, ?, ?)", userID, email, now, now).Error; err != nil {
			return fmt.Errorf("创建账号: %w", err)
		}
		if err := tx.Exec("INSERT INTO password_credentials(user_id, password_hash, updated_at) VALUES (?, ?, ?)", userID, hash, now).Error; err != nil {
			return err
		}
		if err := tx.Exec("INSERT INTO auth_identities(id, user_id, provider, provider_subject, created_at) VALUES (?, ?, 'password', ?, ?)", uuid.NewString(), userID, email, now).Error; err != nil {
			return err
		}
		return tx.Exec("INSERT INTO email_tokens(id, user_id, purpose, token_hash, expires_at, created_at) VALUES (?, ?, 'verify_email', ?, ?, ?)", uuid.NewString(), userID, verificationHash, now.Add(verifyTokenLifetime), now).Error
	})
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed: users.email") {
			return "", fmt.Errorf("无法创建账号")
		}
		return "", err
	}
	return verification, nil
}

func (service *Service) VerifyEmail(rawToken string) error {
	return service.consumeEmailToken(rawToken, "verify_email", func(tx *gorm.DB, userID string, now time.Time) error {
		return tx.Exec("UPDATE users SET email_verified_at = ?, updated_at = ? WHERE id = ?", now, now, userID).Error
	})
}

func (service *Service) ResendVerification(email string) (string, error) {
	return service.newEmailTokenForEmail(email, "verify_email", verifyTokenLifetime, true)
}

func (service *Service) RequestPasswordReset(email string) (string, error) {
	return service.newEmailTokenForEmail(email, "reset_password", resetTokenLifetime, false)
}

func (service *Service) ResetPassword(rawToken string, password string) error {
	if !passwordIsValid(password) {
		return fmt.Errorf("密码不符合要求")
	}
	hash, err := hashPassword(password)
	if err != nil {
		return err
	}
	return service.consumeEmailToken(rawToken, "reset_password", func(tx *gorm.DB, userID string, now time.Time) error {
		if err := tx.Exec("UPDATE password_credentials SET password_hash = ?, updated_at = ? WHERE user_id = ?", hash, now, userID).Error; err != nil {
			return err
		}
		return tx.Exec("UPDATE refresh_tokens SET revoked_at = ? WHERE user_id = ? AND revoked_at IS NULL", now, userID).Error
	})
}

func (service *Service) Login(email string, password string, deviceLabel string) (Session, error) {
	var userID, passwordHash string
	var verifiedAt sql.NullString
	err := service.db.Raw(`SELECT users.id, users.email_verified_at, password_credentials.password_hash
		FROM users JOIN password_credentials ON password_credentials.user_id = users.id
		WHERE users.email = ? AND users.status = 'active'`, normalizeEmail(email)).Row().Scan(&userID, &verifiedAt, &passwordHash)
	if err != nil || !verifyPassword(passwordHash, password) {
		return Session{}, ErrInvalidCredentials
	}
	if !verifiedAt.Valid {
		return Session{}, ErrEmailNotVerified
	}
	return service.issueSession(userID, deviceLabel)
}

func (service *Service) SignInWithApple(ctx context.Context, verifier AppleVerifier, tokenClient AppleTokenClient, rawToken string, authorizationCode string, nonce string, deviceLabel string) (Session, error) {
	identity, err := verifier.Verify(ctx, rawToken, nonce)
	if err != nil {
		return Session{}, err
	}
	if service.appleCredentials == nil {
		return Session{}, ErrAppleUnavailable
	}
	tokens, err := tokenClient.Exchange(ctx, authorizationCode)
	if err != nil {
		return Session{}, err
	}
	if err := service.consumeAppleNonce(nonce); err != nil {
		return Session{}, err
	}
	userID, err := service.userIDForAppleSubject(identity.Subject)
	if err == nil {
		if err := service.appleCredentials.Save(userID, tokens.RefreshToken); err != nil {
			return Session{}, err
		}
		return service.issueSession(userID, deviceLabel)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return Session{}, err
	}
	if !service.registrationEnabled {
		return Session{}, ErrRegistrationClosed
	}
	if identity.Email == "" {
		return Session{}, ErrAppleEmailRequired
	}
	if service.emailExists(identity.Email) {
		return Session{}, ErrAppleAccountLinkRequired
	}
	userID = uuid.NewString()
	now := service.now().UTC()
	err = service.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("INSERT INTO users(id, email, email_verified_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?)", userID, identity.Email, now, now, now).Error; err != nil {
			return err
		}
		if err := tx.Exec("INSERT INTO auth_identities(id, user_id, provider, provider_subject, created_at) VALUES (?, ?, 'apple', ?, ?)", uuid.NewString(), userID, identity.Subject, now).Error; err != nil {
			return err
		}
		return service.appleCredentials.SaveTx(tx, userID, tokens.RefreshToken)
	})
	if err != nil {
		if service.emailExists(identity.Email) {
			return Session{}, ErrAppleAccountLinkRequired
		}
		return Session{}, err
	}
	return service.issueSession(userID, deviceLabel)
}

func (service *Service) LinkAppleIdentity(ctx context.Context, userID string, verifier AppleVerifier, tokenClient AppleTokenClient, rawToken string, authorizationCode string, nonce string) error {
	identity, err := verifier.Verify(ctx, rawToken, nonce)
	if err != nil {
		return err
	}
	if service.appleCredentials == nil {
		return ErrAppleUnavailable
	}
	tokens, err := tokenClient.Exchange(ctx, authorizationCode)
	if err != nil {
		return err
	}
	if err := service.consumeAppleNonce(nonce); err != nil {
		return err
	}
	linkedUserID, err := service.userIDForAppleSubject(identity.Subject)
	if err == nil {
		if linkedUserID == userID {
			return service.appleCredentials.Save(userID, tokens.RefreshToken)
		}
		return ErrAppleIdentityLinked
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	now := service.now().UTC()
	return service.db.Transaction(func(tx *gorm.DB) error {
		result := tx.Exec("INSERT INTO auth_identities(id, user_id, provider, provider_subject, created_at) SELECT ?, id, 'apple', ?, ? FROM users WHERE id = ? AND status = 'active'", uuid.NewString(), identity.Subject, now, userID)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrInvalidToken
		}
		return service.appleCredentials.SaveTx(tx, userID, tokens.RefreshToken)
	})
}

func (service *Service) userIDForAppleSubject(subject string) (string, error) {
	var userID string
	err := service.db.Raw("SELECT user_id FROM auth_identities WHERE provider = 'apple' AND provider_subject = ?", subject).Row().Scan(&userID)
	return userID, err
}

func (service *Service) emailExists(email string) bool {
	var count int64
	service.db.Raw("SELECT COUNT(*) FROM users WHERE email = ? AND status = 'active'", email).Scan(&count)
	return count > 0
}

func (service *Service) NewAppleNonce() (string, error) {
	rawNonce, nonceHash, err := newOpaqueToken()
	if err != nil {
		return "", err
	}
	now := service.now().UTC()
	err = service.db.Exec("INSERT INTO apple_login_nonces(id, token_hash, expires_at, created_at) VALUES (?, ?, ?, ?)", uuid.NewString(), nonceHash, now.Add(appleNonceLifetime), now).Error
	if err != nil {
		return "", err
	}
	return rawNonce, nil
}

func (service *Service) consumeAppleNonce(rawNonce string) error {
	now := service.now().UTC()
	result := service.db.Exec("UPDATE apple_login_nonces SET used_at = ? WHERE token_hash = ? AND used_at IS NULL AND datetime(expires_at) > datetime(?)", now, hashToken(rawNonce), now)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrInvalidToken
	}
	return nil
}

func (service *Service) Refresh(rawToken string, deviceLabel string) (Session, error) {
	now := service.now().UTC()
	var tokenID, userID string
	err := service.db.Raw(`SELECT id, user_id FROM refresh_tokens
		WHERE token_hash = ? AND revoked_at IS NULL AND datetime(expires_at) > datetime(?)`, hashToken(rawToken), now).Row().Scan(&tokenID, &userID)
	if err != nil {
		return Session{}, ErrInvalidToken
	}
	return service.rotateSession(tokenID, userID, deviceLabel)
}

func (service *Service) Logout(rawToken string) error {
	return service.db.Exec("UPDATE refresh_tokens SET revoked_at = ? WHERE token_hash = ? AND revoked_at IS NULL", service.now().UTC(), hashToken(rawToken)).Error
}

func (service *Service) ListSessions(userID string) ([]SessionInfo, error) {
	rows, err := service.db.Raw(`SELECT id, COALESCE(device_label, ''), strftime('%Y-%m-%dT%H:%M:%fZ', created_at), strftime('%Y-%m-%dT%H:%M:%fZ', expires_at)
		FROM refresh_tokens WHERE user_id = ? AND revoked_at IS NULL AND datetime(expires_at) > datetime(?)
		ORDER BY created_at DESC`, userID, service.now().UTC()).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	sessions := make([]SessionInfo, 0)
	for rows.Next() {
		var session SessionInfo
		var createdAt, expiresAt string
		if err := rows.Scan(&session.ID, &session.DeviceLabel, &createdAt, &expiresAt); err != nil {
			return nil, err
		}
		if session.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt); err != nil {
			return nil, err
		}
		if session.ExpiresAt, err = time.Parse(time.RFC3339Nano, expiresAt); err != nil {
			return nil, err
		}
		sessions = append(sessions, session)
	}
	return sessions, rows.Err()
}

func (service *Service) RevokeSession(userID string, sessionID string) error {
	result := service.db.Exec("UPDATE refresh_tokens SET revoked_at = ? WHERE id = ? AND user_id = ? AND revoked_at IS NULL", service.now().UTC(), sessionID, userID)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrInvalidToken
	}
	return nil
}

func (service *Service) DeleteAccount(ctx context.Context, userID string, tokenClient AppleTokenClient) error {
	if service.appleCredentials != nil {
		refreshToken, err := service.appleCredentials.RefreshToken(userID)
		if err == nil {
			if err := tokenClient.Revoke(ctx, refreshToken); err != nil {
				return err
			}
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
	}
	result := service.db.Exec("DELETE FROM users WHERE id = ? AND status = 'active'", userID)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrInvalidToken
	}
	return nil
}

func (service *Service) UserIDFromAccessToken(rawToken string) (string, error) {
	userID, err := service.tokens.verifyAccessToken(rawToken)
	if err != nil {
		return "", err
	}
	var activeUser string
	err = service.db.Raw("SELECT id FROM users WHERE id = ? AND status = 'active'", userID).Row().Scan(&activeUser)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrInvalidToken
	}
	if err != nil {
		return "", err
	}
	return activeUser, nil
}

func (service *Service) issueSession(userID string, deviceLabel string) (Session, error) {
	now := service.now().UTC()
	accessToken, err := service.tokens.newAccessToken(userID, now)
	if err != nil {
		return Session{}, err
	}
	rawRefresh, refreshHash, err := newOpaqueToken()
	if err != nil {
		return Session{}, err
	}
	if err := service.db.Exec("INSERT INTO refresh_tokens(id, user_id, token_hash, expires_at, device_label, created_at) VALUES (?, ?, ?, ?, ?, ?)", uuid.NewString(), userID, refreshHash, now.Add(refreshTokenLifetime), deviceLabel, now).Error; err != nil {
		return Session{}, err
	}
	return Session{AccessToken: accessToken, RefreshToken: rawRefresh, ExpiresIn: int(accessTokenLifetime.Seconds())}, nil
}

func (service *Service) rotateSession(previousID string, userID string, deviceLabel string) (Session, error) {
	return service.issueSessionWithRevocation(previousID, userID, deviceLabel)
}

func (service *Service) issueSessionWithRevocation(previousID string, userID string, deviceLabel string) (Session, error) {
	now := service.now().UTC()
	accessToken, err := service.tokens.newAccessToken(userID, now)
	if err != nil {
		return Session{}, err
	}
	rawRefresh, refreshHash, err := newOpaqueToken()
	if err != nil {
		return Session{}, err
	}
	newID := uuid.NewString()
	err = service.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("INSERT INTO refresh_tokens(id, user_id, token_hash, expires_at, device_label, created_at) VALUES (?, ?, ?, ?, ?, ?)", newID, userID, refreshHash, now.Add(refreshTokenLifetime), deviceLabel, now).Error; err != nil {
			return err
		}
		result := tx.Exec("UPDATE refresh_tokens SET revoked_at = ?, replaced_by_id = ? WHERE id = ? AND revoked_at IS NULL", now, newID, previousID)
		if result.Error != nil || result.RowsAffected != 1 {
			return ErrInvalidToken
		}
		return nil
	})
	if err != nil {
		return Session{}, err
	}
	return Session{AccessToken: accessToken, RefreshToken: rawRefresh, ExpiresIn: int(accessTokenLifetime.Seconds())}, nil
}

func (service *Service) consumeEmailToken(rawToken string, purpose string, action func(*gorm.DB, string, time.Time) error) error {
	now := service.now().UTC()
	return service.db.Transaction(func(tx *gorm.DB) error {
		var id, userID string
		err := tx.Raw("SELECT id, user_id FROM email_tokens WHERE token_hash = ? AND purpose = ? AND used_at IS NULL AND datetime(expires_at) > datetime(?)", hashToken(rawToken), purpose, now).Row().Scan(&id, &userID)
		if err != nil {
			return ErrInvalidToken
		}
		if err := action(tx, userID, now); err != nil {
			return err
		}
		return tx.Exec("UPDATE email_tokens SET used_at = ? WHERE id = ? AND used_at IS NULL", now, id).Error
	})
}

func (service *Service) newEmailTokenForEmail(email string, purpose string, lifetime time.Duration, onlyUnverified bool) (string, error) {
	email = normalizeEmail(email)
	if email == "" {
		return "", nil
	}
	var userID string
	query := "SELECT id FROM users WHERE email = ? AND status = 'active'"
	if onlyUnverified {
		query += " AND email_verified_at IS NULL"
	}
	if err := service.db.Raw(query, email).Row().Scan(&userID); err != nil {
		return "", nil
	}
	rawToken, tokenHash, err := newOpaqueToken()
	if err != nil {
		return "", err
	}
	now := service.now().UTC()
	err = service.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("UPDATE email_tokens SET used_at = ? WHERE user_id = ? AND purpose = ? AND used_at IS NULL", now, userID, purpose).Error; err != nil {
			return err
		}
		return tx.Exec("INSERT INTO email_tokens(id, user_id, purpose, token_hash, expires_at, created_at) VALUES (?, ?, ?, ?, ?, ?)", uuid.NewString(), userID, purpose, tokenHash, now.Add(lifetime), now).Error
	})
	if err != nil {
		return "", err
	}
	return rawToken, nil
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}
