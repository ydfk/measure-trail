package config

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"strings"
	"testing"
)

func TestLoadUsesEnvironmentOverrides(t *testing.T) {
	t.Setenv("MEASURETRAIL_API_PORT", "22000")
	t.Setenv("MEASURETRAIL_SQLITE_BUSY_TIMEOUT_MS", "9000")
	t.Setenv("MEASURETRAIL_CORS_ORIGINS", "https://web.measuretrail.example.com/, https://admin.measuretrail.example.com")

	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded.App.Port != "22000" {
		t.Fatalf("port = %q, want 22000", loaded.App.Port)
	}
	if loaded.Database.BusyTimeoutMS != 9000 {
		t.Fatalf("busy timeout = %d, want 9000", loaded.Database.BusyTimeoutMS)
	}
	if got, want := strings.Join(loaded.App.CORSOrigins, ","), "https://web.measuretrail.example.com,https://admin.measuretrail.example.com"; got != want {
		t.Fatalf("CORS origins = %q, want %q", got, want)
	}
}

func TestLoadRejectsInsecureProductionConfiguration(t *testing.T) {
	t.Setenv("MEASURETRAIL_ENV", "production")
	t.Setenv("MEASURETRAIL_PUBLIC_BASE_URL", "http://measuretrail.example.com")
	t.Setenv("MEASURETRAIL_JWT_ACCESS_SECRET", strings.Repeat("a", 32))
	t.Setenv("MEASURETRAIL_JWT_REFRESH_SECRET", strings.Repeat("b", 32))
	if _, err := Load(); err == nil {
		t.Fatal("HTTP 公网地址仍被生产配置接受")
	}
}

func TestLoadRejectsPartialAppleConfiguration(t *testing.T) {
	t.Setenv("MEASURETRAIL_ENV", "development")
	t.Setenv("MEASURETRAIL_APPLE_CLIENT_ID", "com.ydfk.MeasureTrail")

	if _, err := Load(); err == nil {
		t.Fatal("不完整的 Apple 配置仍被接受")
	}
}

func TestLoadRejectsMalformedApplePrivateKey(t *testing.T) {
	t.Setenv("MEASURETRAIL_APPLE_TEAM_ID", "TEAM123")
	t.Setenv("MEASURETRAIL_APPLE_KEY_ID", "KEY123")
	t.Setenv("MEASURETRAIL_APPLE_CLIENT_ID", "com.example.measuretrail")
	t.Setenv("MEASURETRAIL_APPLE_PRIVATE_KEY", "not-a-private-key")
	t.Setenv("MEASURETRAIL_APPLE_CREDENTIAL_ENCRYPTION_KEY", base64.RawStdEncoding.EncodeToString([]byte("01234567890123456789012345678901")))

	if _, err := Load(); err == nil {
		t.Fatal("畸形 Apple 私钥仍被接受")
	}
}

func TestLoadAcceptsPaddedAppleCredentialEncryptionKey(t *testing.T) {
	t.Setenv("MEASURETRAIL_APPLE_TEAM_ID", "TEAM123")
	t.Setenv("MEASURETRAIL_APPLE_KEY_ID", "KEY123")
	t.Setenv("MEASURETRAIL_APPLE_CLIENT_ID", "com.example.measuretrail")
	t.Setenv("MEASURETRAIL_APPLE_PRIVATE_KEY", validApplePrivateKey(t))
	t.Setenv("MEASURETRAIL_APPLE_CREDENTIAL_ENCRYPTION_KEY", base64.StdEncoding.EncodeToString([]byte("01234567890123456789012345678901")))

	if _, err := Load(); err != nil {
		t.Fatalf("带填充的 Apple 凭据加密密钥应被接受: %v", err)
	}
}

func TestLoadRejectsInsecureProductionAppleEndpoint(t *testing.T) {
	t.Setenv("MEASURETRAIL_ENV", "production")
	t.Setenv("MEASURETRAIL_PUBLIC_BASE_URL", "https://api.measuretrail.example.com")
	t.Setenv("MEASURETRAIL_CORS_ORIGINS", "https://web.measuretrail.example.com")
	t.Setenv("MEASURETRAIL_JWT_ACCESS_SECRET", strings.Repeat("a", 32))
	t.Setenv("MEASURETRAIL_JWT_REFRESH_SECRET", strings.Repeat("b", 32))
	t.Setenv("MEASURETRAIL_APPLE_TEAM_ID", "TEAM123")
	t.Setenv("MEASURETRAIL_APPLE_KEY_ID", "KEY123")
	t.Setenv("MEASURETRAIL_APPLE_CLIENT_ID", "com.example.measuretrail")
	t.Setenv("MEASURETRAIL_APPLE_PRIVATE_KEY", validApplePrivateKey(t))
	t.Setenv("MEASURETRAIL_APPLE_CREDENTIAL_ENCRYPTION_KEY", base64.RawStdEncoding.EncodeToString([]byte("01234567890123456789012345678901")))
	t.Setenv("MEASURETRAIL_APPLE_TOKEN_URL", "http://apple.example.com/auth/token")

	if _, err := Load(); err == nil {
		t.Fatal("生产环境 HTTP Apple 端点仍被接受")
	}
}

