package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
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
	app := New(config.Config{App: config.App{Port: "21000", PublicBaseURL: "http://localhost:21000"}}, db, authService, &recordingNotifier{}, nil)

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
}

func TestCORSPreflightAllowsMeasurementUpsert(t *testing.T) {
	db, err := database.Open(config.Database{Path: filepath.Join(t.TempDir(), "measuretrail.sqlite"), BusyTimeoutMS: 1000})
	if err != nil {
		t.Fatalf("打开测试数据库: %v", err)
	}
	publicURL := "https://api.measuretrail.example.com"
	webOrigin := "https://web.measuretrail.example.com"
	app := New(config.Config{App: config.App{Port: "21000", PublicBaseURL: publicURL, CORSOrigins: []string{webOrigin}}}, db, nil, nil, nil)
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
	notifier := &recordingNotifier{}
	app := New(config.Config{App: config.App{Port: "21000", PublicBaseURL: "http://localhost:21000"}}, db, service, notifier, auth.NewAppleVerifier(config.Apple{}))

	register := request(t, app, http.MethodPost, "/api/v1/auth/register", `{"email":"route@example.com","password":"correct-horse-battery-staple","deviceLabel":"iPhone"}`)
	if register.StatusCode != http.StatusNoContent {
		t.Fatalf("注册 status = %d, body = %s", register.StatusCode, readBody(t, register))
	}
	register.Body.Close()
	if notifier.verificationEmail != "route@example.com" || notifier.verificationToken == "" {
		t.Fatalf("验证邮件记录 = %#v", notifier)
	}

	verify := request(t, app, http.MethodPost, "/api/v1/auth/verify-email", `{"token":"`+notifier.verificationToken+`"}`)
	if verify.StatusCode != http.StatusNoContent {
		t.Fatalf("验证 status = %d, body = %s", verify.StatusCode, readBody(t, verify))
	}
	verify.Body.Close()

	login := request(t, app, http.MethodPost, "/api/v1/auth/login", `{"email":"route@example.com","password":"correct-horse-battery-staple","deviceLabel":"iPhone"}`)
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
	app := New(config.Config{App: config.App{Port: "21000", PublicBaseURL: "http://localhost:21000"}}, db, service, &recordingNotifier{}, nil)

	for attempt := 0; attempt < 10; attempt++ {
		response := request(t, app, http.MethodPost, "/api/v1/auth/login", `{"email":"missing@example.com","password":"correct-horse-battery-staple","deviceLabel":"iPhone"}`)
		if response.StatusCode != http.StatusUnauthorized {
			t.Fatalf("第 %d 次登录 status=%d, body=%s", attempt+1, response.StatusCode, readBody(t, response))
		}
		response.Body.Close()
	}

	limited := request(t, app, http.MethodPost, "/api/v1/auth/login", `{"email":"missing@example.com","password":"correct-horse-battery-staple","deviceLabel":"iPhone"}`)
	defer limited.Body.Close()
	if limited.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("限流登录 status=%d, body=%s", limited.StatusCode, readBody(t, limited))
	}

	registration := request(t, app, http.MethodPost, "/api/v1/auth/register", `{"email":"new@example.com","password":"correct-horse-battery-staple","deviceLabel":"iPhone"}`)
	defer registration.Body.Close()
	if registration.StatusCode != http.StatusNoContent {
		t.Fatalf("注册不应与登录共享限流计数，status=%d, body=%s", registration.StatusCode, readBody(t, registration))
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
	app := New(config.Config{App: config.App{Port: "21000", PublicBaseURL: "http://localhost:21000"}}, db, service, &recordingNotifier{}, nil)

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
	app := New(config.Config{App: config.App{Port: "21000", PublicBaseURL: "http://localhost:21000"}}, db, service, &recordingNotifier{}, nil)

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
	app := New(config.Config{App: config.App{Port: "21000", PublicBaseURL: "http://localhost:21000"}}, db, service, &recordingNotifier{}, nil)
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

type recordingNotifier struct {
	verificationEmail string
	verificationToken string
}

func (notifier *recordingNotifier) SendVerification(_ context.Context, email string, token string) error {
	notifier.verificationEmail = email
	notifier.verificationToken = token
	return nil
}

func (*recordingNotifier) SendPasswordReset(context.Context, string, string) error {
	return nil
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
	notifier := &recordingNotifier{}
	app := New(config.Config{App: config.App{Port: "21000", PublicBaseURL: "http://localhost:21000"}}, db, service, notifier, nil)
	response := request(t, app, http.MethodPost, "/api/v1/auth/register", `{"email":"new@example.com","password":"correct-horse-battery-staple","deviceLabel":"iPhone"}`)
	defer response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("关闭注册 status=%d", response.StatusCode)
	}
	if notifier.verificationToken != "" {
		t.Fatal("关闭注册后不应发送验证邮件")
	}
	var count int64
	if err := db.Table("users").Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("账号数=%d, error=%v", count, err)
	}
}
