package auth

import (
	"context"
	"crypto/rsa"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/ydfk/measure-trail/backend/internal/config"
)

const appleIssuer = "https://appleid.apple.com"

var (
	ErrAppleUnavailable         = fmt.Errorf("Sign in with Apple 尚未配置")
	ErrAppleAccountLinkRequired = fmt.Errorf("此 Apple 账号需要先登录已有量迹账号后绑定")
	ErrAppleIdentityLinked      = fmt.Errorf("此 Apple 账号已绑定到其他量迹账号")
	ErrAppleEmailRequired       = fmt.Errorf("Apple 未提供可用邮箱，无法创建量迹账号")
)

type AppleIdentity struct {
	Subject string
	Email   string
}

type AppleVerifier interface {
	Verify(context.Context, string, string) (AppleIdentity, error)
}

func NewAppleVerifier(appleConfig config.Apple) AppleVerifier {
	if appleConfig.ClientID == "" {
		return unavailableAppleVerifier{}
	}
	return &appleVerifier{
		clientID: appleConfig.ClientID,
		jwksURL:  appleConfig.JWKSURL,
		client:   &http.Client{Timeout: 10 * time.Second},
	}
}

type unavailableAppleVerifier struct{}

func (unavailableAppleVerifier) Verify(context.Context, string, string) (AppleIdentity, error) {
	return AppleIdentity{}, ErrAppleUnavailable
}

type appleVerifier struct {
	clientID string
	jwksURL  string
	client   *http.Client
	mu       sync.Mutex
	keys     map[string]*rsa.PublicKey
	expires  time.Time
}

type appleClaims struct {
	Email string `json:"email"`
	Nonce string `json:"nonce"`
	jwt.RegisteredClaims
}

func (verifier *appleVerifier) Verify(ctx context.Context, rawToken string, nonce string) (AppleIdentity, error) {
	if rawToken == "" || nonce == "" {
		return AppleIdentity{}, fmt.Errorf("Apple 凭据不完整")
	}
	claims := &appleClaims{}
	parsed, err := jwt.ParseWithClaims(rawToken, claims, func(token *jwt.Token) (any, error) {
		if token.Method.Alg() != jwt.SigningMethodRS256.Alg() {
			return nil, fmt.Errorf("不允许的 Apple token 签名算法")
		}
		keyID, _ := token.Header["kid"].(string)
		return verifier.key(ctx, keyID)
	}, jwt.WithIssuer(appleIssuer), jwt.WithAudience(verifier.clientID), jwt.WithValidMethods([]string{jwt.SigningMethodRS256.Alg()}))
	if err != nil || !parsed.Valid || claims.Subject == "" {
		return AppleIdentity{}, fmt.Errorf("Apple identity token 无效")
	}
	if subtle.ConstantTimeCompare([]byte(claims.Nonce), []byte(nonce)) != 1 {
		return AppleIdentity{}, fmt.Errorf("Apple nonce 不匹配")
	}
	return AppleIdentity{Subject: claims.Subject, Email: normalizeEmail(claims.Email)}, nil
}

func (verifier *appleVerifier) key(ctx context.Context, keyID string) (*rsa.PublicKey, error) {
	if keyID == "" {
		return nil, fmt.Errorf("Apple token 缺少 kid")
	}
	verifier.mu.Lock()
	defer verifier.mu.Unlock()
	if time.Now().Before(verifier.expires) {
		if key := verifier.keys[keyID]; key != nil {
			return key, nil
		}
	}
	if err := verifier.refreshKeys(ctx); err != nil {
		return nil, err
	}
	key := verifier.keys[keyID]
	if key == nil {
		return nil, fmt.Errorf("Apple 未发布匹配的签名密钥")
	}
	return key, nil
}

func (verifier *appleVerifier) refreshKeys(ctx context.Context) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, verifier.jwksURL, nil)
	if err != nil {
		return err
	}
	response, err := verifier.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("获取 Apple 签名密钥失败: HTTP %d", response.StatusCode)
	}
	var document struct {
		Keys []struct {
			KeyID string `json:"kid"`
			KTY   string `json:"kty"`
			N     string `json:"n"`
			E     string `json:"e"`
		} `json:"keys"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&document); err != nil {
		return err
	}
	keys := make(map[string]*rsa.PublicKey, len(document.Keys))
	for _, item := range document.Keys {
		if item.KTY != "RSA" || item.KeyID == "" {
			continue
		}
		key, err := rsaKey(item.N, item.E)
		if err != nil {
			continue
		}
		keys[item.KeyID] = key
	}
	if len(keys) == 0 {
		return fmt.Errorf("Apple 签名密钥为空")
	}
	verifier.keys = keys
	verifier.expires = time.Now().Add(time.Hour)
	return nil
}

func rsaKey(modulus string, exponent string) (*rsa.PublicKey, error) {
	n, err := base64.RawURLEncoding.DecodeString(modulus)
	if err != nil {
		return nil, err
	}
	e, err := base64.RawURLEncoding.DecodeString(exponent)
	if err != nil {
		return nil, err
	}
	value := 0
	for _, byteValue := range e {
		value = value<<8 | int(byteValue)
	}
	if len(n) == 0 || value < 3 || value%2 == 0 {
		return nil, fmt.Errorf("无效 RSA 公钥")
	}
	return &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: value}, nil
}
