package auth

import (
	"encoding/base64"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ydfk/measure-trail/backend/internal/config"
	"github.com/ydfk/measure-trail/backend/internal/database"
)

func TestAppleCredentialStoreEncryptsAndBindsTokenToUser(t *testing.T) {
	db, err := database.Open(config.Database{Path: filepath.Join(t.TempDir(), "measuretrail.sqlite"), BusyTimeoutMS: 1000})
	if err != nil {
		t.Fatalf("打开数据库: %v", err)
	}
	now := time.Now().UTC()
	if err := db.Exec("INSERT INTO users(id, email, email_verified_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?)", "user-1", "user@example.com", now, now, now).Error; err != nil {
		t.Fatalf("创建用户: %v", err)
	}
	key := base64.RawStdEncoding.EncodeToString([]byte("01234567890123456789012345678901"))
	store, err := newAppleCredentialStore(db, key)
	if err != nil {
		t.Fatalf("创建凭据存储: %v", err)
	}
	if err := store.Save("user-1", "apple-refresh-token"); err != nil {
		t.Fatalf("保存凭据: %v", err)
	}
	var ciphertext string
	if err := db.Raw("SELECT refresh_token_ciphertext FROM apple_credentials WHERE user_id = ?", "user-1").Row().Scan(&ciphertext); err != nil {
		t.Fatalf("读取密文: %v", err)
	}
	if strings.Contains(ciphertext, "apple-refresh-token") {
		t.Fatal("Apple refresh token 被明文保存")
	}
	refreshToken, err := store.RefreshToken("user-1")
	if err != nil || refreshToken != "apple-refresh-token" {
		t.Fatalf("读取凭据 = %q, error = %v", refreshToken, err)
	}
}
