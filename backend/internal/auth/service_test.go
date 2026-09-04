package auth

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/ydfk/measure-trail/backend/internal/config"
	"github.com/ydfk/measure-trail/backend/internal/database"
)

func TestRegisterVerifyLoginAndRefreshRotation(t *testing.T) {
	db, err := database.Open(config.Database{Path: filepath.Join(t.TempDir(), "measuretrail.sqlite"), BusyTimeoutMS: 1000})
	if err != nil {
		t.Fatalf("打开数据库: %v", err)
	}
	service, err := NewService(db, config.Auth{
		Issuer:        "measuretrail",
		Audience:      "measuretrail-ios",
		AccessSecret:  "01234567890123456789012345678901",
		RefreshSecret: "abcdefghijklmnopqrstuvwxyzABCDEF",
	})
	if err != nil {
		t.Fatalf("创建认证服务: %v", err)
	}

	verification, err := service.Register(" User@Example.com ", "correct-horse-battery-staple")
	if err != nil {
		t.Fatalf("注册: %v", err)
	}
	if _, err := service.Login("user@example.com", "correct-horse-battery-staple", "测试设备"); !errors.Is(err, ErrEmailNotVerified) {
		t.Fatalf("未验证登录 error = %v, want ErrEmailNotVerified", err)
	}
	if err := service.VerifyEmail(verification); err != nil {
		t.Fatalf("验证邮箱: %v", err)
	}
	if err := service.VerifyEmail(verification); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("重复验证 error = %v, want ErrInvalidToken", err)
	}

	session, err := service.Login("user@example.com", "correct-horse-battery-staple", "测试设备")
	if err != nil {
		t.Fatalf("登录: %v", err)
	}
	if _, err := service.tokens.verifyAccessToken(session.AccessToken); err != nil {
		t.Fatalf("验证 access token: %v", err)
	}
	rotated, err := service.Refresh(session.RefreshToken, "测试设备")
	if err != nil {
		t.Fatalf("刷新 token: %v", err)
	}
	if rotated.RefreshToken == session.RefreshToken {
		t.Fatal("refresh token 未轮换")
	}
	if _, err := service.Refresh(session.RefreshToken, "测试设备"); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("重放 refresh token error = %v, want ErrInvalidToken", err)
	}

	resetToken, err := service.RequestPasswordReset("user@example.com")
	if err != nil || resetToken == "" {
		t.Fatalf("请求密码重置 token = %q, error = %v", resetToken, err)
	}
	if err := service.ResetPassword(resetToken, "new-correct-horse-battery-staple"); err != nil {
		t.Fatalf("重置密码: %v", err)
	}
	if _, err := service.Login("user@example.com", "correct-horse-battery-staple", "测试设备"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("旧密码登录 error = %v, want ErrInvalidCredentials", err)
	}
	if _, err := service.Login("user@example.com", "new-correct-horse-battery-staple", "测试设备"); err != nil {
		t.Fatalf("新密码登录: %v", err)
	}
}

func TestRevokeSessionRequiresOwner(t *testing.T) {
	db, err := database.Open(config.Database{Path: filepath.Join(t.TempDir(), "measuretrail.sqlite"), BusyTimeoutMS: 1000})
	if err != nil {
		t.Fatalf("打开数据库: %v", err)
	}
	service, err := NewService(db, config.Auth{Issuer: "measuretrail", Audience: "measuretrail-ios", AccessSecret: "01234567890123456789012345678901", RefreshSecret: "abcdefghijklmnopqrstuvwxyzABCDEF"})
	if err != nil {
		t.Fatalf("创建认证服务: %v", err)
	}

	firstToken, err := service.Register("first@example.com", "correct-horse-battery-staple")
	if err != nil || service.VerifyEmail(firstToken) != nil {
		t.Fatalf("初始化第一个账号: %v", err)
	}
	secondToken, err := service.Register("second@example.com", "correct-horse-battery-staple")
	if err != nil || service.VerifyEmail(secondToken) != nil {
		t.Fatalf("初始化第二个账号: %v", err)
	}
	firstSession, err := service.Login("first@example.com", "correct-horse-battery-staple", "first")
	if err != nil {
		t.Fatalf("第一个账号登录: %v", err)
	}
	secondSession, err := service.Login("second@example.com", "correct-horse-battery-staple", "second")
	if err != nil {
		t.Fatalf("第二个账号登录: %v", err)
	}
	firstID, err := service.UserIDFromAccessToken(firstSession.AccessToken)
	if err != nil {
		t.Fatalf("解析第一个 access token: %v", err)
	}
	secondID, err := service.UserIDFromAccessToken(secondSession.AccessToken)
	if err != nil {
		t.Fatalf("解析第二个 access token: %v", err)
	}
	secondSessions, err := service.ListSessions(secondID)
	if err != nil || len(secondSessions) != 1 {
		t.Fatalf("第二个账号会话 = %#v, error = %v", secondSessions, err)
	}
	if err := service.RevokeSession(firstID, secondSessions[0].ID); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("跨账号撤销 error = %v, want ErrInvalidToken", err)
	}
	if err := service.RevokeSession(secondID, secondSessions[0].ID); err != nil {
		t.Fatalf("本人撤销会话: %v", err)
	}
}

