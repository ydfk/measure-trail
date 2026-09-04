package database

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ydfk/measure-trail/backend/internal/config"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func Open(databaseConfig config.Database) (*gorm.DB, error) {
	if err := os.MkdirAll(filepath.Dir(databaseConfig.Path), 0o750); err != nil {
		return nil, fmt.Errorf("创建 SQLite 目录: %w", err)
	}

	db, err := gorm.Open(sqlite.Open(databaseConfig.Path), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("打开 SQLite: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("获取 SQLite 连接: %w", err)
	}
	if err := configure(sqlDB, databaseConfig.BusyTimeoutMS); err != nil {
		return nil, err
	}
	if err := RunMigrations(sqlDB); err != nil {
		return nil, err
	}
	return db, nil
}

func configure(db *sql.DB, busyTimeoutMS int) error {
	statements := []string{
		"PRAGMA foreign_keys = ON",
		"PRAGMA journal_mode = WAL",
		"PRAGMA synchronous = NORMAL",
		fmt.Sprintf("PRAGMA busy_timeout = %d", busyTimeoutMS),
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			return fmt.Errorf("设置 SQLite pragma: %w", err)
		}
	}
	return nil
}
