package server

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/ydfk/measure-trail/backend/internal/auth"
	"github.com/ydfk/measure-trail/backend/internal/config"
	"github.com/ydfk/measure-trail/backend/internal/database"
)

func TestHealthRouteAndOpenAPI(t *testing.T) {
	db, err := database.Open(config.Database{
		Path:          filepath.Join(t.TempDir(), "measuretrail.sqlite"),
		BusyTimeoutMS: 1000,
	})
	if err != nil {
		t.Fatalf("打开测试数据库: %v", err)
	}
	authService, err := auth.NewService(db, config.Auth{RegistrationEnabled: true,
		Issuer:        "measuretrail",
		Audience:      "measuretrail-ios",
		AccessSecret:  "01234567890123456789012345678901",
		RefreshSecret: "abcdefghijklmnopqrstuvwxyzABCDEF",
	})
	if err != nil {
		t.Fatalf("创建认证服务: %v", err)
	}
	app := New(config.Config{App: config.App{Port: "21000", PublicBaseURL: "http://localhost:21000"}}, db, authService, nil)

	healthResponse, err := app.Test(httptest.NewRequest("GET", "/api/health", nil))
	if err != nil {
		t.Fatalf("health request: %v", err)
	}
	defer healthResponse.Body.Close()
	if healthResponse.StatusCode != 200 {
		t.Fatalf("health status = %d, want 200", healthResponse.StatusCode)
	}
	var health map[string]string
	if err := json.NewDecoder(healthResponse.Body).Decode(&health); err != nil {
		t.Fatalf("解码 health 响应: %v", err)
	}
	if health["status"] != "ok" || health["service"] != "measuretrail-api" {
		t.Fatalf("health response = %#v", health)
	}

	openAPIResponse, err := app.Test(httptest.NewRequest("GET", "/openapi.json", nil))
	if err != nil {
		t.Fatalf("openapi request: %v", err)
	}
	defer openAPIResponse.Body.Close()
	if openAPIResponse.StatusCode != 200 {
		t.Fatalf("openapi status = %d, want 200", openAPIResponse.StatusCode)
	}
	var document struct {
		Components struct {
			SecuritySchemes map[string]map[string]any `json:"securitySchemes"`
		} `json:"components"`
		Paths map[string]map[string]struct {
			Security  []map[string][]string      `json:"security"`
			Responses map[string]json.RawMessage `json:"responses"`
		} `json:"paths"`
	}
	if err := json.NewDecoder(openAPIResponse.Body).Decode(&document); err != nil {
		t.Fatalf("解码 OpenAPI 文档: %v", err)
	}
	if document.Components.SecuritySchemes["bearerAuth"]["scheme"] != "bearer" {
		t.Fatalf("OpenAPI Bearer 认证方案=%#v", document.Components.SecuritySchemes)
	}
	if _, ok := document.Paths["/api/v1/measurements/by-date/{date}"][strings.ToLower(http.MethodPut)].Responses["409"]; !ok {
		t.Fatalf("OpenAPI 未声明创建同日记录的 409 响应")
	}
	for _, path := range []string{"/api/v1/auth/passkey/login/options", "/api/v1/auth/passkey/login/verify", "/api/v1/account/passkeys", "/api/v1/account/passkeys/registration/options", "/api/v1/account/passkeys/registration/verify"} {
		if _, ok := document.Paths[path]; !ok {
			t.Fatalf("OpenAPI 缺少 Passkey 路由 %s", path)
		}
	}
}

