package tracking

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ydfk/measure-trail/backend/internal/config"
	"github.com/ydfk/measure-trail/backend/internal/database"
)

func TestProfileAndMeasurementCreateAndUpdate(t *testing.T) {
	db, err := database.Open(config.Database{Path: filepath.Join(t.TempDir(), "measuretrail.sqlite"), BusyTimeoutMS: 1000})
	if err != nil {
		t.Fatalf("打开数据库: %v", err)
	}
	now := time.Now().UTC()
	if err := db.Exec("INSERT INTO users(id, email, email_verified_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?)", "user-1", "user@example.com", now, now, now).Error; err != nil {
		t.Fatalf("创建测试用户: %v", err)
	}
	service := NewService(db)
	height, target := 1750, 70000
	profile, err := service.SaveProfile("user-1", Profile{HeightMM: &height, TargetWeightG: &target, PreferredUnit: "kg", Timezone: "Asia/Shanghai"})
	if err != nil || profile.UpdatedAt.IsZero() {
		t.Fatalf("保存资料 profile=%#v error=%v", profile, err)
	}
	if loaded, err := service.Profile("user-1"); err != nil || loaded.HeightMM == nil || *loaded.HeightMM != height {
		t.Fatalf("读取资料 profile=%#v error=%v", loaded, err)
	}
	first, err := service.CreateByDate("user-1", MeasurementInput{RecordedOn: "2025-09-26", WeightG: 76120, Note: "晨起"})
	if err != nil {
		t.Fatalf("创建记录: %v", err)
	}
	second, err := service.Update("user-1", first.ID, first.Version, MeasurementInput{WeightG: 76000, Note: "更新"})
	if err != nil {
		t.Fatalf("更新记录: %v", err)
	}
	if first.ID != second.ID || second.Version != 2 || second.WeightG != 76000 {
		t.Fatalf("创建/编辑结果 first=%#v second=%#v", first, second)
	}
	items, err := service.List("user-1", "", "", 50)
	if err != nil || len(items) != 1 || items[0].Note != "更新" {
		t.Fatalf("列表 items=%#v error=%v", items, err)
	}
	export, err := service.ExportCSV("user-1")
	if err != nil || !strings.Contains(string(export), "recorded_on,weight_g") || !strings.Contains(string(export), "2025-09-26,76000") {
		t.Fatalf("导出内容=%q error=%v", export, err)
	}
	firstPage, err := service.ListChanges("user-1", "", 1)
	if err != nil || len(firstPage.Measurements) != 1 || firstPage.NextCursor == "" || !firstPage.HasMore {
		t.Fatalf("首次同步 page=%#v error=%v", firstPage, err)
	}
	secondPage, err := service.ListChanges("user-1", firstPage.NextCursor, 1)
	if err != nil || len(secondPage.Measurements) != 1 || secondPage.Measurements[0].Version != 2 || secondPage.HasMore {
		t.Fatalf("续传同步 page=%#v error=%v", secondPage, err)
	}
	if err := service.Delete("user-1", secondPage.Measurements[0].ID, secondPage.Measurements[0].Version, "delete-0001"); err != nil {
		t.Fatalf("删除记录: %v", err)
	}
	tombstone, err := service.ListChanges("user-1", secondPage.NextCursor, 50)
	if err != nil || len(tombstone.Measurements) != 1 || tombstone.Measurements[0].DeletedAt == nil || tombstone.Measurements[0].Version != 3 {
		t.Fatalf("删除同步 page=%#v error=%v", tombstone, err)
	}
	if _, err := service.ListChanges("user-1", "invalid", 50); err == nil {
		t.Fatal("非法游标未被拒绝")
	}
}

func TestClientMutationIDsAreIdempotent(t *testing.T) {
	db, err := database.Open(config.Database{Path: filepath.Join(t.TempDir(), "measuretrail.sqlite"), BusyTimeoutMS: 1000})
	if err != nil {
		t.Fatalf("打开数据库: %v", err)
	}
	now := time.Now().UTC()
	if err := db.Exec("INSERT INTO users(id, email, email_verified_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?)", "user-1", "user@example.com", now, now, now).Error; err != nil {
		t.Fatalf("创建测试用户: %v", err)
	}
	service := NewService(db)
	created, err := service.CreateByDate("user-1", MeasurementInput{RecordedOn: "2025-09-26", WeightG: 76120, ClientMutationID: "create-0001"})
	if err != nil || created.Version != 1 {
		t.Fatalf("创建记录 measurement=%#v error=%v", created, err)
	}
	createdRetry, err := service.CreateByDate("user-1", MeasurementInput{RecordedOn: "2025-09-26", WeightG: 76120, ClientMutationID: "create-0001"})
	if err != nil || createdRetry.ID != created.ID || createdRetry.Version != 1 {
		t.Fatalf("创建重试 measurement=%#v error=%v", createdRetry, err)
	}
	updated, err := service.Update("user-1", created.ID, 1, MeasurementInput{WeightG: 76000, Note: "更新", ClientMutationID: "update-0001"})
	if err != nil || updated.Version != 2 {
		t.Fatalf("编辑记录 measurement=%#v error=%v", updated, err)
	}
	updatedRetry, err := service.Update("user-1", created.ID, 1, MeasurementInput{WeightG: 76000, Note: "更新", ClientMutationID: "update-0001"})
	if err != nil || updatedRetry.Version != 2 {
		t.Fatalf("编辑重试 measurement=%#v error=%v", updatedRetry, err)
	}
	if err := service.Delete("user-1", created.ID, 2, "delete-0001"); err != nil {
		t.Fatalf("删除记录: %v", err)
	}
	if err := service.Delete("user-1", created.ID, 2, "delete-0001"); err != nil {
		t.Fatalf("删除重试: %v", err)
	}
	changes, err := service.ListChanges("user-1", "", 50)
	if err != nil || len(changes.Measurements) != 3 || changes.Measurements[2].Version != 3 || changes.Measurements[2].DeletedAt == nil {
		t.Fatalf("幂等变更记录 page=%#v error=%v", changes, err)
	}
}

