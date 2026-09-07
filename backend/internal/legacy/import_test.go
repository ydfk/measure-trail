package legacy

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/ydfk/measure-trail/backend/internal/auth"
	"github.com/ydfk/measure-trail/backend/internal/config"
	appdb "github.com/ydfk/measure-trail/backend/internal/database"
)

func TestWeightsAreConsistentAllowsLegacyRounding(t *testing.T) {
	for _, test := range []struct {
		name             string
		weightCentigrams int
		jinCentigrams    int
		want             bool
	}{
		{name: "exact", weightCentigrams: 7182, jinCentigrams: 14364, want: true},
		{name: "rounds up one centigram", weightCentigrams: 7182, jinCentigrams: 14365, want: true},
		{name: "rounds down one centigram", weightCentigrams: 7183, jinCentigrams: 14365, want: true},
		{name: "rejects material mismatch", weightCentigrams: 7182, jinCentigrams: 14366, want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := weightsAreConsistent(test.weightCentigrams, test.jinCentigrams); got != test.want {
				t.Fatalf("weightsAreConsistent() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestImportRecordsIncrementalSyncChanges(t *testing.T) {
	sourcePath := filepath.Join(t.TempDir(), "slimtrack.db")
	source, err := sql.Open("sqlite3", sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := source.Exec(`CREATE TABLE WeightEntries (
		Id INTEGER PRIMARY KEY,
		CreatedAt TEXT NOT NULL,
		Date TEXT NOT NULL,
		Note TEXT,
		UpdatedAt TEXT NOT NULL,
		WeightGongJin TEXT NOT NULL,
		WeightJin TEXT NOT NULL,
		WaistCircumference TEXT
	)`); err != nil {
		t.Fatal(err)
	}
	if _, err := source.Exec(`INSERT INTO WeightEntries
		(Id, CreatedAt, Date, Note, UpdatedAt, WeightGongJin, WeightJin, WaistCircumference)
		VALUES (1, '2025-09-26 08:00:00', '2025-09-26', '测试备注', '2025-09-26 08:00:00', '70.00', '140.00', '80.0')`); err != nil {
		t.Fatal(err)
	}
	if err := source.Close(); err != nil {
		t.Fatal(err)
	}

	target, err := appdb.Open(config.Database{Path: filepath.Join(t.TempDir(), "target.sqlite"), BusyTimeoutMS: 5000})
	if err != nil {
		t.Fatal(err)
	}
	authService, err := auth.NewService(target, config.Auth{
		Issuer:        "measuretrail",
		Audience:      "measuretrail-ios",
		AccessSecret:  "01234567890123456789012345678901",
		RefreshSecret: "abcdefghijklmnopqrstuvwxyzABCDEF",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := authService.EnsureDefaultUser("admin", "111111"); err != nil {
		t.Fatal(err)
	}

	report, err := Import(target, ImportOptions{SourcePath: sourcePath})
	if err != nil {
		t.Fatal(err)
	}
	if report.RowCount != 1 {
		t.Fatalf("导入记录数 = %d, want 1", report.RowCount)
	}
	var changeCount int
	if err := target.Raw(`SELECT COUNT(*) FROM measurement_changes c
		JOIN measurements m ON m.id = c.measurement_id
		WHERE m.source = 'legacy'`).Row().Scan(&changeCount); err != nil {
		t.Fatal(err)
	}
	if changeCount != 1 {
		t.Fatalf("历史导入同步变更数 = %d, want 1", changeCount)
	}
}