func TestLoadRejectsInsecureProductionCORSOrigin(t *testing.T) {
	t.Setenv("MEASURETRAIL_ENV", "production")
	t.Setenv("MEASURETRAIL_PUBLIC_BASE_URL", "https://api.measuretrail.example.com")
	t.Setenv("MEASURETRAIL_CORS_ORIGINS", "http://web.measuretrail.example.com")
	t.Setenv("MEASURETRAIL_JWT_ACCESS_SECRET", strings.Repeat("a", 32))
	t.Setenv("MEASURETRAIL_JWT_REFRESH_SECRET", strings.Repeat("b", 32))
	if _, err := Load(); err == nil {
		t.Fatal("生产环境 HTTP CORS origin 仍被接受")
	}
}

func validApplePrivateKey(t *testing.T) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("生成 Apple 测试私钥: %v", err)
	}
	encoded, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("编码 Apple 测试私钥: %v", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: encoded}))
}

func TestDefaultCredentialsCanBeOverridden(t *testing.T) {
	t.Setenv("MEASURETRAIL_DEFAULT_USERNAME", "operator")
	t.Setenv("MEASURETRAIL_DEFAULT_PASSWORD", "654321")
	loaded, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Auth.DefaultUsername != "operator" || loaded.Auth.DefaultPassword != "654321" {
		t.Fatalf("默认凭证未读取环境变量: %#v", loaded.Auth)
	}
	t.Setenv("MEASURETRAIL_DEFAULT_PASSWORD", "123")
	if _, err := Load(); err == nil {
		t.Fatal("过短默认密码应返回配置错误")
	}
}

func TestLoadDerivesPasskeyRelyingPartyFromPublicURL(t *testing.T) {
	t.Setenv("MEASURETRAIL_PUBLIC_BASE_URL", "https://measure-trail.ydfk.site/")
	loaded, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Passkey.RPID != "measure-trail.ydfk.site" || strings.Join(loaded.Passkey.Origins, ",") != "https://measure-trail.ydfk.site" {
		t.Fatalf("Passkey 配置未从公网地址派生: %#v", loaded.Passkey)
	}
}

func TestLoadRejectsInvalidPasskeyConfiguration(t *testing.T) {
	t.Setenv("MEASURETRAIL_PASSKEY_RP_ID", "https://measure-trail.ydfk.site")
	if _, err := Load(); err == nil {
		t.Fatal("带协议的 Passkey RP ID 应被拒绝")
	}
	t.Setenv("MEASURETRAIL_PASSKEY_RP_ID", "measure-trail.ydfk.site")
	t.Setenv("MEASURETRAIL_PASSKEY_CREDENTIAL_ENCRYPTION_KEY", "too-short")
	if _, err := Load(); err == nil {
		t.Fatal("无效 Passkey 加密密钥应被拒绝")
	}
}

func TestLoadAcceptsProductionPasskeyConfiguration(t *testing.T) {
	t.Setenv("MEASURETRAIL_ENV", "production")
	t.Setenv("MEASURETRAIL_PUBLIC_BASE_URL", "https://measure-trail.ydfk.site")
	t.Setenv("MEASURETRAIL_JWT_ACCESS_SECRET", strings.Repeat("a", 32))
	t.Setenv("MEASURETRAIL_JWT_REFRESH_SECRET", strings.Repeat("b", 32))
	t.Setenv("MEASURETRAIL_PASSKEY_CREDENTIAL_ENCRYPTION_KEY", base64.RawStdEncoding.EncodeToString([]byte("01234567890123456789012345678901")))
	t.Setenv("MEASURETRAIL_IOS_APP_ID", "TEAM123.com.ydfk.MeasureTrail")

	loaded, err := Load()
	if err != nil {
		t.Fatalf("有效生产 Passkey 配置被拒绝: %v", err)
	}
	if loaded.Passkey.RPID != "measure-trail.ydfk.site" || loaded.Passkey.IOSAppID != "TEAM123.com.ydfk.MeasureTrail" {
		t.Fatalf("生产 Passkey 配置错误: %#v", loaded.Passkey)
	}
}
