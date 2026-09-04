package database

import (
	"path/filepath"
	"testing"

	"github.com/ydfk/measure-trail/backend/internal/config"
)

func TestOpenAppliesSQLitePragmasAndMigrations(t *testing.T) {
	db, err := Open(config.Database{
		Path:          filepath.Join(t.TempDir(), "measuretrail.sqlite"),
		BusyTimeoutMS: 1000,
	})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}

	var foreignKeys int
	if err := db.Raw("PRAGMA foreign_keys").Scan(&foreignKeys).Error; err != nil {
		t.Fatalf("读取 foreign_keys: %v", err)
	}
	if foreignKeys != 1 {
		t.Fatalf("foreign_keys = %d, want 1", foreignKeys)
	}

	migrations, err := loadMigrations()
	if err != nil {
		t.Fatalf("读取嵌入迁移: %v", err)
	}
	var migrationCount int
	if err := db.Raw("SELECT COUNT(*) FROM schema_migrations").Scan(&migrationCount).Error; err != nil {
		t.Fatalf("查询 schema_migrations: %v", err)
	}
	if migrationCount != len(migrations) {
		t.Fatalf("migration count = %d, want %d", migrationCount, len(migrations))
	}
}
