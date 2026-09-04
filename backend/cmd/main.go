package main

import (
	"log"

	"github.com/ydfk/measure-trail/backend/internal/auth"
	"github.com/ydfk/measure-trail/backend/internal/config"
	"github.com/ydfk/measure-trail/backend/internal/database"
	"github.com/ydfk/measure-trail/backend/internal/server"
)

func main() {
	appConfig, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	db, err := database.Open(appConfig.Database)
	if err != nil {
		log.Fatal(err)
	}
	authService, err := auth.NewService(db, appConfig.Auth)
	if err != nil {
		log.Fatal(err)
	}
	if appConfig.Apple.ClientID != "" {
		if err := authService.ConfigureAppleCredentials(appConfig.Apple.CredentialEncryptionKey); err != nil {
			log.Fatal(err)
		}
	}
	notifier, err := auth.NewNotifier(appConfig.Mail, appConfig.App.PublicBaseURL)
	if err != nil {
		log.Fatal(err)
	}
	app := server.New(appConfig, db, authService, notifier, auth.NewAppleVerifier(appConfig.Apple))
	log.Printf("MeasureTrail API 正在监听 :%s", appConfig.App.Port)
	log.Fatal(app.Listen(":" + appConfig.App.Port))
}
