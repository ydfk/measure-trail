package config

import (
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	App      App
	Database Database
	Auth     Auth
	Mail     Mail
	Apple    Apple
}

type App struct {
	Port          string
	Environment   string
	PublicBaseURL string
	CORSOrigins   []string
}

type Database struct {
	Path          string
	BusyTimeoutMS int
}

type Auth struct {
	Issuer        string
	Audience      string
	AccessSecret  string
	RefreshSecret string
}

type Mail struct {
	Mode     string
	Host     string
	Port     string
	Username string
	Password string
	From     string
}

type Apple struct {
	TeamID                  string
	KeyID                   string
	ClientID                string
	PrivateKey              string
	CredentialEncryptionKey string
	JWKSURL                 string
	TokenURL                string
	RevokeURL               string
}

func Load() (Config, error) {
	environment := value("MEASURETRAIL_ENV", "development")
	publicBaseURL := value("MEASURETRAIL_PUBLIC_BASE_URL", "http://localhost:21000")
	corsOrigins, err := parseCORSOrigins(value("MEASURETRAIL_CORS_ORIGINS", publicBaseURL), strings.EqualFold(environment, "production"))
	if err != nil {
		return Config{}, err
	}
	config := Config{
		App: App{
			Port:          value("MEASURETRAIL_API_PORT", "21000"),
			Environment:   environment,
			PublicBaseURL: publicBaseURL,
			CORSOrigins:   corsOrigins,
		},
		Database: Database{
			Path: value("MEASURETRAIL_SQLITE_PATH", "data/measuretrail.sqlite"),
		},
		Auth: Auth{
			Issuer:        value("MEASURETRAIL_JWT_ISSUER", "measuretrail"),
			Audience:      value("MEASURETRAIL_JWT_AUDIENCE", "measuretrail-ios"),
			AccessSecret:  strings.TrimSpace(os.Getenv("MEASURETRAIL_JWT_ACCESS_SECRET")),
			RefreshSecret: strings.TrimSpace(os.Getenv("MEASURETRAIL_JWT_REFRESH_SECRET")),
		},
		Mail: Mail{
			Mode:     value("MEASURETRAIL_MAIL_MODE", "log"),
			Host:     strings.TrimSpace(os.Getenv("MEASURETRAIL_SMTP_HOST")),
			Port:     value("MEASURETRAIL_SMTP_PORT", "587"),
			Username: strings.TrimSpace(os.Getenv("MEASURETRAIL_SMTP_USERNAME")),
			Password: strings.TrimSpace(os.Getenv("MEASURETRAIL_SMTP_PASSWORD")),
			From:     strings.TrimSpace(os.Getenv("MEASURETRAIL_SMTP_FROM")),
		},
		Apple: Apple{
			TeamID:                  strings.TrimSpace(os.Getenv("MEASURETRAIL_APPLE_TEAM_ID")),
			KeyID:                   strings.TrimSpace(os.Getenv("MEASURETRAIL_APPLE_KEY_ID")),
			ClientID:                strings.TrimSpace(os.Getenv("MEASURETRAIL_APPLE_CLIENT_ID")),
			PrivateKey:              strings.TrimSpace(os.Getenv("MEASURETRAIL_APPLE_PRIVATE_KEY")),
			CredentialEncryptionKey: strings.TrimSpace(os.Getenv("MEASURETRAIL_APPLE_CREDENTIAL_ENCRYPTION_KEY")),
			JWKSURL:                 value("MEASURETRAIL_APPLE_JWKS_URL", "https://appleid.apple.com/auth/keys"),
			TokenURL:                value("MEASURETRAIL_APPLE_TOKEN_URL", "https://appleid.apple.com/auth/token"),
			RevokeURL:               value("MEASURETRAIL_APPLE_REVOKE_URL", "https://appleid.apple.com/auth/revoke"),
		},
	}

	busyTimeout, err := strconv.Atoi(value("MEASURETRAIL_SQLITE_BUSY_TIMEOUT_MS", "5000"))
	if err != nil || busyTimeout < 1 {
		return Config{}, fmt.Errorf("MEASURETRAIL_SQLITE_BUSY_TIMEOUT_MS 必须是正整数")
	}
	config.Database.BusyTimeoutMS = busyTimeout
	if strings.EqualFold(config.App.Environment, "production") {
		if err := validateProduction(config); err != nil {
			return Config{}, err
		}
	}
	if err := validateApple(config.Apple, strings.EqualFold(config.App.Environment, "production")); err != nil {
		return Config{}, err
	}
	return config, nil
}

