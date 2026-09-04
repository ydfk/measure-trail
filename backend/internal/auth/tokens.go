package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/ydfk/measure-trail/backend/internal/config"
)

const (
	accessTokenLifetime  = 15 * time.Minute
	refreshTokenLifetime = 30 * 24 * time.Hour
	verifyTokenLifetime  = 24 * time.Hour
	resetTokenLifetime   = 30 * time.Minute
	appleNonceLifetime   = 10 * time.Minute
)

type accessClaims struct {
	jwt.RegisteredClaims
}

type tokenManager struct {
	issuer       string
	audience     string
	accessSecret []byte
}

func newTokenManager(authConfig config.Auth) (*tokenManager, error) {
	if len(authConfig.AccessSecret) < 32 || len(authConfig.RefreshSecret) < 32 {
		return nil, fmt.Errorf("JWT access 与 refresh secret 均至少需要 32 个字符")
	}
	return &tokenManager{issuer: authConfig.Issuer, audience: authConfig.Audience, accessSecret: []byte(authConfig.AccessSecret)}, nil
}

func (manager *tokenManager) newAccessToken(userID string, now time.Time) (string, error) {
	claims := accessClaims{RegisteredClaims: jwt.RegisteredClaims{
		Issuer:    manager.issuer,
		Subject:   userID,
		Audience:  jwt.ClaimStrings{manager.audience},
		ExpiresAt: jwt.NewNumericDate(now.Add(accessTokenLifetime)),
		NotBefore: jwt.NewNumericDate(now),
		IssuedAt:  jwt.NewNumericDate(now),
		ID:        uuid.NewString(),
	}}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(manager.accessSecret)
}

func (manager *tokenManager) verifyAccessToken(raw string) (string, error) {
	claims := &accessClaims{}
	parsed, err := jwt.ParseWithClaims(raw, claims, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("不允许的 JWT 签名算法")
		}
		return manager.accessSecret, nil
	}, jwt.WithIssuer(manager.issuer), jwt.WithAudience(manager.audience))
	if err != nil || !parsed.Valid || claims.Subject == "" {
		return "", fmt.Errorf("无效 access token")
	}
	return claims.Subject, nil
}

func newOpaqueToken() (raw string, hash string, err error) {
	bytes := make([]byte, 32)
	if _, err = rand.Read(bytes); err != nil {
		return "", "", fmt.Errorf("生成随机 token: %w", err)
	}
	raw = base64.RawURLEncoding.EncodeToString(bytes)
	return raw, hashToken(raw), nil
}

func hashToken(raw string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(raw)))
}
