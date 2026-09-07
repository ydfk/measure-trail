package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/ydfk/measure-trail/backend/internal/auth"
	"github.com/ydfk/measure-trail/backend/internal/config"
	"github.com/ydfk/measure-trail/backend/internal/database"
	"github.com/ydfk/measure-trail/backend/internal/legacy"
	"gorm.io/gorm"
)

func main() {
	source := flag.String("source", "", "旧 slimtrack SQLite 文件路径")
	owner := flag.String("owner-username", "", "目标量迹用户名；默认使用环境配置中的默认用户名")
	dryRun := flag.Bool("dry-run", false, "只校验并输出报告，不写入数据库")
	flag.Parse()
	if *source == "" {
		fmt.Fprintln(os.Stderr, "必须提供 --source")
		os.Exit(2)
	}
	var target *gorm.DB
	if !*dryRun {
		loaded, err := config.Load()
		if err != nil {
			panic(err)
		}
		db, err := database.Open(loaded.Database)
		if err != nil {
			panic(err)
		}
		target = db
		authService, err := auth.NewService(db, loaded.Auth)
		if err != nil {
			panic(err)
		}
		if err := authService.EnsureDefaultUser(loaded.Auth.DefaultUsername, loaded.Auth.DefaultPassword); err != nil {
			panic(err)
		}
	}
	options := legacy.ImportOptions{SourcePath: *source, OwnerUsername: *owner, DryRun: *dryRun}
	var report legacy.Report
	var err error
	report, err = legacy.Import(target, options)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	json.NewEncoder(os.Stdout).Encode(report)
}
