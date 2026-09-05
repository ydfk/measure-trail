package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/ydfk/measure-trail/backend/internal/config"
	"github.com/ydfk/measure-trail/backend/internal/database"
)

func TestAppleVerifierChecksSignatureClaimsAndNonce(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("生成测试密钥: %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(response).Encode(map[string]any{"keys": []map[string]string{{
			"kid": "test-key",
			"kty": "RSA",
			"n":   base64.RawURLEncoding.EncodeToString(privateKey.PublicKey.N.Bytes()),
			"e":   base64.RawURLEncoding.EncodeToString(exponentBytes(privateKey.PublicKey.E)),
		}}})
	}))
	defer server.Close()

	verifier := &appleVerifier{clientID: "com.example.measuretrail", jwksURL: server.URL, client: server.Client()}
	token := signedAppleToken(t, privateKey, "test-key", "com.example.measuretrail", "nonce-value-123456", "apple-subject", "apple@example.com")
	identity, err := verifier.Verify(context.Background(), token, "nonce-value-123456")
	if err != nil {
		t.Fatalf("验证 Apple token: %v", err)
	}
	if identity.Subject != "apple-subject" || identity.Email != "apple@example.com" {
		t.Fatalf("identity = %#v", identity)
	}
	if _, err := verifier.Verify(context.Background(), token, "wrong-nonce-value"); err == nil {
		t.Fatal("nonce 不匹配仍通过验证")
	}
	wrongAudience := signedAppleToken(t, privateKey, "test-key", "other-client", "nonce-value-123456", "apple-subject", "apple@example.com")
	if _, err := verifier.Verify(context.Background(), wrongAudience, "nonce-value-123456"); err == nil {
		t.Fatal("audience 不匹配仍通过验证")
	}
}

func TestAppleSignInConsumesServerNonce(t *testing.T) {
	db, err := database.Open(config.Database{Path: t.TempDir() + "/measuretrail.sqlite", BusyTimeoutMS: 1000})
	if err != nil {
		t.Fatalf("打开测试数据库: %v", err)
	}
	service, err := NewService(db, config.Auth{RegistrationEnabled: true, Issuer: "measuretrail", Audience: "measuretrail-ios", AccessSecret: "01234567890123456789012345678901", RefreshSecret: "abcdefghijklmnopqrstuvwxyzABCDEF"})
	if err != nil {
		t.Fatalf("创建认证服务: %v", err)
	}
	if err := service.ConfigureAppleCredentials(base64.RawStdEncoding.EncodeToString([]byte("01234567890123456789012345678901"))); err != nil {
		t.Fatalf("配置 Apple 凭据存储: %v", err)
	}
	nonce, err := service.NewAppleNonce()
	if err != nil {
		t.Fatalf("创建 nonce: %v", err)
	}
	verifier := fixedAppleVerifier{identity: AppleIdentity{Subject: "apple-subject", Email: "apple@example.com"}}
	if _, err := service.SignInWithApple(context.Background(), verifier, fixedAppleTokenClient{}, "identity-token", "authorization-code", nonce, "iPhone"); err != nil {
		t.Fatalf("Apple 登录: %v", err)
	}
	if _, err := service.SignInWithApple(context.Background(), verifier, fixedAppleTokenClient{}, "identity-token", "authorization-code", nonce, "iPhone"); err == nil {
		t.Fatal("已使用 nonce 仍可登录")
	}
}

