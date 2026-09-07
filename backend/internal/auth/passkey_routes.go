package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-webauthn/webauthn/protocol"
	"gorm.io/gorm"
)

type passkeyRegistrationOptionsInput struct {
	Authorization string `header:"Authorization"`
	Body          struct {
		Name string `json:"name" minLength:"1" maxLength:"128" required:"true"`
	}
}

type passkeyVerifyInput struct {
	Authorization string `header:"Authorization"`
	Body          struct {
		SessionID   string          `json:"sessionId" minLength:"1" maxLength:"128" required:"true"`
		Credential  json.RawMessage `json:"credential" required:"true"`
		DeviceLabel string          `json:"deviceLabel" maxLength:"128"`
	}
}

type passkeyCreationOptionsOutput struct {
	Body struct {
		SessionID string                      `json:"sessionId"`
		Options   protocol.CredentialCreation `json:"options"`
	}
}

type passkeyRequestOptionsOutput struct {
	Body struct {
		SessionID string                       `json:"sessionId"`
		Options   protocol.CredentialAssertion `json:"options"`
	}
}

type passkeyItem struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	CreatedAt  time.Time  `json:"createdAt"`
	LastUsedAt *time.Time `json:"lastUsedAt,omitempty"`
}

type passkeyOutput struct{ Body passkeyItem }
type passkeyListOutput struct {
	Body struct {
		Passkeys []passkeyItem `json:"passkeys"`
	}
}

type passkeyIDInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" required:"true"`
}

type passkeyRenameInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" required:"true"`
	Body          struct {
		Name string `json:"name" minLength:"1" maxLength:"128" required:"true"`
	}
}