func TestPasskeyRoutesCreateRegistrationChallenge(t *testing.T) {
	db, err := database.Open(config.Database{Path: filepath.Join(t.TempDir(), "measuretrail.sqlite"), BusyTimeoutMS: 1000})
	if err != nil {
		t.Fatal(err)
	}
	service, err := auth.NewService(db, config.Auth{Issuer: "measuretrail", Audience: "measuretrail-ios", AccessSecret: "01234567890123456789012345678901", RefreshSecret: "abcdefghijklmnopqrstuvwxyzABCDEF"})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.EnsureDefaultUser("admin", "111111"); err != nil {
		t.Fatal(err)
	}
	passkeyConfig := config.Passkey{
		RPID:                    "localhost",
		RPName:                  "量迹测试",
		Origins:                 []string{"http://localhost:21000"},
		CredentialEncryptionKey: base64.RawStdEncoding.EncodeToString([]byte("01234567890123456789012345678901")),
	}
	if err := service.ConfigurePasskeys(passkeyConfig); err != nil {
		t.Fatal(err)
	}
	app := New(config.Config{App: config.App{Port: "21000", PublicBaseURL: "http://localhost:21000"}, Passkey: passkeyConfig}, db, service, nil)
	login := request(t, app, http.MethodPost, "/api/v1/auth/login", `{"username":"admin","password":"111111","deviceLabel":"iPhone"}`)
	defer login.Body.Close()
	var session struct {
		AccessToken string `json:"accessToken"`
	}
	if login.StatusCode != http.StatusOK || json.NewDecoder(login.Body).Decode(&session) != nil {
		t.Fatalf("登录失败: %d", login.StatusCode)
	}
	registration := httptest.NewRequest(http.MethodPost, "/api/v1/account/passkeys/registration/options", strings.NewReader(`{"name":"我的 iPhone"}`))
	registration.Header.Set("Authorization", "Bearer "+session.AccessToken)
	registration.Header.Set("Content-Type", "application/json")
	response, err := app.Test(registration)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body := readBody(t, response)
	if response.StatusCode != http.StatusOK || !strings.Contains(body, `"sessionId"`) || !strings.Contains(body, `"id":"localhost"`) {
		t.Fatalf("Passkey 注册选项 status=%d body=%s", response.StatusCode, body)
	}
	listRequest := httptest.NewRequest(http.MethodGet, "/api/v1/account/passkeys", nil)
	// iOS 注册请求不携带登录会话的设备名称，也应进入 WebAuthn 校验。
	verify := httptest.NewRequest(http.MethodPost, "/api/v1/account/passkeys/registration/verify", strings.NewReader(`{"sessionId":"missing-session","credential":{}}`))
	verify.Header.Set("Authorization", "Bearer "+session.AccessToken)
	verify.Header.Set("Content-Type", "application/json")
	verifyResponse, err := app.Test(verify)
	if err != nil {
		t.Fatal(err)
	}
	defer verifyResponse.Body.Close()
	if body := readBody(t, verifyResponse); verifyResponse.StatusCode != http.StatusBadRequest || !strings.Contains(body, "Passkey 会话不存在或已过期") {
		t.Fatalf("注册请求应进入会话校验，status=%d body=%s", verifyResponse.StatusCode, body)
	}
	listRequest.Header.Set("Authorization", "Bearer "+session.AccessToken)
	listResponse, err := app.Test(listRequest)
	if err != nil {
		t.Fatal(err)
	}
	defer listResponse.Body.Close()
	if body := readBody(t, listResponse); listResponse.StatusCode != http.StatusOK || !strings.Contains(body, `"passkeys":[]`) {
		t.Fatalf("Passkey 列表 status=%d body=%s", listResponse.StatusCode, body)
	}
	var options struct {
		SessionID string `json:"sessionId"`
		Options   struct {
			PublicKey struct {
				Challenge string `json:"challenge"`
			} `json:"publicKey"`
		} `json:"options"`
	}
	for _, includeDeviceLabel := range []bool{false, true} {
		// 每次注册都重新获取挑战，分别覆盖旧、新客户端请求格式。
		challengeRequest := httptest.NewRequest(http.MethodPost, "/api/v1/account/passkeys/registration/options", strings.NewReader(`{"name":"测试凭据"}`))
		challengeRequest.Header.Set("Authorization", "Bearer "+session.AccessToken)
		challengeRequest.Header.Set("Content-Type", "application/json")
		challengeResponse, err := app.Test(challengeRequest)
		if err != nil {
			t.Fatal(err)
		}
		if challengeResponse.StatusCode != http.StatusOK {
			t.Fatalf("创建注册挑战失败: %d", challengeResponse.StatusCode)
		}
		if err := json.NewDecoder(challengeResponse.Body).Decode(&options); err != nil {
			t.Fatal(err)
		}
		challengeResponse.Body.Close()
		payload := map[string]any{"sessionId": options.SessionID, "credential": registrationCredential(t, options.Options.PublicKey.Challenge)}
		if includeDeviceLabel {
			payload["deviceLabel"] = "iPhone"
		}
		encoded, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		verified := httptest.NewRequest(http.MethodPost, "/api/v1/account/passkeys/registration/verify", strings.NewReader(string(encoded)))
		verified.Header.Set("Authorization", "Bearer "+session.AccessToken)
		verified.Header.Set("Content-Type", "application/json")
		verifiedResponse, err := app.Test(verified)
		if err != nil {
			t.Fatal(err)
		}
		result := readBody(t, verifiedResponse)
		verifiedResponse.Body.Close()
		if verifiedResponse.StatusCode != http.StatusOK {
			t.Fatalf("注册失败 deviceLabel=%v status=%d body=%s", includeDeviceLabel, verifiedResponse.StatusCode, result)
		}
		var saved struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal([]byte(result), &saved); err != nil || saved.ID == "" {
			t.Fatalf("凭据未保存: %s", result)
		}
		var count int64
		if err := db.Raw("SELECT COUNT(*) FROM passkey_credentials WHERE id = ?", saved.ID).Scan(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("凭据未写入数据库: %s", saved.ID)
		}
	}
}

