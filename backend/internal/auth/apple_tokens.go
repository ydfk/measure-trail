package auth

import (
	"context"
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/ydfk/measure-trail/backend/internal/config"
)

type AppleTokenSet struct {
	RefreshToken string
}

type AppleTokenClient interface {
	Exchange(context.Context, string) (AppleTokenSet, error)
	Revoke(context.Context, string) error
}

func NewAppleTokenClient(appleConfig config.Apple) AppleTokenClient {
	client, err := newAppleTokenClient(appleConfig)
	if err != nil {
		return unavailableAppleTokenClient{}
	}
	return client
}

type unavailableAppleTokenClient struct{}

func (unavailableAppleTokenClient) Exchange(context.Context, string) (AppleTokenSet, error) {
	return AppleTokenSet{}, ErrAppleUnavailable
}

func (unavailableAppleTokenClient) Revoke(context.Context, string) error {
	return ErrAppleUnavailable
}

type appleTokenClient struct {
	teamID     string
	keyID      string
	clientID   string
	privateKey *ecdsa.PrivateKey
	tokenURL   string
	revokeURL  string
	client     *http.Client
	now        func() time.Time
}

func newAppleTokenClient(appleConfig config.Apple) (*appleTokenClient, error) {
	if appleConfig.TeamID == "" || appleConfig.KeyID == "" || appleConfig.ClientID == "" || appleConfig.PrivateKey == "" {
		return nil, ErrAppleUnavailable
	}
	privateKey, err := parseApplePrivateKey(appleConfig.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("解析 Apple 私钥: %w", err)
	}
	if appleConfig.TokenURL == "" || appleConfig.RevokeURL == "" {
		return nil, fmt.Errorf("Apple token 地址不能为空")
	}
	return &appleTokenClient{teamID: appleConfig.TeamID, keyID: appleConfig.KeyID, clientID: appleConfig.ClientID, privateKey: privateKey, tokenURL: appleConfig.TokenURL, revokeURL: appleConfig.RevokeURL, client: &http.Client{Timeout: 10 * time.Second}, now: time.Now}, nil
}

func (client *appleTokenClient) Exchange(ctx context.Context, authorizationCode string) (AppleTokenSet, error) {
	if strings.TrimSpace(authorizationCode) == "" {
		return AppleTokenSet{}, fmt.Errorf("Apple authorization code 不能为空")
	}
	values := url.Values{"client_id": {client.clientID}, "client_secret": {client.clientSecret()}, "code": {authorizationCode}, "grant_type": {"authorization_code"}}
	response, err := client.request(ctx, client.tokenURL, values)
	if err != nil {
		return AppleTokenSet{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return AppleTokenSet{}, fmt.Errorf("Apple code 兑换失败: HTTP %d", response.StatusCode)
	}
	var payload struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&payload); err != nil {
		return AppleTokenSet{}, fmt.Errorf("解析 Apple token 响应: %w", err)
	}
	if payload.RefreshToken == "" {
		return AppleTokenSet{}, fmt.Errorf("Apple token 响应缺少 refresh_token")
	}
	return AppleTokenSet{RefreshToken: payload.RefreshToken}, nil
}

func (client *appleTokenClient) Revoke(ctx context.Context, refreshToken string) error {
	if strings.TrimSpace(refreshToken) == "" {
		return fmt.Errorf("Apple refresh token 不能为空")
	}
	values := url.Values{"client_id": {client.clientID}, "client_secret": {client.clientSecret()}, "token": {refreshToken}, "token_type_hint": {"refresh_token"}}
	response, err := client.request(ctx, client.revokeURL, values)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("Apple token 撤销失败: HTTP %d", response.StatusCode)
	}
	return nil
}

func (client *appleTokenClient) clientSecret() string {
	now := client.now().UTC()
	token := jwt.NewWithClaims(jwt.SigningMethodES256, jwt.RegisteredClaims{Issuer: client.teamID, Subject: client.clientID, Audience: jwt.ClaimStrings{appleIssuer}, IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(5 * time.Minute))})
	token.Header["kid"] = client.keyID
	signed, err := token.SignedString(client.privateKey)
	if err != nil {
		return ""
	}
	return signed
}

func (client *appleTokenClient) request(ctx context.Context, endpoint string, values url.Values) (*http.Response, error) {
	if values.Get("client_secret") == "" {
		return nil, fmt.Errorf("生成 Apple client secret 失败")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(values.Encode()))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return client.client.Do(request)
}

func parseApplePrivateKey(value string) (*ecdsa.PrivateKey, error) {
	decoded, _ := pem.Decode([]byte(strings.ReplaceAll(value, `\n`, "\n")))
	if decoded == nil {
		return nil, fmt.Errorf("私钥不是 PEM")
	}
	key, err := x509.ParsePKCS8PrivateKey(decoded.Bytes)
	if err != nil {
		return nil, err
	}
	privateKey, ok := key.(*ecdsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("私钥不是 ECDSA")
	}
	return privateKey, nil
}
