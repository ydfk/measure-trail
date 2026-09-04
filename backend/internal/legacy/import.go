package legacy

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ImportOptions struct {
	SourcePath string
	OwnerEmail string
	DryRun     bool
}

func Import(target *gorm.DB, options ImportOptions) (Report, error) {
	report, err := Inspect(options.SourcePath)
	if err != nil || options.DryRun {
		return report, err
	}
	if strings.TrimSpace(options.OwnerEmail) == "" {
		return Report{}, fmt.Errorf("必须提供目标账号邮箱")
	}
	records, err := loadRecords(options.SourcePath)
	if err != nil {
		return Report{}, err
	}
	now := time.Now().UTC()
	err = target.Transaction(func(tx *gorm.DB) error {
		var userID string
		var verifiedAt sql.NullString
		if err := tx.Raw("SELECT id, email_verified_at FROM users WHERE email = ? AND status = 'active'", strings.ToLower(strings.TrimSpace(options.OwnerEmail))).Row().Scan(&userID, &verifiedAt); err != nil {
			return fmt.Errorf("目标账号不存在")
		}
		if !verifiedAt.Valid {
			return fmt.Errorf("目标账号尚未验证邮箱")
		}
		var count int
		if err := tx.Raw("SELECT COUNT(*) FROM legacy_imports WHERE user_id = ? AND source_sha256 = ?", userID, report.SourceSHA256).Row().Scan(&count); err != nil {
			return err
		}
		if count > 0 {
			return fmt.Errorf("此旧库已导入该账号")
		}
		for _, record := range records {
			var existing int
			if err := tx.Raw("SELECT COUNT(*) FROM measurements WHERE user_id = ? AND recorded_on = ? AND deleted_at IS NULL", userID, record.RecordedOn).Row().Scan(&existing); err != nil {
				return err
			}
			if existing > 0 {
				return fmt.Errorf("目标账号已有 %s 的记录", record.RecordedOn)
			}
		}
		for _, record := range records {
			if err := tx.Exec("INSERT INTO measurements(id, user_id, recorded_on, weight_g, waist_mm, note, source, version, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, 'legacy', 1, ?, ?)", uuid.NewString(), userID, record.RecordedOn, record.WeightG, record.WaistMM, record.Note, record.CreatedAt, record.UpdatedAt).Error; err != nil {
				return err
			}
		}
		reportJSON, err := json.Marshal(report)
		if err != nil {
			return err
		}
		return tx.Exec("INSERT INTO legacy_imports(id, user_id, source_sha256, source_path, row_count, imported_at, report_json) VALUES (?, ?, ?, ?, ?, ?, ?)", uuid.NewString(), userID, report.SourceSHA256, filepath.Base(options.SourcePath), len(records), now, string(reportJSON)).Error
	})
	return report, err
}

type legacyRecord struct {
	RecordedOn string
	WeightG    int
	WaistMM    *int
	Note       string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

func loadRecords(sourcePath string) ([]legacyRecord, error) {
	absPath, err := filepath.Abs(sourcePath)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite3", "file:"+absPath+"?mode=ro&immutable=1")
	if err != nil {
		return nil, err
	}
	defer db.Close()
	if err := validateSchema(db); err != nil {
		return nil, err
	}
	rows, err := db.Query("SELECT Date, WeightGongJin, WeightJin, WaistCircumference, Note, CreatedAt, UpdatedAt FROM WeightEntries ORDER BY Date, Id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	records := make([]legacyRecord, 0)
	for rows.Next() {
		var date, weight, jin, created, updated string
		var waist, note sql.NullString
		if err := rows.Scan(&date, &weight, &jin, &waist, &note, &created, &updated); err != nil {
			return nil, err
		}
		if _, err := time.Parse(time.DateOnly, date); err != nil {
			return nil, err
		}
		weightG, err := decimalUnits(weight, 1000)
		if err != nil || weightG < 10_000 || weightG > 500_000 {
			return nil, fmt.Errorf("旧库体重无效")
		}
		weightCentigrams, err := decimalUnits(weight, 100)
		if err != nil {
			return nil, fmt.Errorf("旧库公斤字段无效")
		}
		jinCentigrams, err := decimalUnits(jin, 100)
		if err != nil || jinCentigrams != weightCentigrams*2 {
			return nil, fmt.Errorf("旧库斤公斤字段不一致")
		}
		var waistMM *int
		if waist.Valid && strings.TrimSpace(waist.String) != "" {
			value, err := decimalUnits(waist.String, 10)
			if err != nil || value < 100 || value > 3000 {
				return nil, fmt.Errorf("旧库腰围无效")
			}
			waistMM = &value
		}
		createdAt, err := legacyTime(created)
		if err != nil {
			return nil, fmt.Errorf("旧库创建时间无效: %w", err)
		}
		updatedAt, err := legacyTime(updated)
		if err != nil {
			return nil, fmt.Errorf("旧库更新时间无效: %w", err)
		}
		records = append(records, legacyRecord{RecordedOn: date, WeightG: weightG, WaistMM: waistMM, Note: strings.TrimSpace(note.String), CreatedAt: createdAt, UpdatedAt: updatedAt})
	}
	return records, rows.Err()
}

func decimalUnits(value string, multiplier int64) (int, error) {
	rational, ok := new(big.Rat).SetString(strings.TrimSpace(value))
	if !ok {
		return 0, errors.New("无效小数")
	}
	rational.Mul(rational, big.NewRat(multiplier, 1))
	quotient, remainder := new(big.Int), new(big.Int)
	quotient.QuoRem(rational.Num(), rational.Denom(), remainder)
	if new(big.Int).Mul(remainder, big.NewInt(2)).Cmp(rational.Denom()) >= 0 {
		quotient.Add(quotient, big.NewInt(1))
	}
	if !quotient.IsInt64() {
		return 0, errors.New("数值溢出")
	}
	return int(quotient.Int64()), nil
}

func legacyTime(value string) (time.Time, error) {
	for _, layout := range []string{"2006-01-02 15:04:05.999999999", time.RFC3339Nano} {
		if parsed, err := time.ParseInLocation(layout, value, time.UTC); err == nil {
			return parsed.UTC(), nil
		}
	}
	return time.Time{}, errors.New("无法解析时间")
}
