package database

import (
	"crypto/sha256"
	"database/sql"
	"embed"
	"fmt"
	"sort"
	"strings"
	"time"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

type migration struct {
	Version  string
	SQL      string
	Checksum string
}

func RunMigrations(db *sql.DB) error {
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version TEXT PRIMARY KEY,
		checksum TEXT NOT NULL,
		applied_at TEXT NOT NULL
	)`); err != nil {
		return fmt.Errorf("创建迁移元数据表: %w", err)
	}

	migrations, err := loadMigrations()
	if err != nil {
		return err
	}
	for _, migration := range migrations {
		if err := applyMigration(db, migration); err != nil {
			return err
		}
	}
	return nil
}

func loadMigrations() ([]migration, error) {
	entries, err := migrationFiles.ReadDir("migrations")
	if err != nil {
		return nil, fmt.Errorf("读取迁移文件: %w", err)
	}
	migrations := make([]migration, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		contents, err := migrationFiles.ReadFile("migrations/" + entry.Name())
		if err != nil {
			return nil, fmt.Errorf("读取迁移 %s: %w", entry.Name(), err)
		}
		migrations = append(migrations, migration{Version: entry.Name(), SQL: string(contents), Checksum: fmt.Sprintf("%x", sha256.Sum256(contents))})
	}
	sort.Slice(migrations, func(left, right int) bool { return migrations[left].Version < migrations[right].Version })
	return migrations, nil
}

func applyMigration(db *sql.DB, migration migration) error {
	var checksum string
	err := db.QueryRow("SELECT checksum FROM schema_migrations WHERE version = ?", migration.Version).Scan(&checksum)
	if err == nil {
		if checksum != migration.Checksum {
			return fmt.Errorf("迁移 %s 的校验和已变化", migration.Version)
		}
		return nil
	}
	if err != sql.ErrNoRows {
		return fmt.Errorf("读取迁移状态: %w", err)
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("开始迁移事务: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(migration.SQL); err != nil {
		return fmt.Errorf("执行迁移 %s: %w", migration.Version, err)
	}
	if _, err := tx.Exec("INSERT INTO schema_migrations(version, checksum, applied_at) VALUES (?, ?, ?)", migration.Version, migration.Checksum, time.Now().UTC().Format(time.RFC3339)); err != nil {
		return fmt.Errorf("记录迁移 %s: %w", migration.Version, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交迁移 %s: %w", migration.Version, err)
	}
	return nil
}
