package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
)

type credentialsInput struct {
	Body struct {
		Email       string `json:"email" format:"email" required:"true"`
		Password    string `json:"password" minLength:"12" required:"true"`
		DeviceLabel string `json:"deviceLabel" maxLength:"128"`
	}
}

type verifyInput struct {
	Body struct {
		Token string `json:"token" minLength:"20" required:"true"`
	}
}

type refreshInput struct {
	Body struct {
		RefreshToken string `json:"refreshToken" minLength:"20" required:"true"`
		DeviceLabel  string `json:"deviceLabel" maxLength:"128"`
	}
}

type emailInput struct {
	Body struct {
		Email string `json:"email" format:"email" required:"true"`
	}
}

type resetPasswordInput struct {
	Body struct {
		Token    string `json:"token" minLength:"20" required:"true"`
		Password string `json:"password" minLength:"12" required:"true"`
	}
}

type appleCredentialInput struct {
	Authorization string `header:"Authorization"`
	Body          struct {
		IdentityToken     string `json:"identityToken" minLength:"20" maxLength:"16384" required:"true"`
		AuthorizationCode string `json:"authorizationCode" minLength:"8" maxLength:"16384" required:"true"`
		Nonce             string `json:"nonce" minLength:"16" maxLength:"256" required:"true"`
		DeviceLabel       string `json:"deviceLabel" maxLength:"128"`
	}
}

type appleNonceOutput struct {
	Body struct {
		Nonce string `json:"nonce"`
	}
}

type accessTokenInput struct {
	Authorization string `header:"Authorization"`
}

type revokeSessionInput struct {
	Authorization string `header:"Authorization"`
	SessionID     string `path:"id" required:"true"`
}

type sessionsOutput struct {
	Body struct {
		Sessions []sessionInfoOutput `json:"sessions"`
	}
}

type sessionInfoOutput struct {
	ID          string `json:"id"`
	DeviceLabel string `json:"deviceLabel"`
	CreatedAt   string `json:"createdAt"`
	ExpiresAt   string `json:"expiresAt"`
}

type sessionOutput struct {
	Body struct {
		AccessToken  string `json:"accessToken"`
		RefreshToken string `json:"refreshToken"`
		ExpiresIn    int    `json:"expiresIn"`
	}
}