func TestAppleAppSiteAssociation(t *testing.T) {
	app := New(config.Config{
		App:     config.App{Port: "21000", PublicBaseURL: "https://measure-trail.ydfk.site"},
		Passkey: config.Passkey{IOSAppID: "TEAM123.com.ydfk.MeasureTrail"},
	}, nil, nil, nil)
	response, err := app.Test(httptest.NewRequest(http.MethodGet, "/.well-known/apple-app-site-association", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body := readBody(t, response)
	if response.StatusCode != http.StatusOK || !strings.Contains(response.Header.Get("Content-Type"), "application/json") || !strings.Contains(body, "TEAM123.com.ydfk.MeasureTrail") {
		t.Fatalf("AASA status=%d content-type=%q body=%s", response.StatusCode, response.Header.Get("Content-Type"), body)
	}
}

func TestCORSPreflightAllowsMeasurementUpsert(t *testing.T) {
	db, err := database.Open(config.Database{Path: filepath.Join(t.TempDir(), "measuretrail.sqlite"), BusyTimeoutMS: 1000})
	if err != nil {
		t.Fatalf("打开测试数据库: %v", err)
	}
	publicURL := "https://api.measuretrail.example.com"
	webOrigin := "https://web.measuretrail.example.com"
	app := New(config.Config{App: config.App{Port: "21000", PublicBaseURL: publicURL, CORSOrigins: []string{webOrigin}}}, db, nil, nil)
	request := httptest.NewRequest(http.MethodOptions, "/api/v1/measurements/by-date/2025-09-26", nil)
	request.Header.Set("Origin", webOrigin)
	request.Header.Set("Access-Control-Request-Method", http.MethodPut)
	request.Header.Set("Access-Control-Request-Headers", "Authorization, Content-Type")

	response, err := app.Test(request)
	if err != nil {
		t.Fatalf("CORS 预检请求: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("CORS 预检 status=%d, body=%s", response.StatusCode, readBody(t, response))
	}
	if !strings.Contains(response.Header.Get("Access-Control-Allow-Methods"), http.MethodPut) {
		t.Fatalf("CORS 未允许 PUT，header=%q", response.Header.Get("Access-Control-Allow-Methods"))
	}
	if response.Header.Get("Access-Control-Allow-Origin") != webOrigin {
		t.Fatalf("CORS origin=%q, want %q", response.Header.Get("Access-Control-Allow-Origin"), webOrigin)
	}
}

func TestAuthenticationRoutes(t *testing.T) {
	db, err := database.Open(config.Database{Path: filepath.Join(t.TempDir(), "measuretrail.sqlite"), BusyTimeoutMS: 1000})
	if err != nil {
		t.Fatalf("打开测试数据库: %v", err)
	}
	service, err := auth.NewService(db, config.Auth{RegistrationEnabled: true,
		Issuer:        "measuretrail",
		Audience:      "measuretrail-ios",
		AccessSecret:  "01234567890123456789012345678901",
		RefreshSecret: "abcdefghijklmnopqrstuvwxyzABCDEF",
	})
	if err != nil {
		t.Fatalf("创建认证服务: %v", err)
	}
	app := New(config.Config{App: config.App{Port: "21000", PublicBaseURL: "http://localhost:21000"}}, db, service, auth.NewAppleVerifier(config.Apple{}))

	verification, err := service.Register("route@example.com", "correct-horse-battery-staple")
	if err != nil || service.VerifyEmail(verification) != nil {
		t.Fatalf("准备测试账号: %v", err)
	}

	login := request(t, app, http.MethodPost, "/api/v1/auth/login", `{"username":"route@example.com","password":"correct-horse-battery-staple","deviceLabel":"iPhone"}`)
	defer login.Body.Close()
	if login.StatusCode != http.StatusOK {
		t.Fatalf("登录 status = %d, body = %s", login.StatusCode, readBody(t, login))
	}
	var session struct {
		AccessToken  string `json:"accessToken"`
		RefreshToken string `json:"refreshToken"`
	}
	if err := json.NewDecoder(login.Body).Decode(&session); err != nil {
		t.Fatalf("解码登录响应: %v", err)
	}
	if session.AccessToken == "" || session.RefreshToken == "" {
		t.Fatalf("登录响应缺少 token: %#v", session)
	}
}

func TestAuthenticationRoutesRateLimitedByPath(t *testing.T) {
	db, err := database.Open(config.Database{Path: filepath.Join(t.TempDir(), "measuretrail.sqlite"), BusyTimeoutMS: 1000})
	if err != nil {
		t.Fatalf("打开测试数据库: %v", err)
	}
	service, err := auth.NewService(db, config.Auth{RegistrationEnabled: true,
		Issuer:        "measuretrail",
		Audience:      "measuretrail-ios",
		AccessSecret:  "01234567890123456789012345678901",
		RefreshSecret: "abcdefghijklmnopqrstuvwxyzABCDEF",
	})
	if err != nil {
		t.Fatalf("创建认证服务: %v", err)
	}
	app := New(config.Config{App: config.App{Port: "21000", PublicBaseURL: "http://localhost:21000"}}, db, service, nil)

	for attempt := 0; attempt < 10; attempt++ {
		response := request(t, app, http.MethodPost, "/api/v1/auth/login", `{"username":"missing","password":"correct-horse-battery-staple","deviceLabel":"iPhone"}`)
		if response.StatusCode != http.StatusUnauthorized {
			t.Fatalf("第 %d 次登录 status=%d, body=%s", attempt+1, response.StatusCode, readBody(t, response))
		}
		response.Body.Close()
	}

	limited := request(t, app, http.MethodPost, "/api/v1/auth/login", `{"username":"missing","password":"correct-horse-battery-staple","deviceLabel":"iPhone"}`)
	defer limited.Body.Close()
	if limited.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("限流登录 status=%d, body=%s", limited.StatusCode, readBody(t, limited))
	}

	registration := request(t, app, http.MethodPost, "/api/v1/auth/register", `{"username":"new-user","password":"correct-horse-battery-staple","deviceLabel":"iPhone"}`)
	defer registration.Body.Close()
	if registration.StatusCode != http.StatusNotFound {
		t.Fatalf("注册路由应不存在，status=%d, body=%s", registration.StatusCode, readBody(t, registration))
	}
}

func TestTrackingRoutesRequireOwner(t *testing.T) {
	db, err := database.Open(config.Database{Path: filepath.Join(t.TempDir(), "measuretrail.sqlite"), BusyTimeoutMS: 1000})
	if err != nil {
		t.Fatalf("打开测试数据库: %v", err)
	}
	service, err := auth.NewService(db, config.Auth{RegistrationEnabled: true, Issuer: "measuretrail", Audience: "measuretrail-ios", AccessSecret: "01234567890123456789012345678901", RefreshSecret: "abcdefghijklmnopqrstuvwxyzABCDEF"})
	if err != nil {
		t.Fatalf("创建认证服务: %v", err)
	}
	firstToken, err := service.Register("owner@example.com", "correct-horse-battery-staple")
	if err != nil || service.VerifyEmail(firstToken) != nil {
		t.Fatalf("初始化所有者: %v", err)
	}
	secondToken, err := service.Register("other@example.com", "correct-horse-battery-staple")
	if err != nil || service.VerifyEmail(secondToken) != nil {
		t.Fatalf("初始化其他用户: %v", err)
	}
	owner, err := service.Login("owner@example.com", "correct-horse-battery-staple", "owner")
	if err != nil {
		t.Fatalf("所有者登录: %v", err)
	}
	other, err := service.Login("other@example.com", "correct-horse-battery-staple", "other")
	if err != nil {
		t.Fatalf("其他用户登录: %v", err)
	}
	app := New(config.Config{App: config.App{Port: "21000", PublicBaseURL: "http://localhost:21000"}}, db, service, nil)

	requestWithAuthorization(t, app, http.MethodPut, "/api/v1/measurements/by-date/2025-09-26", `{"weightG":76120,"note":"晨起","clientMutationId":"mutation-0001"}`, owner.AccessToken, http.StatusOK)

	ownerList := requestWithAuthorization(t, app, http.MethodGet, "/api/v1/measurements", "", owner.AccessToken, http.StatusOK)
	defer ownerList.Body.Close()
	var list struct {
		Measurements []struct {
			ID string `json:"id"`
		} `json:"measurements"`
	}
	if err := json.NewDecoder(ownerList.Body).Decode(&list); err != nil || len(list.Measurements) != 1 {
		t.Fatalf("所有者列表=%#v error=%v", list, err)
	}
	changes := requestWithAuthorization(t, app, http.MethodGet, "/api/v1/measurement-changes?limit=50", "", owner.AccessToken, http.StatusOK)
	defer changes.Body.Close()
	var changePage struct {
		Measurements []struct {
			ID string `json:"id"`
		} `json:"measurements"`
		NextCursor string `json:"nextCursor"`
	}
	if err := json.NewDecoder(changes.Body).Decode(&changePage); err != nil || len(changePage.Measurements) != 1 || changePage.Measurements[0].ID != list.Measurements[0].ID || changePage.NextCursor == "" {
		t.Fatalf("增量同步=%#v error=%v", changePage, err)
	}
	otherGet := requestWithAuthorization(t, app, http.MethodGet, "/api/v1/measurements/"+list.Measurements[0].ID, "", other.AccessToken, http.StatusNotFound)
	otherGet.Body.Close()
}

func TestTrackingRoutesRejectStaleMeasurementVersionAcrossDeviceSessions(t *testing.T) {
	db, err := database.Open(config.Database{Path: filepath.Join(t.TempDir(), "measuretrail.sqlite"), BusyTimeoutMS: 1000})
	if err != nil {
		t.Fatalf("打开测试数据库: %v", err)
	}
	service, err := auth.NewService(db, config.Auth{RegistrationEnabled: true, Issuer: "measuretrail", Audience: "measuretrail-ios", AccessSecret: "01234567890123456789012345678901", RefreshSecret: "abcdefghijklmnopqrstuvwxyzABCDEF"})
	if err != nil {
		t.Fatalf("创建认证服务: %v", err)
	}
	verification, err := service.Register("conflict@example.com", "correct-horse-battery-staple")
	if err != nil || service.VerifyEmail(verification) != nil {
		t.Fatalf("初始化冲突测试用户: %v", err)
	}
	firstDevice, err := service.Login("conflict@example.com", "correct-horse-battery-staple", "iPhone")
	if err != nil {
		t.Fatalf("第一台设备登录: %v", err)
	}
	secondDevice, err := service.Login("conflict@example.com", "correct-horse-battery-staple", "iPad")
	if err != nil {
		t.Fatalf("第二台设备登录: %v", err)
	}
	app := New(config.Config{App: config.App{Port: "21000", PublicBaseURL: "http://localhost:21000"}}, db, service, nil)

	created := requestWithAuthorization(t, app, http.MethodPut, "/api/v1/measurements/by-date/2025-09-26", `{"weightG":76120,"clientMutationId":"mutation-create-0001"}`, firstDevice.AccessToken, http.StatusOK)
	var firstVersion struct {
		ID      string `json:"id"`
		Version int    `json:"version"`
	}
	if err := json.NewDecoder(created.Body).Decode(&firstVersion); err != nil {
		t.Fatalf("解码创建记录: %v", err)
	}
	created.Body.Close()
	if firstVersion.ID == "" || firstVersion.Version != 1 {
		t.Fatalf("创建记录=%#v", firstVersion)
	}
	staleCreate := requestWithAuthorization(t, app, http.MethodPut, "/api/v1/measurements/by-date/2025-09-26", `{"weightG":75000,"note":"离线新设备","clientMutationId":"mutation-create-0002"}`, secondDevice.AccessToken, http.StatusConflict)
	staleCreate.Body.Close()

	updated := requestWithAuthorization(t, app, http.MethodPatch, "/api/v1/measurements/"+firstVersion.ID, `{"weightG":76000,"note":"另一台设备","expectedVersion":1,"clientMutationId":"mutation-update-0001"}`, secondDevice.AccessToken, http.StatusOK)
	updated.Body.Close()
	stale := requestWithAuthorization(t, app, http.MethodPatch, "/api/v1/measurements/"+firstVersion.ID, `{"weightG":75000,"note":"本机旧编辑","expectedVersion":1,"clientMutationId":"mutation-update-0002"}`, firstDevice.AccessToken, http.StatusConflict)
	stale.Body.Close()
	latest := requestWithAuthorization(t, app, http.MethodGet, "/api/v1/measurements/"+firstVersion.ID, "", firstDevice.AccessToken, http.StatusOK)
	defer latest.Body.Close()
	var remote struct {
		WeightG int    `json:"weightG"`
		Note    string `json:"note"`
		Version int    `json:"version"`
	}
	if err := json.NewDecoder(latest.Body).Decode(&remote); err != nil {
		t.Fatalf("解码另一台设备的最新记录: %v", err)
	}
	if remote.WeightG != 76000 || remote.Note != "另一台设备" || remote.Version != 2 {
		t.Fatalf("另一台设备的最新记录=%#v", remote)
	}
}

func TestHealthKitImportRouteRequiresAuthenticationAndReturnsSource(t *testing.T) {
	db, err := database.Open(config.Database{Path: filepath.Join(t.TempDir(), "measuretrail.sqlite"), BusyTimeoutMS: 1000})
	if err != nil {
		t.Fatalf("打开测试数据库: %v", err)
	}
	service, err := auth.NewService(db, config.Auth{RegistrationEnabled: true, Issuer: "measuretrail", Audience: "measuretrail-ios", AccessSecret: "01234567890123456789012345678901", RefreshSecret: "abcdefghijklmnopqrstuvwxyzABCDEF"})
	if err != nil {
		t.Fatalf("创建认证服务: %v", err)
	}
	verification, err := service.Register("healthkit@example.com", "correct-horse-battery-staple")
	if err != nil || service.VerifyEmail(verification) != nil {
		t.Fatalf("初始化 HealthKit 用户: %v", err)
	}
	session, err := service.Login("healthkit@example.com", "correct-horse-battery-staple", "iPhone")
	if err != nil {
		t.Fatalf("HealthKit 用户登录: %v", err)
	}
	app := New(config.Config{App: config.App{Port: "21000", PublicBaseURL: "http://localhost:21000"}}, db, service, nil)
	body := `{"recordedOn":"2025-09-26","weightG":76120,"waistMm":810,"healthkitUuid":"healthkit-sample-0001","clientMutationId":"mutation-0001"}`

	unauthorized := request(t, app, http.MethodPost, "/api/v1/measurements/healthkit", body)
	if unauthorized.StatusCode != http.StatusUnauthorized {
		t.Fatalf("缺失 Authorization 的 HealthKit 导入 status=%d, body=%s", unauthorized.StatusCode, readBody(t, unauthorized))
	}
	unauthorized.Body.Close()
	invalid := requestWithAuthorization(t, app, http.MethodPost, "/api/v1/measurements/healthkit", body, "not-a-jwt", http.StatusUnauthorized)
	invalid.Body.Close()

	response := requestWithAuthorization(t, app, http.MethodPost, "/api/v1/measurements/healthkit", body, session.AccessToken, http.StatusOK)
	defer response.Body.Close()
	var measurement struct {
		Source        string `json:"source"`
		HealthKitUUID string `json:"healthkitUuid"`
		WeightG       int    `json:"weightG"`
	}
	if err := json.NewDecoder(response.Body).Decode(&measurement); err != nil {
		t.Fatalf("解码 HealthKit 导入响应: %v", err)
	}
	if measurement.Source != "healthkit" || measurement.HealthKitUUID != "healthkit-sample-0001" || measurement.WeightG != 76120 {
		t.Fatalf("HealthKit 导入响应=%#v", measurement)
	}

	openAPIResponse := request(t, app, http.MethodGet, "/openapi.json", "")
	defer openAPIResponse.Body.Close()
	var document struct {
		Paths map[string]map[string]struct {
			Security []map[string][]string `json:"security"`
		} `json:"paths"`
	}
	if err := json.NewDecoder(openAPIResponse.Body).Decode(&document); err != nil {
		t.Fatalf("解码 HealthKit OpenAPI 文档: %v", err)
	}
	if security := document.Paths["/api/v1/measurements/healthkit"]["post"].Security; len(security) != 1 || len(security[0]["bearerAuth"]) != 0 {
		t.Fatalf("HealthKit 导入 OpenAPI 认证声明=%#v", security)
	}
}

func request(t *testing.T, app *fiber.App, method string, path string, body string) *http.Response {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	response, err := app.Test(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	return response
}

func requestWithAuthorization(t *testing.T, app *fiber.App, method string, path string, body string, accessToken string, wantStatus int) *http.Response {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)
	response, err := app.Test(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	if response.StatusCode != wantStatus {
		t.Fatalf("%s %s status=%d want=%d body=%s", method, path, response.StatusCode, wantStatus, readBody(t, response))
	}
	return response
}

func readBody(t *testing.T, response *http.Response) string {
	t.Helper()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("读取响应: %v", err)
	}
	return string(body)
}

func TestRegistrationRouteClosedByDefault(t *testing.T) {
	db, err := database.Open(config.Database{Path: filepath.Join(t.TempDir(), "measuretrail.sqlite"), BusyTimeoutMS: 1000})
	if err != nil {
		t.Fatal(err)
	}
	service, err := auth.NewService(db, config.Auth{Issuer: "measuretrail", Audience: "measuretrail-ios", AccessSecret: "01234567890123456789012345678901", RefreshSecret: "abcdefghijklmnopqrstuvwxyzABCDEF"})
	if err != nil {
		t.Fatal(err)
	}
	app := New(config.Config{App: config.App{Port: "21000", PublicBaseURL: "http://localhost:21000"}}, db, service, nil)
	response := request(t, app, http.MethodPost, "/api/v1/auth/register", `{"email":"new@example.com","password":"correct-horse-battery-staple","deviceLabel":"iPhone"}`)
	defer response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("注册路由 status=%d", response.StatusCode)
	}
	var count int64
	if err := db.Table("users").Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("账号数=%d, error=%v", count, err)
	}
}

