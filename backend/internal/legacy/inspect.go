package legacy

import (
	"crypto/sha256"
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type Report struct {
	SourceSHA256 string `json:"sourceSha256"`
	RowCount     int    `json:"rowCount"`
	FirstDate    string `json:"firstDate"`
	LastDate     string `json:"lastDate"`
	WaistEntries int    `json:"waistEntries"`
	NoteEntries  int    `json:"noteEntries"`
	GeneratedAt  string `json:"generatedAt"`
}

var requiredColumns = []string{"Id", "CreatedAt", "Date", "Note", "UpdatedAt", "WeightGongJin", "WeightJin", "WaistCircumference"}

func Inspect(sourcePath string) (Report, error) {
	absPath, err := filepath.Abs(sourcePath)
	if err != nil {
		return Report{}, err
	}
	digest, err := fileSHA256(absPath)
	if err != nil {
		return Report{}, err
	}
	db, err := sql.Open("sqlite3", "file:"+absPath+"?mode=ro&immutable=1")
	if err != nil {
		return Report{}, err
	}
	defer db.Close()
	if err := validateSchema(db); err != nil {
		return Report{}, err
	}
	rows, err := db.Query("SELECT Date, WeightGongJin, WeightJin, WaistCircumference, Note FROM WeightEntries ORDER BY Date, Id")
	if err != nil {
		return Report{}, err
	}
	defer rows.Close()
	report := Report{SourceSHA256: digest, GeneratedAt: time.Now().UTC().Format(time.RFC3339)}
	for rows.Next() {
		var date, gongjin, jin string
		var waist, note sql.NullString
		if err := rows.Scan(&date, &gongjin, &jin, &waist, &note); err != nil {
			return Report{}, err
		}
		if _, err := time.Parse(time.DateOnly, date); err != nil {
			return Report{}, fmt.Errorf("旧库日期无效: %w", err)
		}
		if gongjin == "" || jin == "" {
			return Report{}, fmt.Errorf("旧库体重为空")
		}
		if waist.Valid && strings.TrimSpace(waist.String) != "" {
			report.WaistEntries++
		}
		if note.Valid && strings.TrimSpace(note.String) != "" {
			report.NoteEntries++
		}
		report.RowCount++
		if report.FirstDate == "" {
			report.FirstDate = date
		}
		report.LastDate = date
	}
	if err := rows.Err(); err != nil {
		return Report{}, err
	}
	if report.RowCount == 0 {
		return Report{}, fmt.Errorf("旧库没有记录")
	}
	return report, nil
}

func validateSchema(db *sql.DB) error {
	rows, err := db.Query("PRAGMA table_info(WeightEntries)")
	if err != nil {
		return err
	}
	defer rows.Close()
	columns := make([]string, 0)
	for rows.Next() {
		var cid int
		var name, valueType string
		var notNull int
		var defaultValue any
		var primaryKey int
		if err := rows.Scan(&cid, &name, &valueType, &notNull, &defaultValue, &primaryKey); err != nil {
			return err
		}
		columns = append(columns, name)
	}
	sort.Strings(columns)
	expected := append([]string(nil), requiredColumns...)
	sort.Strings(expected)
	if strings.Join(columns, ",") != strings.Join(expected, ",") {
		return fmt.Errorf("旧库 WeightEntries 表结构不匹配")
	}
	return rows.Err()
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}
