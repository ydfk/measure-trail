package tracking

import (
	"bytes"
	"database/sql"
	"encoding/base64"
	"encoding/csv"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

var (
	ErrNotFound    = errors.New("记录不存在")
	ErrConflict    = errors.New("记录版本冲突")
	ErrStorageBusy = errors.New("SQLite 暂时忙碌")
)

type Service struct {
	db  *gorm.DB
	now func() time.Time
}

type Profile struct {
	HeightMM      *int
	TargetWeightG *int
	PreferredUnit string
	Timezone      string
	UpdatedAt     time.Time
}

type Measurement struct {
	ID            string
	RecordedOn    string
	WeightG       int
	WaistMM       *int
	Note          string
	Source        string
	HealthKitUUID *string
	Version       int
	DeletedAt     *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type MeasurementInput struct {
	RecordedOn       string
	WeightG          int
	WaistMM          *int
	Note             string
	ClientMutationID string
}

type HealthKitMeasurementInput struct {
	RecordedOn       string
	WeightG          int
	WaistMM          *int
	HealthKitUUID    string
	ClientMutationID string
}

type Statistics struct {
	Count          int
	FirstWeightG   int
	LastWeightG    int
	ChangeG        int
	AverageWeightG int
}

type ChangePage struct {
	Measurements []Measurement
	NextCursor   string
	HasMore      bool
}

const measurementColumns = "id, recorded_on, weight_g, waist_mm, note, source, healthkit_uuid, version, strftime('%Y-%m-%dT%H:%M:%fZ', deleted_at), strftime('%Y-%m-%dT%H:%M:%fZ', created_at), strftime('%Y-%m-%dT%H:%M:%fZ', updated_at)"
const sqliteWriteAttempts = 5

func NewService(db *gorm.DB) *Service {
	return &Service{db: db, now: time.Now}
}

func (service *Service) Profile(userID string) (Profile, error) {
	var profile Profile
	var height, target sql.NullInt64
	var updated string
	err := service.db.Raw("SELECT height_mm, target_weight_g, preferred_unit, timezone, strftime('%Y-%m-%dT%H:%M:%fZ', updated_at) FROM profiles WHERE user_id = ?", userID).Row().Scan(&height, &target, &profile.PreferredUnit, &profile.Timezone, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return Profile{PreferredUnit: "kg", Timezone: "Asia/Shanghai"}, nil
	}
	if err != nil {
		return Profile{}, err
	}
	profile.HeightMM = nullableInt(height)
	profile.TargetWeightG = nullableInt(target)
	profile.UpdatedAt, err = parseTime(updated)
	return profile, err
}

func (service *Service) SaveProfile(userID string, profile Profile) (Profile, error) {
	if err := validateProfile(profile); err != nil {
		return Profile{}, err
	}
	now := service.now().UTC()
	err := service.retryWrite(func() error {
		return service.db.Exec(`INSERT INTO profiles(user_id, height_mm, target_weight_g, preferred_unit, timezone, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(user_id) DO UPDATE SET height_mm = excluded.height_mm, target_weight_g = excluded.target_weight_g,
		preferred_unit = excluded.preferred_unit, timezone = excluded.timezone, updated_at = excluded.updated_at`,
			userID, profile.HeightMM, profile.TargetWeightG, profile.PreferredUnit, profile.Timezone, now).Error
	})
	if err != nil {
		return Profile{}, err
	}
	profile.UpdatedAt = now
	return profile, nil
}

func (service *Service) CreateByDate(userID string, input MeasurementInput) (Measurement, error) {
	if err := validateMeasurement(input); err != nil {
		return Measurement{}, err
	}
	now := service.now().UTC()
	var result Measurement
	err := service.transaction(func(tx *gorm.DB) error {
		if previous, processed, err := findMutation(tx, userID, input.ClientMutationID); err != nil {
			return err
		} else if processed {
			result = previous
			return nil
		}
		var existingID string
		err := tx.Raw("SELECT id FROM measurements WHERE user_id = ? AND recorded_on = ? AND deleted_at IS NULL", userID, input.RecordedOn).Row().Scan(&existingID)
		if errors.Is(err, sql.ErrNoRows) {
			result = Measurement{ID: uuid.NewString(), RecordedOn: input.RecordedOn, WeightG: input.WeightG, WaistMM: input.WaistMM, Note: strings.TrimSpace(input.Note), Source: "manual", Version: 1, CreatedAt: now, UpdatedAt: now}
			if err := tx.Exec("INSERT INTO measurements(id, user_id, recorded_on, weight_g, waist_mm, note, source, version, client_mutation_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, 'manual', 1, ?, ?, ?)", result.ID, userID, result.RecordedOn, result.WeightG, result.WaistMM, result.Note, input.ClientMutationID, now, now).Error; err != nil {
				return err
			}
			if err := recordMutation(tx, userID, input.ClientMutationID, result.ID, now); err != nil {
				return err
			}
			return recordChange(tx, userID, result.ID, now)
		}
		if err != nil {
			return err
		}
		// 没有服务端 ID 的同日写入不能覆盖另一台设备已经创建的记录。
		return ErrConflict
	})
	return result, err
}

func (service *Service) ImportHealthKitByDate(userID string, input HealthKitMeasurementInput) (Measurement, error) {
	if err := validateHealthKitMeasurement(input); err != nil {
		return Measurement{}, err
	}
	now := service.now().UTC()
	var result Measurement
	err := service.transaction(func(tx *gorm.DB) error {
		if previous, processed, err := findMutation(tx, userID, input.ClientMutationID); err != nil {
			return err
		} else if processed {
			result = previous
			return nil
		}
		if previous, err := scanMeasurement(tx.Raw("SELECT "+measurementColumns+" FROM measurements WHERE user_id = ? AND healthkit_uuid = ?", userID, input.HealthKitUUID).Row()); err == nil {
			result = previous
			return recordMutation(tx, userID, input.ClientMutationID, result.ID, now)
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}

		var id, source string
		err := tx.Raw("SELECT id, source FROM measurements WHERE user_id = ? AND recorded_on = ? AND deleted_at IS NULL", userID, input.RecordedOn).Row().Scan(&id, &source)
		if errors.Is(err, sql.ErrNoRows) {
			result = Measurement{ID: uuid.NewString(), RecordedOn: input.RecordedOn, WeightG: input.WeightG, WaistMM: input.WaistMM, Source: "healthkit", Version: 1, CreatedAt: now, UpdatedAt: now}
			result.HealthKitUUID = stringPointer(input.HealthKitUUID)
			if err := tx.Exec("INSERT INTO measurements(id, user_id, recorded_on, weight_g, waist_mm, note, source, healthkit_uuid, version, client_mutation_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?, '', 'healthkit', ?, 1, ?, ?, ?)", result.ID, userID, result.RecordedOn, result.WeightG, result.WaistMM, input.HealthKitUUID, input.ClientMutationID, now, now).Error; err != nil {
				return err
			}
			if err := recordMutation(tx, userID, input.ClientMutationID, result.ID, now); err != nil {
				return err
			}
			return recordChange(tx, userID, result.ID, now)
		}
		if err != nil {
			return err
		}
		if source != "healthkit" {
			result, err = scanMeasurement(tx.Raw("SELECT "+measurementColumns+" FROM measurements WHERE id = ?", id).Row())
			if err != nil {
				return err
			}
			return recordMutation(tx, userID, input.ClientMutationID, result.ID, now)
		}
		if err := tx.Exec("UPDATE measurements SET weight_g = ?, waist_mm = ?, healthkit_uuid = ?, version = version + 1, client_mutation_id = ?, updated_at = ? WHERE id = ?", input.WeightG, input.WaistMM, input.HealthKitUUID, input.ClientMutationID, now, id).Error; err != nil {
			return err
		}
		result, err = scanMeasurement(tx.Raw("SELECT "+measurementColumns+" FROM measurements WHERE id = ?", id).Row())
		if err != nil {
			return err
		}
		if err := recordMutation(tx, userID, input.ClientMutationID, id, now); err != nil {
			return err
		}
		return recordChange(tx, userID, id, now)
	})
	return result, err
}

func (service *Service) ListChanges(userID string, cursor string, limit int) (ChangePage, error) {
	sequence, err := parseSyncCursor(cursor)
	if err != nil {
		return ChangePage{}, err
	}
	if limit < 1 || limit > 100 {
		limit = 50
	}
	rows, err := service.db.Raw(`SELECT c.sequence, `+measurementColumns+` FROM measurement_changes c
		JOIN measurements m ON m.id = c.measurement_id
		WHERE c.user_id = ? AND c.sequence > ? ORDER BY c.sequence ASC LIMIT ?`, userID, sequence, limit+1).Rows()
	if err != nil {
		return ChangePage{}, err
	}
	defer rows.Close()
	page := ChangePage{Measurements: make([]Measurement, 0, limit)}
	for rows.Next() {
		var nextSequence int64
		item, err := scanChangedMeasurement(rows, &nextSequence)
		if err != nil {
			return ChangePage{}, err
		}
		if len(page.Measurements) == limit {
			page.HasMore = true
			break
		}
		page.Measurements = append(page.Measurements, item)
		page.NextCursor = formatSyncCursor(nextSequence)
	}
	if err := rows.Err(); err != nil {
		return ChangePage{}, err
	}
	if page.NextCursor == "" {
		page.NextCursor = cursor
	}
	return page, nil
}

func (service *Service) List(userID string, from string, to string, limit int) ([]Measurement, error) {
	if limit < 1 || limit > 100 {
		limit = 50
	}
	query := "SELECT " + measurementColumns + " FROM measurements WHERE user_id = ? AND deleted_at IS NULL"
	args := []any{userID}
	if from != "" {
		query += " AND recorded_on >= ?"
		args = append(args, from)
	}
	if to != "" {
		query += " AND recorded_on <= ?"
		args = append(args, to)
	}
	query += " ORDER BY recorded_on DESC, id DESC LIMIT ?"
	args = append(args, limit)
	rows, err := service.db.Raw(query, args...).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Measurement, 0)
	for rows.Next() {
		item, err := scanMeasurement(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (service *Service) Get(userID string, id string) (Measurement, error) {
	item, err := scanMeasurement(service.db.Raw("SELECT "+measurementColumns+" FROM measurements WHERE user_id = ? AND id = ?", userID, id).Row())
	if errors.Is(err, sql.ErrNoRows) {
		return Measurement{}, ErrNotFound
	}
	return item, err
}

func (service *Service) Update(userID string, id string, expectedVersion int, input MeasurementInput) (Measurement, error) {
	if expectedVersion < 1 || input.RecordedOn != "" {
		return Measurement{}, ErrConflict
	}
	if err := validateMeasurement(MeasurementInput{RecordedOn: "2000-01-01", WeightG: input.WeightG, WaistMM: input.WaistMM, Note: input.Note}); err != nil {
		return Measurement{}, err
	}
	now := service.now().UTC()
	var updated Measurement
	err := service.transaction(func(tx *gorm.DB) error {
		if previous, processed, err := findMutation(tx, userID, input.ClientMutationID); err != nil {
			return err
		} else if processed {
			updated = previous
			return nil
		}
		result := tx.Exec("UPDATE measurements SET weight_g = ?, waist_mm = ?, note = ?, source = 'manual', healthkit_uuid = NULL, version = version + 1, client_mutation_id = ?, updated_at = ? WHERE id = ? AND user_id = ? AND deleted_at IS NULL AND version = ?", input.WeightG, input.WaistMM, strings.TrimSpace(input.Note), input.ClientMutationID, now, id, userID, expectedVersion)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrConflict
		}
		measurement, err := scanMeasurement(tx.Raw("SELECT "+measurementColumns+" FROM measurements WHERE id = ?", id).Row())
		if err != nil {
			return err
		}
		updated = measurement
		if err := recordMutation(tx, userID, input.ClientMutationID, id, now); err != nil {
			return err
		}
		return recordChange(tx, userID, id, now)
	})
	if errors.Is(err, ErrConflict) {
		if _, err := service.Get(userID, id); errors.Is(err, ErrNotFound) {
			return Measurement{}, ErrNotFound
		}
		return Measurement{}, ErrConflict
	}
	return updated, err
}

func (service *Service) Delete(userID string, id string, expectedVersion int, mutationID string) error {
	if expectedVersion < 1 {
		return ErrConflict
	}
	now := service.now().UTC()
	err := service.transaction(func(tx *gorm.DB) error {
		if _, processed, err := findMutation(tx, userID, mutationID); err != nil {
			return err
		} else if processed {
			return nil
		}
		result := tx.Exec("UPDATE measurements SET deleted_at = ?, updated_at = ?, version = version + 1, client_mutation_id = ? WHERE id = ? AND user_id = ? AND deleted_at IS NULL AND version = ?", now, now, mutationID, id, userID, expectedVersion)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrConflict
		}
		if err := recordMutation(tx, userID, mutationID, id, now); err != nil {
			return err
		}
		return recordChange(tx, userID, id, now)
	})
	if errors.Is(err, ErrConflict) {
		if _, err := service.Get(userID, id); errors.Is(err, ErrNotFound) {
			return ErrNotFound
		}
		return ErrConflict
	}
	return err
}

func (service *Service) Statistics(userID string, from string) (Statistics, error) {
	query := "SELECT COUNT(*), MIN(weight_g), MAX(weight_g), AVG(weight_g) FROM measurements WHERE user_id = ? AND deleted_at IS NULL"
	args := []any{userID}
	if from != "" {
		query += " AND recorded_on >= ?"
		args = append(args, from)
	}
	var count int
	var first, last, average sql.NullInt64
	if err := service.db.Raw(query, args...).Row().Scan(&count, &first, &last, &average); err != nil {
		return Statistics{}, err
	}
	if count == 0 {
		return Statistics{}, nil
	}
	var firstWeight, lastWeight int
	firstQuery := "SELECT weight_g FROM measurements WHERE user_id = ? AND deleted_at IS NULL"
	lastQuery := firstQuery
	if from != "" {
		firstQuery += " AND recorded_on >= ?"
		lastQuery += " AND recorded_on >= ?"
	}
	firstQuery += " ORDER BY recorded_on ASC, id ASC LIMIT 1"
	lastQuery += " ORDER BY recorded_on DESC, id DESC LIMIT 1"
	if err := service.db.Raw(firstQuery, args...).Row().Scan(&firstWeight); err != nil {
		return Statistics{}, err
	}
	if err := service.db.Raw(lastQuery, args...).Row().Scan(&lastWeight); err != nil {
		return Statistics{}, err
	}
	return Statistics{Count: count, FirstWeightG: firstWeight, LastWeightG: lastWeight, ChangeG: lastWeight - firstWeight, AverageWeightG: int(average.Int64)}, nil
}

func (service *Service) ExportCSV(userID string) ([]byte, error) {
	rows, err := service.db.Raw("SELECT recorded_on, weight_g, waist_mm, note, source, version, strftime('%Y-%m-%dT%H:%M:%fZ', created_at), strftime('%Y-%m-%dT%H:%M:%fZ', updated_at) FROM measurements WHERE user_id = ? AND deleted_at IS NULL ORDER BY recorded_on ASC, id ASC", userID).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var output bytes.Buffer
	writer := csv.NewWriter(&output)
	if err := writer.Write([]string{"recorded_on", "weight_g", "waist_mm", "note", "source", "version", "created_at", "updated_at"}); err != nil {
		return nil, err
	}
	for rows.Next() {
		var date, note, source, created, updated string
		var weight, version int
		var waist sql.NullInt64
		if err := rows.Scan(&date, &weight, &waist, &note, &source, &version, &created, &updated); err != nil {
			return nil, err
		}
		waistValue := ""
		if waist.Valid {
			waistValue = fmt.Sprintf("%d", waist.Int64)
		}
		if err := writer.Write([]string{date, fmt.Sprintf("%d", weight), waistValue, note, source, fmt.Sprintf("%d", version), created, updated}); err != nil {
			return nil, err
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	writer.Flush()
	return output.Bytes(), writer.Error()
}

func (service *Service) transaction(work func(*gorm.DB) error) error {
	return service.retryWrite(func() error { return service.db.Transaction(work) })
}

func (service *Service) retryWrite(operation func() error) error {
	var err error
	for attempt := 0; attempt < sqliteWriteAttempts; attempt++ {
		err = operation()
		if !isSQLiteBusy(err) {
			return err
		}
		time.Sleep(time.Duration(attempt+1) * 10 * time.Millisecond)
	}
	if isSQLiteBusy(err) {
		return fmt.Errorf("%w: %v", ErrStorageBusy, err)
	}
	return err
}

func isSQLiteBusy(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "database is locked") || strings.Contains(message, "database is busy")
}

func validateProfile(profile Profile) error {
	if profile.HeightMM != nil && (*profile.HeightMM < 500 || *profile.HeightMM > 3000) {
		return fmt.Errorf("身高超出合理范围")
	}
	if profile.TargetWeightG != nil && (*profile.TargetWeightG < 10000 || *profile.TargetWeightG > 500000) {
		return fmt.Errorf("目标体重超出合理范围")
	}
	if profile.PreferredUnit != "kg" && profile.PreferredUnit != "jin" {
		return fmt.Errorf("体重单位无效")
	}
	if strings.TrimSpace(profile.Timezone) == "" {
		return fmt.Errorf("时区不能为空")
	}
	return nil
}

func validateMeasurement(input MeasurementInput) error {
	if _, err := time.Parse(time.DateOnly, input.RecordedOn); err != nil {
		return fmt.Errorf("记录日期无效")
	}
	if input.WeightG < 10000 || input.WeightG > 500000 {
		return fmt.Errorf("体重超出合理范围")
	}
	if input.WaistMM != nil && (*input.WaistMM < 100 || *input.WaistMM > 3000) {
		return fmt.Errorf("腰围超出合理范围")
	}
	if utf8.RuneCountInString(input.Note) > 500 {
		return fmt.Errorf("备注过长")
	}
	return nil
}

func validateHealthKitMeasurement(input HealthKitMeasurementInput) error {
	if err := validateMeasurement(MeasurementInput{RecordedOn: input.RecordedOn, WeightG: input.WeightG, WaistMM: input.WaistMM}); err != nil {
		return err
	}
	if length := utf8.RuneCountInString(strings.TrimSpace(input.HealthKitUUID)); length < 8 || length > 128 {
		return fmt.Errorf("HealthKit 样本标识无效")
	}
	return nil
}

type rowScanner interface{ Scan(...any) error }

func scanMeasurement(row rowScanner) (Measurement, error) {
	var item Measurement
	var waist sql.NullInt64
	var healthKitUUID sql.NullString
	var deleted sql.NullString
	var created, updated string
	err := row.Scan(&item.ID, &item.RecordedOn, &item.WeightG, &waist, &item.Note, &item.Source, &healthKitUUID, &item.Version, &deleted, &created, &updated)
	if err != nil {
		return Measurement{}, err
	}
	item.WaistMM = nullableInt(waist)
	item.HealthKitUUID = nullableString(healthKitUUID)
	var parseErr error
	if deleted.Valid {
		item.DeletedAt, parseErr = parseTimePointer(deleted.String)
		if parseErr != nil {
			return Measurement{}, parseErr
		}
	}
	item.CreatedAt, parseErr = parseTime(created)
	if parseErr != nil {
		return Measurement{}, parseErr
	}
	item.UpdatedAt, parseErr = parseTime(updated)
	return item, parseErr
}

func nullableInt(value sql.NullInt64) *int {
	if !value.Valid {
		return nil
	}
	result := int(value.Int64)
	return &result
}

func nullableString(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	return stringPointer(value.String)
}

func stringPointer(value string) *string { return &value }

func recordChange(tx *gorm.DB, userID string, measurementID string, changedAt time.Time) error {
	return tx.Exec("INSERT INTO measurement_changes(user_id, measurement_id, changed_at) VALUES (?, ?, ?)", userID, measurementID, changedAt).Error
}

func findMutation(tx *gorm.DB, userID string, mutationID string) (Measurement, bool, error) {
	if strings.TrimSpace(mutationID) == "" {
		return Measurement{}, false, nil
	}
	var measurementID string
	err := tx.Raw("SELECT measurement_id FROM client_mutations WHERE user_id = ? AND mutation_id = ?", userID, mutationID).Row().Scan(&measurementID)
	if errors.Is(err, sql.ErrNoRows) {
		return Measurement{}, false, nil
	}
	if err != nil {
		return Measurement{}, false, err
	}
	measurement, err := scanMeasurement(tx.Raw("SELECT "+measurementColumns+" FROM measurements WHERE user_id = ? AND id = ?", userID, measurementID).Row())
	return measurement, true, err
}

func recordMutation(tx *gorm.DB, userID string, mutationID string, measurementID string, createdAt time.Time) error {
	if strings.TrimSpace(mutationID) == "" {
		return nil
	}
	return tx.Exec("INSERT INTO client_mutations(user_id, mutation_id, measurement_id, created_at) VALUES (?, ?, ?, ?)", userID, mutationID, measurementID, createdAt).Error
}

func formatSyncCursor(sequence int64) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatInt(sequence, 10)))
}

