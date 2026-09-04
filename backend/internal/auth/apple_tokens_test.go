package auth

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/ydfk/measure-trail/backend/internal/config"
)

func TestAppleTokenClientExchangesAndRevokesTokens(t *testing.T) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("生成测试私钥: %v", err)
	}
	privateKeyBytes, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		t.Fatalf("编码测试私钥: %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if err := request.ParseForm(); err != nil {
			t.Fatalf("解析表单: %v", err)
		}
		verifyAppleClientSecret(t, request.Form, privateKey)
		switch request.URL.Path {
		case "/token":
			if request.Form.Get("grant_type") != "authorization_code" || request.Form.Get("code") != "authorization-code" {
				t.Fatalf("兑换表单 = %#v", request.Form)
			}
			_, _ = response.Write([]byte(`{"refresh_token":"apple-refresh-token"}`))
		case "/revoke":
			if request.Form.Get("token") != "apple-refresh-token" || request.Form.Get("token_type_hint") != "refresh_token" {
				t.Fatalf("撤销表单 = %#v", request.Form)
			}
			response.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("意外路径: %s", request.URL.Path)
		}
	}))
	defer server.Close()

	client, err := newAppleTokenClient(config.Apple{TeamID: "TEAM123", KeyID: "KEY123", ClientID: "com.example.measuretrail", PrivateKey: string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateKeyBytes})), TokenURL: server.URL + "/token", RevokeURL: server.URL + "/revoke"})
	if err != nil {
		t.Fatalf("创建 Apple token client: %v", err)
	}
	tokens, err := client.Exchange(context.Background(), "authorization-code")
	if err != nil || tokens.RefreshToken != "apple-refresh-token" {
		t.Fatalf("兑换结果 = %#v, error = %v", tokens, err)
	}
	if err := client.Revoke(context.Background(), tokens.RefreshToken); err != nil {
		t.Fatalf("撤销 Apple token: %v", err)
	}
}

func verifyAppleClientSecret(t *testing.T, values url.Values, privateKey *ecdsa.PrivateKey) {
	t.Helper()
	if values.Get("client_id") != "com.example.measuretrail" || values.Get("client_secret") == "" {
		t.Fatalf("client 凭据缺失: %#v", values)
	}
	claims := &jwt.RegisteredClaims{}
	token, err := jwt.ParseWithClaims(values.Get("client_secret"), claims, func(token *jwt.Token) (any, error) {
		if token.Method.Alg() != jwt.SigningMethodES256.Alg() || token.Header["kid"] != "KEY123" {
			t.Fatalf("Apple client secret header = %#v", token.Header)
		}
		return &privateKey.PublicKey, nil
	})
	if err != nil || !token.Valid || claims.Issuer != "TEAM123" || claims.Subject != "com.example.measuretrail" || len(claims.Audience) != 1 || claims.Audience[0] != appleIssuer {
		t.Fatalf("Apple client secret claims = %#v, error = %v", claims, err)
	}
}