func TestEmailAndRegistrationRoutesAreUnavailable(t *testing.T) {
	db, err := database.Open(config.Database{Path: filepath.Join(t.TempDir(), "measuretrail.sqlite"), BusyTimeoutMS: 1000})
	if err != nil {
		t.Fatal(err)
	}
	service, err := auth.NewService(db, config.Auth{Issuer: "measuretrail", Audience: "measuretrail-ios", AccessSecret: "01234567890123456789012345678901", RefreshSecret: "abcdefghijklmnopqrstuvwxyzABCDEF"})
	if err != nil {
		t.Fatal(err)
	}
	app := New(config.Config{App: config.App{Port: "21000", PublicBaseURL: "http://localhost:21000"}}, db, service, nil)
	for _, path := range []string{"/api/v1/auth/register", "/api/v1/auth/verify-email", "/api/v1/auth/resend-verification", "/api/v1/auth/forgot-password", "/api/v1/auth/reset-password"} {
		response := request(t, app, http.MethodPost, path, `{}`)
		response.Body.Close()
		if response.StatusCode != http.StatusNotFound {
			t.Fatalf("%s status=%d, want 404", path, response.StatusCode)
		}
	}
}

func TestAccountCredentialsCanBeReadAndChanged(t *testing.T) {
	db, err := database.Open(config.Database{Path: filepath.Join(t.TempDir(), "measuretrail.sqlite"), BusyTimeoutMS: 1000})
	if err != nil {
		t.Fatal(err)
	}
	service, err := auth.NewService(db, config.Auth{Issuer: "measuretrail", Audience: "measuretrail-ios", AccessSecret: "01234567890123456789012345678901", RefreshSecret: "abcdefghijklmnopqrstuvwxyzABCDEF"})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.EnsureDefaultUser("admin", "111111"); err != nil {
		t.Fatal(err)
	}
	app := New(config.Config{App: config.App{Port: "21000", PublicBaseURL: "http://localhost:21000"}}, db, service, nil)
	login := request(t, app, http.MethodPost, "/api/v1/auth/login", `{"username":"admin","password":"111111","deviceLabel":"Web"}`)
	defer login.Body.Close()
	var session struct {
		AccessToken  string `json:"accessToken"`
		RefreshToken string `json:"refreshToken"`
	}
	if login.StatusCode != http.StatusOK || json.NewDecoder(login.Body).Decode(&session) != nil {
		t.Fatalf("默认账号登录 status=%d", login.StatusCode)
	}
	credentialsRequest := httptest.NewRequest(http.MethodGet, "/api/v1/account/credentials", nil)
	credentialsRequest.Header.Set("Authorization", "Bearer "+session.AccessToken)
	credentials, err := app.Test(credentialsRequest)
	if err != nil {
		t.Fatal(err)
	}
	defer credentials.Body.Close()
	if credentials.StatusCode != http.StatusOK || !strings.Contains(readBody(t, credentials), `"username":"admin"`) {
		t.Fatalf("读取凭证 status=%d", credentials.StatusCode)
	}
	updateRequest := httptest.NewRequest(http.MethodPatch, "/api/v1/account/credentials", strings.NewReader(`{"currentPassword":"111111","username":"owner","password":"654321"}`))
	updateRequest.Header.Set("Authorization", "Bearer "+session.AccessToken)
	updateRequest.Header.Set("Content-Type", "application/json")
	updated, err := app.Test(updateRequest)
	if err != nil {
		t.Fatal(err)
	}
	updated.Body.Close()
	if updated.StatusCode != http.StatusNoContent {
		t.Fatalf("修改凭证 status=%d", updated.StatusCode)
	}
	oldRefresh := request(t, app, http.MethodPost, "/api/v1/auth/refresh", `{"refreshToken":"`+session.RefreshToken+`","deviceLabel":"Web"}`)
	oldRefresh.Body.Close()
	if oldRefresh.StatusCode != http.StatusUnauthorized {
		t.Fatalf("旧 refresh token status=%d", oldRefresh.StatusCode)
	}
	oldLogin := request(t, app, http.MethodPost, "/api/v1/auth/login", `{"username":"admin","password":"111111","deviceLabel":"Web"}`)
	oldLogin.Body.Close()
	if oldLogin.StatusCode != http.StatusUnauthorized {
		t.Fatalf("旧凭证 status=%d", oldLogin.StatusCode)
	}
	newLogin := request(t, app, http.MethodPost, "/api/v1/auth/login", `{"username":"owner","password":"654321","deviceLabel":"Web"}`)
	newLogin.Body.Close()
	if newLogin.StatusCode != http.StatusOK {
		t.Fatalf("新凭证 status=%d", newLogin.StatusCode)
	}
}

