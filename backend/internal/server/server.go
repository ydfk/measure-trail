package server

import (
	"fmt"
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

func New(config config.Config, db *gorm.DB, authService *auth.Service, notifier auth.Notifier, appleVerifier auth.AppleVerifier) *fiber.App {
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
	if authService != nil && notifier != nil {
		if appleVerifier == nil {
			appleVerifier = auth.NewAppleVerifier(config.Apple)
		}
		auth.RegisterRoutes(api, authService, notifier, appleVerifier, auth.NewAppleTokenClient(config.Apple))
		tracking.RegisterRoutes(api, tracking.NewService(db), authService)
	}
	return app
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
