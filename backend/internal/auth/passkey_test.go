package auth

import (
	"context"
	"encoding/base64"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/ydfk/measure-trail/backend/internal/config"
	"github.com/ydfk/measure-trail/backend/internal/database"
)

func TestPasskeyRegistrationOptionsAndCredentialManagement(t *testing.T) {
	db, err := database.Open(config.Database{Path: filepath.Join(t.TempDir(), "passkey.sqlite"), BusyTimeoutMS: 1000})
	if err != nil {
		t.Fatal(err)
	}
	authService, err := NewService(db, testAuthConfig())
	if err != nil {
		t.Fatal(err)
	}
	if err := authService.EnsureDefaultUser("admin", "111111"); err != nil {
		t.Fatal(err)
	}
	if err := authService.ConfigurePasskeys(testPasskeyConfig()); err != nil {
		t.Fatal(err)
	}
	session, err := authService.Login("admin", "111111", "测试设备")
	if err != nil {
		t.Fatal(err)
	}
	userID, err := authService.UserIDFromAccessToken(session.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	creation, sessionID, err := authService.passkeys.beginRegistration(context.Background(), userID, "我的 iPhone")
	if err != nil {
		t.Fatal(err)
	}
	if sessionID == "" || creation.Response.RelyingParty.ID != "localhost" {
		t.Fatalf("注册挑战无效: session=%q rp=%q", sessionID, creation.Response.RelyingParty.ID)
	}
	record, err := authService.passkeys.saveCredential(context.Background(), userID, "我的 iPhone", &webauthn.Credential{ID: []byte("credential-id")})
	if err != nil {
		t.Fatal(err)
	}
	var encrypted []byte
	if err := db.Raw("SELECT encrypted_credential FROM passkey_credentials WHERE id = ?", record.ID).Row().Scan(&encrypted); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encrypted), "credential-id") {
		t.Fatal("Passkey 凭据不应以明文保存")
	}
	items, err := authService.passkeys.list(context.Background(), userID)
	if err != nil || len(items) != 1 {
		t.Fatalf("Passkey 列表=%#v err=%v", items, err)
	}
	renamed, err := authService.passkeys.rename(context.Background(), userID, record.ID, "工作手机")
	if err != nil || renamed.Name != "工作手机" {
		t.Fatalf("重命名结果=%#v err=%v", renamed, err)
	}
	if err := authService.passkeys.delete(context.Background(), userID, record.ID); err != nil {
		t.Fatal(err)
	}
}

func TestPasskeyChallengeCanOnlyBeUsedOnce(t *testing.T) {
	service := &passkeyService{sessions: map[string]passkeySession{}}
	sessionID, err := service.storeSession(passkeySession{kind: "login"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.takeSession(sessionID, "login", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := service.takeSession(sessionID, "login", ""); err == nil {
		t.Fatal("同一个 Passkey 挑战不应被重复消费")
	}
}

func testPasskeyConfig() config.Passkey {
	return config.Passkey{
		RPID:                    "localhost",
		RPName:                  "量迹测试",
		Origins:                 []string{"http://localhost:21000"},
		CredentialEncryptionKey: base64.RawStdEncoding.EncodeToString([]byte("01234567890123456789012345678901")),
	}
}

func testAuthConfig() config.Auth {
	return config.Auth{
		Issuer:        "measuretrail",
		Audience:      "measuretrail-ios",
		AccessSecret:  "01234567890123456789012345678901",
		RefreshSecret: "abcdefghijklmnopqrstuvwxyzABCDEF",
	}
}