func TestAppleSignInStoresCredentialAndAccountDeletionRevokesIt(t *testing.T) {
	db, err := database.Open(config.Database{Path: t.TempDir() + "/measuretrail.sqlite", BusyTimeoutMS: 1000})
	if err != nil {
		t.Fatalf("打开测试数据库: %v", err)
	}
	service, err := NewService(db, config.Auth{RegistrationEnabled: true, Issuer: "measuretrail", Audience: "measuretrail-ios", AccessSecret: "01234567890123456789012345678901", RefreshSecret: "abcdefghijklmnopqrstuvwxyzABCDEF"})
	if err != nil {
		t.Fatalf("创建认证服务: %v", err)
	}
	if err := service.ConfigureAppleCredentials(base64.RawStdEncoding.EncodeToString([]byte("01234567890123456789012345678901"))); err != nil {
		t.Fatalf("配置 Apple 凭据存储: %v", err)
	}
	nonce, err := service.NewAppleNonce()
	if err != nil {
		t.Fatalf("创建 nonce: %v", err)
	}
	tokenClient := &recordingAppleTokenClient{refreshToken: "apple-refresh-token"}
	session, err := service.SignInWithApple(context.Background(), fixedAppleVerifier{identity: AppleIdentity{Subject: "apple-subject", Email: "apple@example.com"}}, tokenClient, "identity-token", "authorization-code", nonce, "iPhone")
	if err != nil {
		t.Fatalf("Apple 登录: %v", err)
	}
	userID, err := service.UserIDFromAccessToken(session.AccessToken)
	if err != nil {
		t.Fatalf("解析 access token: %v", err)
	}
	storedToken, err := service.appleCredentials.RefreshToken(userID)
	if err != nil || storedToken != tokenClient.refreshToken {
		t.Fatalf("保存的 Apple refresh token = %q, error = %v", storedToken, err)
	}
	if err := service.DeleteAccount(context.Background(), userID, tokenClient); err != nil {
		t.Fatalf("删除 Apple 账号: %v", err)
	}
	if len(tokenClient.revokedTokens) != 1 || tokenClient.revokedTokens[0] != tokenClient.refreshToken {
		t.Fatalf("撤销的 Apple token = %#v", tokenClient.revokedTokens)
	}
	var count int
	if err := db.Raw("SELECT COUNT(*) FROM users WHERE id = ?", userID).Row().Scan(&count); err != nil || count != 0 {
		t.Fatalf("删除后的用户数量 = %d, error = %v", count, err)
	}
}

type fixedAppleVerifier struct {
	identity AppleIdentity
	err      error
}

type fixedAppleTokenClient struct{}

func (fixedAppleTokenClient) Exchange(context.Context, string) (AppleTokenSet, error) {
	return AppleTokenSet{RefreshToken: "apple-refresh-token"}, nil
}

func (fixedAppleTokenClient) Revoke(context.Context, string) error { return nil }

type recordingAppleTokenClient struct {
	refreshToken  string
	revokedTokens []string
}

func (client *recordingAppleTokenClient) Exchange(context.Context, string) (AppleTokenSet, error) {
	return AppleTokenSet{RefreshToken: client.refreshToken}, nil
}

func (client *recordingAppleTokenClient) Revoke(_ context.Context, refreshToken string) error {
	client.revokedTokens = append(client.revokedTokens, refreshToken)
	return nil
}

func (verifier fixedAppleVerifier) Verify(context.Context, string, string) (AppleIdentity, error) {
	return verifier.identity, verifier.err
}

func signedAppleToken(t *testing.T, privateKey *rsa.PrivateKey, keyID string, audience string, nonce string, subject string, email string) string {
	t.Helper()
	now := time.Now()
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, appleClaims{
		Email: email,
		Nonce: nonce,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    appleIssuer,
			Subject:   subject,
			Audience:  jwt.ClaimStrings{audience},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Minute)),
		},
	})
	token.Header["kid"] = keyID
	signed, err := token.SignedString(privateKey)
	if err != nil {
		t.Fatalf("签名测试 token: %v", err)
	}
	return signed
}

func exponentBytes(exponent int) []byte {
	value := big.NewInt(int64(exponent)).Bytes()
	return value
}

func TestClosedRegistrationAllowsExistingAppleIdentityOnly(t *testing.T) {
	db, err := database.Open(config.Database{Path: t.TempDir() + "/measuretrail.sqlite", BusyTimeoutMS: 1000})
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewService(db, config.Auth{RegistrationEnabled: true, Issuer: "measuretrail", Audience: "measuretrail-ios", AccessSecret: "01234567890123456789012345678901", RefreshSecret: "abcdefghijklmnopqrstuvwxyzABCDEF"})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.ConfigureAppleCredentials(base64.RawStdEncoding.EncodeToString([]byte("01234567890123456789012345678901"))); err != nil {
		t.Fatal(err)
	}
	signIn := func(subject, email string) error {
		nonce, err := service.NewAppleNonce()
		if err != nil {
			return err
		}
		_, err = service.SignInWithApple(context.Background(), fixedAppleVerifier{identity: AppleIdentity{Subject: subject, Email: email}}, fixedAppleTokenClient{}, "identity-token", "authorization-code", nonce, "iPhone")
		return err
	}
	if err := signIn("existing-apple", "existing@example.com"); err != nil {
		t.Fatal(err)
	}
	service.registrationEnabled = false
	if err := signIn("existing-apple", "existing@example.com"); err != nil {
		t.Fatalf("已有 Apple 账号无法登录: %v", err)
	}
	if err := signIn("new-apple", "new@example.com"); !errors.Is(err, ErrRegistrationClosed) {
		t.Fatalf("新 Apple 账号未被拒绝: %v", err)
	}
	var count int64
	if err := db.Table("users").Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("账号数=%d, error=%v", count, err)
	}
}