func TestDeleteAccountRevokesAllAuthenticationData(t *testing.T) {
	db, err := database.Open(config.Database{Path: filepath.Join(t.TempDir(), "measuretrail.sqlite"), BusyTimeoutMS: 1000})
	if err != nil {
		t.Fatalf("打开数据库: %v", err)
	}
	service, err := NewService(db, config.Auth{Issuer: "measuretrail", Audience: "measuretrail-ios", AccessSecret: "01234567890123456789012345678901", RefreshSecret: "abcdefghijklmnopqrstuvwxyzABCDEF"})
	if err != nil {
		t.Fatalf("创建认证服务: %v", err)
	}
	verification, err := service.Register("delete@example.com", "correct-horse-battery-staple")
	if err != nil || service.VerifyEmail(verification) != nil {
		t.Fatalf("初始化账号: %v", err)
	}
	session, err := service.Login("delete@example.com", "correct-horse-battery-staple", "设备")
	if err != nil {
		t.Fatalf("登录: %v", err)
	}
	userID, err := service.UserIDFromAccessToken(session.AccessToken)
	if err != nil {
		t.Fatalf("解析 access token: %v", err)
	}
	now := time.Now().UTC()
	for _, statement := range []struct {
		query string
		args  []any
	}{
		{"INSERT INTO profiles(user_id, preferred_unit, timezone, updated_at) VALUES (?, 'kg', 'Asia/Shanghai', ?)", []any{userID, now}},
		{"INSERT INTO measurements(id, user_id, recorded_on, weight_g, note, source, version, client_mutation_id, created_at, updated_at) VALUES ('measurement-delete-1', ?, '2025-09-26', 76000, '', 'manual', 1, 'delete-account-measurement', ?, ?)", []any{userID, now, now}},
		{"INSERT INTO measurement_changes(user_id, measurement_id, changed_at) VALUES (?, 'measurement-delete-1', ?)", []any{userID, now}},
		{"INSERT INTO client_mutations(user_id, mutation_id, measurement_id, created_at) VALUES (?, 'delete-account-mutation', 'measurement-delete-1', ?)", []any{userID, now}},
		{"INSERT INTO legacy_imports(id, user_id, source_sha256, source_path, row_count, imported_at, report_json) VALUES ('legacy-delete-1', ?, 'sha256-delete-test', '/tmp/delete-test.sqlite', 1, ?, '{}')", []any{userID, now}},
	} {
		if err := db.Exec(statement.query, statement.args...).Error; err != nil {
			t.Fatalf("准备关联数据: %v", err)
		}
	}
	if err := service.DeleteAccount(context.Background(), userID, unavailableAppleTokenClient{}); err != nil {
		t.Fatalf("删除账号: %v", err)
	}
	if _, err := service.Login("delete@example.com", "correct-horse-battery-staple", "设备"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("删除后登录 error = %v, want ErrInvalidCredentials", err)
	}
	if _, err := service.Refresh(session.RefreshToken, "设备"); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("删除后 refresh error = %v, want ErrInvalidToken", err)
	}
	if _, err := service.UserIDFromAccessToken(session.AccessToken); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("删除后 access token error = %v, want ErrInvalidToken", err)
	}
	for _, table := range []string{"profiles", "measurements", "measurement_changes", "client_mutations", "legacy_imports", "password_credentials", "auth_identities", "refresh_tokens", "email_tokens"} {
		var count int
		if err := db.Raw("SELECT COUNT(*) FROM "+table+" WHERE user_id = ?", userID).Row().Scan(&count); err != nil {
			t.Fatalf("检查 %s: %v", table, err)
		}
		if count != 0 {
			t.Fatalf("删除后 %s 仍有 %d 条关联数据", table, count)
		}
	}
}
