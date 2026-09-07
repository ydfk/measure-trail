package server

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humafiber"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/cors"
	"github.com/gofiber/fiber/v3/middleware/limiter"
	"github.com/gofiber/fiber/v3/middleware/recover"
	"github.com/ydfk/measure-trail/backend/internal/api/health"
	"github.com/ydfk/measure-trail/backend/internal/auth"
	"github.com/ydfk/measure-trail/backend/internal/config"
	"github.com/ydfk/measure-trail/backend/internal/tracking"
	"gorm.io/gorm"
)

func New(config config.Config, db *gorm.DB, authService *auth.Service, appleVerifier auth.AppleVerifier) *fiber.App {
	corsOrigins := config.App.CORSOrigins
	if len(corsOrigins) == 0 {
		corsOrigins = []string{config.App.PublicBaseURL}
	}
	app := fiber.New(fiber.Config{ErrorHandler: problemHandler})
	app.Use(recover.New())
	app.Use(limiter.New(limiter.Config{
		Max:        10,
		Expiration: time.Minute,
		KeyGenerator: func(ctx fiber.Ctx) string {
			return ctx.IP() + ":" + ctx.Path()
		},
		Next: func(ctx fiber.Ctx) bool {
			return !strings.HasPrefix(ctx.Path(), "/api/v1/auth/")
		},
	}))
	app.Use(cors.New(cors.Config{
		AllowOrigins: corsOrigins,
		AllowMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders: []string{"Authorization", "Content-Type"},
	}))

	humaConfig := huma.DefaultConfig("MeasureTrail API", "0.1.0")
	humaConfig.Info.Description = "量迹跨客户端共享 API。"
	humaConfig.Components.SecuritySchemes = map[string]*huma.SecurityScheme{
		"bearerAuth": {Type: "http", Scheme: "bearer", BearerFormat: "JWT", Description: "量迹 access token"},
	}
	humaConfig.CreateHooks = nil
	humaConfig.SchemasPath = ""
	api := humafiber.New(app, humaConfig)
	health.Register(api, db)
	if authService != nil {
		if appleVerifier == nil {
			appleVerifier = auth.NewAppleVerifier(config.Apple)
		}
		auth.RegisterRoutes(api, authService, appleVerifier, auth.NewAppleTokenClient(config.Apple))
		tracking.RegisterRoutes(api, tracking.NewService(db), authService)
	}
	registerAppleAppSiteAssociation(app, config.Passkey.IOSAppID)
	registerWebRoutes(app, config.App.WebRoot)
	return app
}

func registerAppleAppSiteAssociation(app *fiber.App, iosAppID string) {
	iosAppID = strings.TrimSpace(iosAppID)
	if iosAppID == "" {
		return
	}
	app.Get("/.well-known/apple-app-site-association", func(ctx fiber.Ctx) error {
		ctx.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
		return ctx.JSON(fiber.Map{"webcredentials": fiber.Map{"apps": []string{iosAppID}}})
	})
}

func registerWebRoutes(app *fiber.App, webRoot string) {
	webRoot = strings.TrimSpace(webRoot)
	if webRoot == "" {
		return
	}
	root, err := filepath.Abs(webRoot)
	if err != nil {
		return
	}
	indexPath := filepath.Join(root, "index.html")
	if info, err := os.Stat(indexPath); err != nil || !info.Mode().IsRegular() {
		return
	}
	app.All("/api/*", func(ctx fiber.Ctx) error {
		return fiber.ErrNotFound
	})
	app.Get("/*", func(ctx fiber.Ctx) error {
		requestPath := ctx.Path()
		if strings.HasPrefix(requestPath, "/api/") || requestPath == "/openapi.json" {
			return fiber.ErrNotFound
		}
		assetPath := filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(requestPath, "/")))
		if relative, err := filepath.Rel(root, assetPath); err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			if info, err := os.Stat(assetPath); err == nil && info.Mode().IsRegular() {
				return ctx.SendFile(assetPath)
			}
		}
		return ctx.SendFile(indexPath)
	})
}

func problemHandler(ctx fiber.Ctx, err error) error {
	status := fiber.StatusInternalServerError
	if typed, ok := err.(*fiber.Error); ok {
		status = typed.Code
	}
	return ctx.Status(status).JSON(fiber.Map{
		"type":   "about:blank",
		"title":  httpStatusText(status),
		"status": status,
	})
}

func httpStatusText(status int) string {
	return fmt.Sprintf("HTTP %d", status)
}
