package main

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"

	"github.com/ydfk/measure-trail/backend/internal/auth"
	"github.com/ydfk/measure-trail/backend/internal/config"
	"github.com/ydfk/measure-trail/backend/internal/database"
	"github.com/ydfk/measure-trail/backend/internal/server"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "用法: openapi <3.1 输出路径> <3.0 输出路径>")
		os.Exit(2)
	}
	appConfig, err := config.Load()
	if err != nil {
		panic(err)
	}
	db, err := database.Open(appConfig.Database)
	if err != nil {
		panic(err)
	}
	authService, err := auth.NewService(db, appConfig.Auth)
	if err != nil {
		panic(err)
	}
	if appConfig.Apple.ClientID != "" {
		if err := authService.ConfigureAppleCredentials(appConfig.Apple.CredentialEncryptionKey); err != nil {
			panic(err)
		}
	}
	app := server.New(appConfig, db, authService, auth.NewAppleVerifier(appConfig.Apple))
	response, err := app.Test(httptest.NewRequest("GET", "/openapi.json", nil))
	if err != nil {
		panic(err)
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		panic(fmt.Sprintf("OpenAPI 状态码为 %d", response.StatusCode))
	}
	var document map[string]any
	if err := json.NewDecoder(response.Body).Decode(&document); err != nil {
		panic(err)
	}
	if err := writeJSON(os.Args[1], document); err != nil {
		panic(err)
	}
	document["openapi"] = "3.0.3"
	if err := writeJSON(os.Args[2], document); err != nil {
		panic(err)
	}
}

func writeJSON(path string, document map[string]any) error {
	contents, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(contents, '\n'), 0o644)
}