func TestConcurrentSameDayCreatesReturnConflictsWithoutOverwriting(t *testing.T) {
	db, err := database.Open(config.Database{Path: filepath.Join(t.TempDir(), "measuretrail.sqlite"), BusyTimeoutMS: 1000})
	if err != nil {
		t.Fatalf("打开测试数据库: %v", err)
	}
	now := time.Now().UTC()
	if err := db.Exec("INSERT INTO users(id, email, email_verified_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?)", "user-1", "user@example.com", now, now, now).Error; err != nil {
		t.Fatalf("创建测试用户: %v", err)
	}
	service := NewService(db)
	const writers = 4
	start := make(chan struct{})
	results := make(chan error, writers)
	var group sync.WaitGroup
	for writer := range writers {
		group.Add(1)
		go func(writer int) {
			defer group.Done()
			<-start
			_, err := service.CreateByDate("user-1", MeasurementInput{RecordedOn: "2025-09-26", WeightG: 76_000 + writer, ClientMutationID: fmt.Sprintf("concurrent-%d", writer)})
			results <- err
		}(writer)
	}
	close(start)
	group.Wait()
	close(results)
	successes, conflicts := 0, 0
	for err := range results {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrConflict):
			conflicts++
		default:
			t.Fatalf("并发同日创建错误: %v", err)
		}
	}
	if successes != 1 || conflicts != writers-1 {
		t.Fatalf("并发创建 successes=%d conflicts=%d", successes, conflicts)
	}
	items, err := service.List("user-1", "", "", 50)
	if err != nil || len(items) != 1 {
		t.Fatalf("并发后记录=%#v error=%v", items, err)
	}
}

func TestRetryWriteReturnsRetryableStorageErrorAfterBusyExhaustion(t *testing.T) {
	service := &Service{}
	attempts := 0
	err := service.retryWrite(func() error {
		attempts++
		return errors.New("database is locked")
	})
	if !errors.Is(err, ErrStorageBusy) {
		t.Fatalf("重试耗尽 error=%v, want ErrStorageBusy", err)
	}
	if attempts != sqliteWriteAttempts {
		t.Fatalf("尝试次数=%d, want %d", attempts, sqliteWriteAttempts)
	}
}

func TestHealthKitImportDoesNotOverwriteManualMeasurement(t *testing.T) {
	db, err := database.Open(config.Database{Path: filepath.Join(t.TempDir(), "measuretrail.sqlite"), BusyTimeoutMS: 1000})
	if err != nil {
		t.Fatalf("打开测试数据库: %v", err)
	}
	now := time.Now().UTC()
	if err := db.Exec("INSERT INTO users(id, email, email_verified_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?)", "user-1", "user@example.com", now, now, now).Error; err != nil {
		t.Fatalf("创建测试用户: %v", err)
	}
	service := NewService(db)
	first, err := service.ImportHealthKitByDate("user-1", HealthKitMeasurementInput{RecordedOn: "2025-09-26", WeightG: 76120, HealthKitUUID: "healthkit-sample-0001", ClientMutationID: "healthkit-import-0001"})
	if err != nil || first.Source != "healthkit" || first.HealthKitUUID == nil || *first.HealthKitUUID != "healthkit-sample-0001" {
		t.Fatalf("首次 HealthKit 导入 measurement=%#v error=%v", first, err)
	}
	retry, err := service.ImportHealthKitByDate("user-1", HealthKitMeasurementInput{RecordedOn: "2025-09-26", WeightG: 76120, HealthKitUUID: "healthkit-sample-0001", ClientMutationID: "healthkit-import-0002"})
	if err != nil || retry.ID != first.ID || retry.Version != first.Version {
		t.Fatalf("重复 HealthKit 样本 measurement=%#v error=%v", retry, err)
	}
	manual, err := service.Update("user-1", first.ID, first.Version, MeasurementInput{WeightG: 76000, Note: "手工修正", ClientMutationID: "manual-update-0001"})
	if err != nil || manual.Source != "manual" || manual.HealthKitUUID != nil {
		t.Fatalf("手工编辑 measurement=%#v error=%v", manual, err)
	}
	preserved, err := service.ImportHealthKitByDate("user-1", HealthKitMeasurementInput{RecordedOn: "2025-09-26", WeightG: 75000, HealthKitUUID: "healthkit-sample-0002", ClientMutationID: "healthkit-import-0003"})
	if err != nil || preserved.ID != manual.ID || preserved.WeightG != manual.WeightG || preserved.Source != "manual" {
		t.Fatalf("手工记录被 HealthKit 覆盖 measurement=%#v error=%v", preserved, err)
	}
}
