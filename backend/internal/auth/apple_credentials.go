package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"time"

	"gorm.io/gorm"
)

type appleCredentialStore struct {
	db   *gorm.DB
	aead cipher.AEAD
	now  func() time.Time
}

func newAppleCredentialStore(db *gorm.DB, encodedKey string) (*appleCredentialStore, error) {
	key, err := base64.RawStdEncoding.DecodeString(encodedKey)
	if err != nil || len(key) != 32 {
		return nil, fmt.Errorf("Apple 凭据加密密钥必须是 32-byte base64")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &appleCredentialStore{db: db, aead: aead, now: time.Now}, nil
}

func (store *appleCredentialStore) Save(userID string, refreshToken string) error {
	return store.save(store.db, userID, refreshToken)
}

func (store *appleCredentialStore) SaveTx(tx *gorm.DB, userID string, refreshToken string) error {
	return store.save(tx, userID, refreshToken)
}

func (store *appleCredentialStore) save(db *gorm.DB, userID string, refreshToken string) error {
	if userID == "" || refreshToken == "" {
		return fmt.Errorf("Apple 凭据不完整")
	}
	nonce := make([]byte, store.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return err
	}
	ciphertext := append(nonce, store.aead.Seal(nil, nonce, []byte(refreshToken), []byte(userID))...)
	now := store.now().UTC()
	return db.Exec(`INSERT INTO apple_credentials(user_id, refresh_token_ciphertext, created_at, updated_at) VALUES (?, ?, ?, ?)
		ON CONFLICT(user_id) DO UPDATE SET refresh_token_ciphertext = excluded.refresh_token_ciphertext, updated_at = excluded.updated_at`, userID, ciphertext, now, now).Error
}

func (store *appleCredentialStore) RefreshToken(userID string) (string, error) {
	var ciphertext []byte
	if err := store.db.Raw("SELECT refresh_token_ciphertext FROM apple_credentials WHERE user_id = ?", userID).Row().Scan(&ciphertext); err != nil {
		return "", err
	}
	if len(ciphertext) < store.aead.NonceSize() {
		return "", fmt.Errorf("Apple 凭据密文无效")
	}
	nonce, encrypted := ciphertext[:store.aead.NonceSize()], ciphertext[store.aead.NonceSize():]
	plaintext, err := store.aead.Open(nil, nonce, encrypted, []byte(userID))
	if err != nil {
		return "", fmt.Errorf("解密 Apple 凭据: %w", err)
	}
	return string(plaintext), nil
}