func registerPasskeyRoutes(api huma.API, service *Service) {
	huma.Register(api, huma.Operation{OperationID: "passkey-login-options", Method: http.MethodPost, Path: "/api/v1/auth/passkey/login/options", Summary: "创建 Passkey 登录挑战", Errors: []int{http.StatusBadRequest, http.StatusServiceUnavailable}, Tags: []string{"认证"}}, func(ctx context.Context, _ *struct{}) (*passkeyRequestOptionsOutput, error) {
		passkeys, err := configuredPasskeys(service)
		if err != nil {
			return nil, huma.Error503ServiceUnavailable(err.Error())
		}
		options, sessionID, err := passkeys.beginLogin(ctx)
		if err != nil {
			return nil, huma.Error400BadRequest(err.Error())
		}
		output := &passkeyRequestOptionsOutput{}
		output.Body.SessionID, output.Body.Options = sessionID, *options
		return output, nil
	})

	huma.Register(api, huma.Operation{OperationID: "passkey-login-verify", Method: http.MethodPost, Path: "/api/v1/auth/passkey/login/verify", Summary: "验证 Passkey 并登录", Errors: []int{http.StatusUnauthorized, http.StatusServiceUnavailable}, Tags: []string{"认证"}}, func(ctx context.Context, input *passkeyVerifyInput) (*sessionOutput, error) {
		passkeys, err := configuredPasskeys(service)
		if err != nil {
			return nil, huma.Error503ServiceUnavailable(err.Error())
		}
		session, err := passkeys.finishLogin(ctx, input.Body.SessionID, input.Body.Credential, input.Body.DeviceLabel)
		if err != nil {
			return nil, huma.Error401Unauthorized(err.Error())
		}
		return outputSession(session), nil
	})

	huma.Register(api, protectedOperation(huma.Operation{OperationID: "list-passkeys", Method: http.MethodGet, Path: "/api/v1/account/passkeys", Summary: "列出当前账号的 Passkey", Tags: []string{"账号"}}), func(ctx context.Context, input *accessTokenInput) (*passkeyListOutput, error) {
		userID, err := UserIDFromAuthorization(service, input.Authorization)
		if err != nil {
			return nil, huma.Error401Unauthorized("access token 无效")
		}
		passkeys, err := configuredPasskeys(service)
		if err != nil {
			return nil, huma.Error503ServiceUnavailable(err.Error())
		}
		credentials, err := passkeys.list(ctx, userID)
		if err != nil {
			return nil, huma.Error500InternalServerError("无法读取 Passkey")
		}
		output := &passkeyListOutput{}
		output.Body.Passkeys = make([]passkeyItem, 0, len(credentials))
		for _, credential := range credentials {
			output.Body.Passkeys = append(output.Body.Passkeys, outputPasskey(credential))
		}
		return output, nil
	})

	huma.Register(api, protectedOperation(huma.Operation{OperationID: "passkey-registration-options", Method: http.MethodPost, Path: "/api/v1/account/passkeys/registration/options", Summary: "创建 Passkey 注册挑战", Errors: []int{http.StatusBadRequest, http.StatusServiceUnavailable}, Tags: []string{"账号"}}), func(ctx context.Context, input *passkeyRegistrationOptionsInput) (*passkeyCreationOptionsOutput, error) {
		userID, err := UserIDFromAuthorization(service, input.Authorization)
		if err != nil {
			return nil, huma.Error401Unauthorized("access token 无效")
		}
		passkeys, err := configuredPasskeys(service)
		if err != nil {
			return nil, huma.Error503ServiceUnavailable(err.Error())
		}
		options, sessionID, err := passkeys.beginRegistration(ctx, userID, input.Body.Name)
		if err != nil {
			return nil, huma.Error400BadRequest(err.Error())
		}
		output := &passkeyCreationOptionsOutput{}
		output.Body.SessionID, output.Body.Options = sessionID, *options
		return output, nil
	})

	huma.Register(api, protectedOperation(huma.Operation{OperationID: "passkey-registration-verify", Method: http.MethodPost, Path: "/api/v1/account/passkeys/registration/verify", Summary: "验证并保存 Passkey", Errors: []int{http.StatusBadRequest, http.StatusServiceUnavailable}, Tags: []string{"账号"}}), func(ctx context.Context, input *passkeyVerifyInput) (*passkeyOutput, error) {
		userID, err := UserIDFromAuthorization(service, input.Authorization)
		if err != nil {
			return nil, huma.Error401Unauthorized("access token 无效")
		}
		passkeys, err := configuredPasskeys(service)
		if err != nil {
			return nil, huma.Error503ServiceUnavailable(err.Error())
		}
		credential, err := passkeys.finishRegistration(ctx, userID, input.Body.SessionID, input.Body.Credential)
		if err != nil {
			return nil, huma.Error400BadRequest(err.Error())
		}
		return &passkeyOutput{Body: outputPasskey(credential)}, nil
	})

	huma.Register(api, protectedOperation(huma.Operation{OperationID: "rename-passkey", Method: http.MethodPatch, Path: "/api/v1/account/passkeys/{id}", Summary: "重命名 Passkey", Tags: []string{"账号"}}), func(ctx context.Context, input *passkeyRenameInput) (*passkeyOutput, error) {
		userID, err := UserIDFromAuthorization(service, input.Authorization)
		if err != nil {
			return nil, huma.Error401Unauthorized("access token 无效")
		}
		passkeys, err := configuredPasskeys(service)
		if err != nil {
			return nil, huma.Error503ServiceUnavailable(err.Error())
		}
		credential, err := passkeys.rename(ctx, userID, input.ID, input.Body.Name)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, huma.Error404NotFound("Passkey 不存在")
		}
		if err != nil {
			return nil, huma.Error400BadRequest(err.Error())
		}
		return &passkeyOutput{Body: outputPasskey(credential)}, nil
	})

	huma.Register(api, protectedOperation(huma.Operation{OperationID: "delete-passkey", Method: http.MethodDelete, Path: "/api/v1/account/passkeys/{id}", Summary: "删除 Passkey", DefaultStatus: http.StatusNoContent, Tags: []string{"账号"}}), func(ctx context.Context, input *passkeyIDInput) (*struct{}, error) {
		userID, err := UserIDFromAuthorization(service, input.Authorization)
		if err != nil {
			return nil, huma.Error401Unauthorized("access token 无效")
		}
		passkeys, err := configuredPasskeys(service)
		if err != nil {
			return nil, huma.Error503ServiceUnavailable(err.Error())
		}
		if err := passkeys.delete(ctx, userID, input.ID); errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, huma.Error404NotFound("Passkey 不存在")
		} else if err != nil {
			return nil, huma.Error500InternalServerError("无法删除 Passkey")
		}
		return &struct{}{}, nil
	})
}

func configuredPasskeys(service *Service) (*passkeyService, error) {
	if service == nil || service.passkeys == nil {
		return nil, errPasskeyUnavailable
	}
	return service.passkeys, nil
}

func outputPasskey(credential PasskeyCredential) passkeyItem {
	return passkeyItem{ID: credential.ID, Name: credential.Name, CreatedAt: credential.CreatedAt, LastUsedAt: credential.LastUsedAt}
}