func validateProduction(config Config) error {
	publicURL, err := url.ParseRequestURI(config.App.PublicBaseURL)
	if err != nil || publicURL.Scheme != "https" || publicURL.Host == "" || publicURL.User != nil || publicURL.RawQuery != "" || publicURL.Fragment != "" || (publicURL.Path != "" && publicURL.Path != "/") {
		return fmt.Errorf("生产环境 MEASURETRAIL_PUBLIC_BASE_URL 必须是 HTTPS 基础地址")
	}
	if invalidProductionSecret(config.Auth.AccessSecret) || invalidProductionSecret(config.Auth.RefreshSecret) {
		return fmt.Errorf("生产环境 JWT secret 必须是至少 32 个字符的非占位值")
	}
	if !strings.EqualFold(config.Mail.Mode, "smtp") || config.Mail.Host == "" || config.Mail.From == "" {
		return fmt.Errorf("生产环境必须配置 SMTP host、from 与 smtp 邮件模式")
	}
	return nil
}

func invalidProductionSecret(value string) bool {
	return len(value) < 32 || strings.EqualFold(value, "CHANGE_ME_BEFORE_DEPLOYMENT")
}

func parseCORSOrigins(value string, requireHTTPS bool) ([]string, error) {
	origins := make([]string, 0)
	seen := map[string]struct{}{}
	for _, candidate := range strings.Split(value, ",") {
		candidate = strings.TrimSpace(candidate)
		origin, err := url.ParseRequestURI(candidate)
		if err != nil || origin.Host == "" || origin.User != nil || origin.RawQuery != "" || origin.Fragment != "" || (origin.Path != "" && origin.Path != "/") || (origin.Scheme != "http" && origin.Scheme != "https") || (requireHTTPS && origin.Scheme != "https") {
			return nil, fmt.Errorf("MEASURETRAIL_CORS_ORIGINS 必须是%s基础 origin 的逗号分隔列表", map[bool]string{true: " HTTPS", false: " HTTP 或 HTTPS"}[requireHTTPS])
		}
		normalized := origin.Scheme + "://" + origin.Host
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		origins = append(origins, normalized)
	}
	if len(origins) == 0 {
		return nil, fmt.Errorf("MEASURETRAIL_CORS_ORIGINS 至少需要一个 origin")
	}
	return origins, nil
}

func validateApple(apple Apple, requireHTTPS bool) error {
	values := []string{apple.TeamID, apple.KeyID, apple.ClientID, apple.PrivateKey, apple.CredentialEncryptionKey}
	configured := false
	for _, value := range values {
		configured = configured || value != ""
	}
	if !configured {
		return nil
	}
	for _, value := range values {
		if value == "" {
			return fmt.Errorf("Sign in with Apple 配置必须包含 team ID、key ID、client ID、私钥和凭据加密密钥")
		}
	}
	key, err := decodeCredentialEncryptionKey(apple.CredentialEncryptionKey)
	if err != nil || len(key) != 32 {
		return fmt.Errorf("MEASURETRAIL_APPLE_CREDENTIAL_ENCRYPTION_KEY 必须是 32-byte base64")
	}
	if err := validateApplePrivateKey(apple.PrivateKey); err != nil {
		return err
	}
	if requireHTTPS {
		for name, endpoint := range map[string]string{
			"MEASURETRAIL_APPLE_JWKS_URL":   apple.JWKSURL,
			"MEASURETRAIL_APPLE_TOKEN_URL":  apple.TokenURL,
			"MEASURETRAIL_APPLE_REVOKE_URL": apple.RevokeURL,
		} {
			if err := validateHTTPSURL(name, endpoint); err != nil {
				return err
			}
		}
	}
	return nil
}

func decodeCredentialEncryptionKey(value string) ([]byte, error) {
	key, err := base64.RawStdEncoding.DecodeString(value)
	if err == nil {
		return key, nil
	}
	return base64.StdEncoding.DecodeString(value)
}

func validateApplePrivateKey(value string) error {
	block, _ := pem.Decode([]byte(strings.ReplaceAll(value, `\n`, "\n")))
	if block == nil {
		return fmt.Errorf("MEASURETRAIL_APPLE_PRIVATE_KEY 必须是 PEM 格式的 P-256 私钥")
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return fmt.Errorf("MEASURETRAIL_APPLE_PRIVATE_KEY 无法解析: %w", err)
	}
	privateKey, ok := key.(*ecdsa.PrivateKey)
	if !ok || privateKey.Curve.Params().Name != "P-256" {
		return fmt.Errorf("MEASURETRAIL_APPLE_PRIVATE_KEY 必须是 P-256 私钥")
	}
	return nil
}

func validateHTTPSURL(name string, value string) error {
	endpoint, err := url.ParseRequestURI(value)
	if err != nil || endpoint.Scheme != "https" || endpoint.Host == "" || endpoint.User != nil || endpoint.RawQuery != "" || endpoint.Fragment != "" {
		return fmt.Errorf("生产环境 %s 必须是 HTTPS 地址", name)
	}
	return nil
}

func value(name string, fallback string) string {
	if candidate := strings.TrimSpace(os.Getenv(name)); candidate != "" {
		return candidate
	}
	return fallback
}