func parseSyncCursor(cursor string) (int64, error) {
	if cursor == "" {
		return 0, nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return 0, fmt.Errorf("同步游标无效")
	}
	sequence, err := strconv.ParseInt(string(decoded), 10, 64)
	if err != nil || sequence < 1 {
		return 0, fmt.Errorf("同步游标无效")
	}
	return sequence, nil
}

func scanChangedMeasurement(row rowScanner, sequence *int64) (Measurement, error) {
	var item Measurement
	var waist sql.NullInt64
	var healthKitUUID sql.NullString
	var deleted sql.NullString
	var created, updated string
	err := row.Scan(sequence, &item.ID, &item.RecordedOn, &item.WeightG, &waist, &item.Note, &item.Source, &healthKitUUID, &item.Version, &deleted, &created, &updated)
	if err != nil {
		return Measurement{}, err
	}
	item.WaistMM = nullableInt(waist)
	item.HealthKitUUID = nullableString(healthKitUUID)
	var parseErr error
	if deleted.Valid {
		item.DeletedAt, parseErr = parseTimePointer(deleted.String)
		if parseErr != nil {
			return Measurement{}, parseErr
		}
	}
	item.CreatedAt, parseErr = parseTime(created)
	if parseErr != nil {
		return Measurement{}, parseErr
	}
	item.UpdatedAt, parseErr = parseTime(updated)
	return item, parseErr
}

func parseTime(value string) (time.Time, error) { return time.Parse(time.RFC3339Nano, value) }
func parseTimePointer(value string) (*time.Time, error) {
	parsed, err := parseTime(value)
	return &parsed, err
}