func RegisterRoutes(api huma.API, service *Service, notifier Notifier, appleVerifier AppleVerifier, appleTokenClient AppleTokenClient) {
	huma.Register(api, huma.Operation{OperationID: "register", Method: http.MethodPost, Path: "/api/v1/auth/register", Summary: "注册账号", Tags: []string{"认证"}}, func(ctx context.Context, input *credentialsInput) (*struct{}, error) {
		verificationToken, err := service.Register(input.Body.Email, input.Body.Password)
		if err != nil {
			return nil, huma.Error400BadRequest("无法创建账号")
		}
		if err := notifier.SendVerification(ctx, input.Body.Email, verificationToken); err != nil {
			return nil, huma.Error503ServiceUnavailable("验证邮件暂时无法发送")
		}
		return &struct{}{}, nil
	})
	huma.Register(api, huma.Operation{OperationID: "verify-email", Method: http.MethodPost, Path: "/api/v1/auth/verify-email", Summary: "验证邮箱", Tags: []string{"认证"}}, func(_ context.Context, input *verifyInput) (*struct{}, error) {
		if err := service.VerifyEmail(input.Body.Token); err != nil {
			return nil, huma.Error400BadRequest("验证链接无效或已过期")
		}
		return &struct{}{}, nil
	})
	huma.Register(api, huma.Operation{OperationID: "resend-verification", Method: http.MethodPost, Path: "/api/v1/auth/resend-verification", Summary: "重发验证邮件", Tags: []string{"认证"}}, func(ctx context.Context, input *emailInput) (*struct{}, error) {
		token, err := service.ResendVerification(input.Body.Email)
		if err != nil {
			return nil, huma.Error500InternalServerError("无法发送验证邮件")
		}
		if token != "" {
			if err := notifier.SendVerification(ctx, input.Body.Email, token); err != nil {
				return nil, huma.Error503ServiceUnavailable("验证邮件暂时无法发送")
			}
		}
		return &struct{}{}, nil
	})
	huma.Register(api, huma.Operation{OperationID: "login", Method: http.MethodPost, Path: "/api/v1/auth/login", Summary: "登录", Tags: []string{"认证"}}, func(_ context.Context, input *credentialsInput) (*sessionOutput, error) {
		session, err := service.Login(input.Body.Email, input.Body.Password, input.Body.DeviceLabel)
		if errors.Is(err, ErrEmailNotVerified) {
			return nil, huma.Error403Forbidden("邮箱尚未验证")
		}
		if err != nil {
			return nil, huma.Error401Unauthorized("邮箱或密码不正确")
		}
		return outputSession(session), nil
	})
	huma.Register(api, huma.Operation{OperationID: "apple-login", Method: http.MethodPost, Path: "/api/v1/auth/apple", Summary: "使用 Apple 登录", Tags: []string{"认证"}}, func(ctx context.Context, input *appleCredentialInput) (*sessionOutput, error) {
		session, err := service.SignInWithApple(ctx, appleVerifier, appleTokenClient, input.Body.IdentityToken, input.Body.AuthorizationCode, input.Body.Nonce, input.Body.DeviceLabel)
		if errors.Is(err, ErrAppleUnavailable) {
			return nil, huma.Error503ServiceUnavailable("Sign in with Apple 尚未配置")
		}
		if errors.Is(err, ErrAppleAccountLinkRequired) {
			return nil, huma.Error409Conflict("请先登录已有账号，再在账号设置中绑定 Apple")
		}
		if errors.Is(err, ErrAppleEmailRequired) {
			return nil, huma.Error400BadRequest("Apple 未提供可用邮箱")
		}
		if err != nil {
			return nil, huma.Error401Unauthorized("Apple 凭据无效")
		}
		return outputSession(session), nil
	})
	huma.Register(api, huma.Operation{OperationID: "apple-nonce", Method: http.MethodPost, Path: "/api/v1/auth/apple/nonce", Summary: "创建一次性 Apple 登录 nonce", Tags: []string{"认证"}}, func(context.Context, *struct{}) (*appleNonceOutput, error) {
		nonce, err := service.NewAppleNonce()
		if err != nil {
			return nil, huma.Error500InternalServerError("无法创建 Apple 登录请求")
		}
		output := &appleNonceOutput{}
		output.Body.Nonce = nonce
		return output, nil
	})
	huma.Register(api, huma.Operation{OperationID: "refresh", Method: http.MethodPost, Path: "/api/v1/auth/refresh", Summary: "轮换登录会话", Tags: []string{"认证"}}, func(_ context.Context, input *refreshInput) (*sessionOutput, error) {
		session, err := service.Refresh(input.Body.RefreshToken, input.Body.DeviceLabel)
		if err != nil {
			return nil, huma.Error401Unauthorized("refresh token 无效或已过期")
		}
		return outputSession(session), nil
	})
	huma.Register(api, huma.Operation{OperationID: "logout", Method: http.MethodPost, Path: "/api/v1/auth/logout", Summary: "退出当前会话", Tags: []string{"认证"}}, func(_ context.Context, input *refreshInput) (*struct{}, error) {
		if err := service.Logout(input.Body.RefreshToken); err != nil {
			return nil, huma.Error500InternalServerError("退出会话失败")
		}
		return &struct{}{}, nil
	})
	huma.Register(api, huma.Operation{OperationID: "forgot-password", Method: http.MethodPost, Path: "/api/v1/auth/forgot-password", Summary: "请求重置密码", Tags: []string{"认证"}}, func(ctx context.Context, input *emailInput) (*struct{}, error) {
		token, err := service.RequestPasswordReset(input.Body.Email)
		if err != nil {
			return nil, huma.Error500InternalServerError("无法请求密码重置")
		}
		if token != "" {
			if err := notifier.SendPasswordReset(ctx, input.Body.Email, token); err != nil {
				return nil, huma.Error503ServiceUnavailable("邮件服务暂时不可用")
			}
		}
		return &struct{}{}, nil
	})
	huma.Register(api, huma.Operation{OperationID: "reset-password", Method: http.MethodPost, Path: "/api/v1/auth/reset-password", Summary: "重置密码", Tags: []string{"认证"}}, func(_ context.Context, input *resetPasswordInput) (*struct{}, error) {
		if err := service.ResetPassword(input.Body.Token, input.Body.Password); err != nil {
			return nil, huma.Error400BadRequest("重置链接无效、已过期或密码不符合要求")
		}
		return &struct{}{}, nil
	})
	huma.Register(api, protectedOperation(huma.Operation{OperationID: "list-sessions", Method: http.MethodGet, Path: "/api/v1/auth/sessions", Summary: "列出活跃会话", Tags: []string{"认证"}}), func(_ context.Context, input *accessTokenInput) (*sessionsOutput, error) {
		userID, err := UserIDFromAuthorization(service, input.Authorization)
		if err != nil {
			return nil, huma.Error401Unauthorized("access token 无效")
		}
		sessions, err := service.ListSessions(userID)
		if err != nil {
			return nil, huma.Error500InternalServerError("无法读取会话")
		}
		output := &sessionsOutput{}
		output.Body.Sessions = make([]sessionInfoOutput, 0, len(sessions))
		for _, session := range sessions {
			output.Body.Sessions = append(output.Body.Sessions, sessionInfoOutput{ID: session.ID, DeviceLabel: session.DeviceLabel, CreatedAt: session.CreatedAt.UTC().Format(time.RFC3339), ExpiresAt: session.ExpiresAt.UTC().Format(time.RFC3339)})
		}
		return output, nil
	})
	huma.Register(api, protectedOperation(huma.Operation{OperationID: "revoke-session", Method: http.MethodDelete, Path: "/api/v1/auth/sessions/{id}", Summary: "撤销指定会话", Tags: []string{"认证"}}), func(_ context.Context, input *revokeSessionInput) (*struct{}, error) {
		userID, err := UserIDFromAuthorization(service, input.Authorization)
		if err != nil {
			return nil, huma.Error401Unauthorized("access token 无效")
		}
		if err := service.RevokeSession(userID, input.SessionID); err != nil {
			return nil, huma.Error404NotFound("会话不存在")
		}
		return &struct{}{}, nil
	})
	huma.Register(api, protectedOperation(huma.Operation{OperationID: "delete-account", Method: http.MethodDelete, Path: "/api/v1/account", Summary: "删除账号及关联数据", Tags: []string{"账号"}}), func(ctx context.Context, input *accessTokenInput) (*struct{}, error) {
		userID, err := UserIDFromAuthorization(service, input.Authorization)
		if err != nil {
			return nil, huma.Error401Unauthorized("access token 无效")
		}
		if err := service.DeleteAccount(ctx, userID, appleTokenClient); err != nil {
			if errors.Is(err, ErrAppleUnavailable) {
				return nil, huma.Error503ServiceUnavailable("Apple 凭据撤销服务未配置")
			}
			return nil, huma.Error404NotFound("账号不存在或凭据撤销失败")
		}
		return &struct{}{}, nil
	})
	huma.Register(api, protectedOperation(huma.Operation{OperationID: "link-apple", Method: http.MethodPost, Path: "/api/v1/account/auth-identities/apple", Summary: "绑定 Apple 登录", Tags: []string{"账号"}}), func(ctx context.Context, input *appleCredentialInput) (*struct{}, error) {
		userID, err := UserIDFromAuthorization(service, input.Authorization)
		if err != nil {
			return nil, huma.Error401Unauthorized("access token 无效")
		}
		err = service.LinkAppleIdentity(ctx, userID, appleVerifier, appleTokenClient, input.Body.IdentityToken, input.Body.AuthorizationCode, input.Body.Nonce)
		if errors.Is(err, ErrAppleUnavailable) {
			return nil, huma.Error503ServiceUnavailable("Sign in with Apple 尚未配置")
		}
		if errors.Is(err, ErrAppleIdentityLinked) {
			return nil, huma.Error409Conflict("此 Apple 账号已绑定到其他量迹账号")
		}
		if err != nil {
			return nil, huma.Error401Unauthorized("Apple 凭据无效")
		}
		return &struct{}{}, nil
	})
}

func outputSession(session Session) *sessionOutput {
	output := &sessionOutput{}
	output.Body.AccessToken = session.AccessToken
	output.Body.RefreshToken = session.RefreshToken
	output.Body.ExpiresIn = session.ExpiresIn
	return output
}

func UserIDFromAuthorization(service *Service, value string) (string, error) {
	parts := strings.Fields(value)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return "", ErrInvalidToken
	}
	return service.UserIDFromAccessToken(parts[1])
}

func protectedOperation(operation huma.Operation) huma.Operation {
	operation.Security = []map[string][]string{{"bearerAuth": {}}}
	operation.Errors = append(operation.Errors, http.StatusUnauthorized)
	return operation
}