func TestGoServesStaticAssetsAndSPAFallback(t *testing.T) {
	webRoot := t.TempDir()
	if err := os.Mkdir(filepath.Join(webRoot, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(webRoot, "index.html"), []byte("<main>MeasureTrail SPA</main>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(webRoot, "assets", "app.js"), []byte("window.measureTrail=true"), 0o644); err != nil {
		t.Fatal(err)
	}
	db, err := database.Open(config.Database{Path: filepath.Join(t.TempDir(), "measuretrail.sqlite"), BusyTimeoutMS: 1000})
	if err != nil {
		t.Fatal(err)
	}
	app := New(config.Config{App: config.App{Port: "21000", PublicBaseURL: "http://localhost:21000", WebRoot: webRoot}}, db, nil, nil)
	for path, expected := range map[string]string{"/": "MeasureTrail SPA", "/dashboard": "MeasureTrail SPA", "/assets/app.js": "window.measureTrail=true"} {
		response, err := app.Test(httptest.NewRequest(http.MethodGet, path, nil))
		if err != nil {
			t.Fatal(err)
		}
		body := readBody(t, response)
		response.Body.Close()
		if response.StatusCode != http.StatusOK || !strings.Contains(body, expected) {
			t.Fatalf("%s status=%d body=%q", path, response.StatusCode, body)
		}
	}
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		response, err := app.Test(httptest.NewRequest(method, "/api/missing", nil))
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusNotFound {
			t.Fatalf("%s 未知 API status=%d", method, response.StatusCode)
		}
	}
}
