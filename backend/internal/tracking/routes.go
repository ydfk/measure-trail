package tracking

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/ydfk/measure-trail/backend/internal/auth"
)

type profileInput struct {
	Authorization string `header:"Authorization"`
	Body          struct {
		HeightMM      *int   `json:"heightMm,omitempty"`
		TargetWeightG *int   `json:"targetWeightG,omitempty"`
		PreferredUnit string `json:"preferredUnit" enum:"kg,jin" required:"true"`
		Timezone      string `json:"timezone" minLength:"1" maxLength:"128" required:"true"`
	}
}

type accessInput struct {
	Authorization string `header:"Authorization"`
}

type profileOutput struct{ Body profileBody }
type profileBody struct {
	HeightMM      *int   `json:"heightMm"`
	TargetWeightG *int   `json:"targetWeightG"`
	PreferredUnit string `json:"preferredUnit"`
	Timezone      string `json:"timezone"`
	UpdatedAt     string `json:"updatedAt,omitempty"`
}

type measurementInput struct {
	Authorization string `header:"Authorization"`
	Date          string `path:"date"`
	Body          struct {
		WeightG          int    `json:"weightG" minimum:"10000" maximum:"500000" required:"true"`
		WaistMM          *int   `json:"waistMm,omitempty" minimum:"100" maximum:"3000"`
		Note             string `json:"note,omitempty" maxLength:"500"`
		ClientMutationID string `json:"clientMutationId" minLength:"8" maxLength:"128" required:"true"`
	}
}

type healthKitMeasurementInput struct {
	Authorization string `header:"Authorization"`
	Body          struct {
		RecordedOn       string `json:"recordedOn" format:"date" required:"true"`
		WeightG          int    `json:"weightG" minimum:"10000" maximum:"500000" required:"true"`
		WaistMM          *int   `json:"waistMm,omitempty" minimum:"100" maximum:"3000"`
		HealthKitUUID    string `json:"healthkitUuid" minLength:"8" maxLength:"128" required:"true"`
		ClientMutationID string `json:"clientMutationId" minLength:"8" maxLength:"128" required:"true"`
	}
}

type measurementIDInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" required:"true"`
}

type updateMeasurementInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" required:"true"`
	Body          struct {
		WeightG          int    `json:"weightG" minimum:"10000" maximum:"500000" required:"true"`
		WaistMM          *int   `json:"waistMm,omitempty" minimum:"100" maximum:"3000"`
		Note             string `json:"note,omitempty" maxLength:"500"`
		ExpectedVersion  int    `json:"expectedVersion" minimum:"1" required:"true"`
		ClientMutationID string `json:"clientMutationId" minLength:"8" maxLength:"128" required:"true"`
	}
}

type deleteMeasurementInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" required:"true"`
	Body          struct {
		ExpectedVersion  int    `json:"expectedVersion" minimum:"1" required:"true"`
		ClientMutationID string `json:"clientMutationId" minLength:"8" maxLength:"128" required:"true"`
	}
}

type listMeasurementsInput struct {
	Authorization string `header:"Authorization"`
	From          string `query:"from" format:"date"`
	To            string `query:"to" format:"date"`
	Limit         int    `query:"limit" minimum:"1" maximum:"100" default:"50"`
}

type listChangesInput struct {
	Authorization string `header:"Authorization"`
	Cursor        string `query:"cursor" maxLength:"128"`
	Limit         int    `query:"limit" minimum:"1" maximum:"100" default:"50"`
}

type statisticsInput struct {
	Authorization string `header:"Authorization"`
	Range         string `query:"range" enum:"7d,30d,90d,all" default:"30d"`
}

type exportOutput struct {
	ContentDisposition string `header:"Content-Disposition"`
	RawBody            []byte `contentType:"text/csv; charset=utf-8"`
}

type measurementOutput struct{ Body measurementBody }
type measurementListOutput struct {
	Body struct {
		Measurements []measurementBody `json:"measurements"`
	}
}
type measurementChangesOutput struct {
	Body struct {
		Measurements []measurementBody `json:"measurements"`
		NextCursor   string            `json:"nextCursor,omitempty"`
		HasMore      bool              `json:"hasMore"`
	}
}
type measurementBody struct {
	ID            string  `json:"id"`
	RecordedOn    string  `json:"recordedOn"`
	WeightG       int     `json:"weightG"`
	WaistMM       *int    `json:"waistMm"`
	Note          string  `json:"note"`
	Source        string  `json:"source"`
	HealthKitUUID *string `json:"healthkitUuid,omitempty"`
	Version       int     `json:"version"`
	DeletedAt     *string `json:"deletedAt,omitempty"`
	CreatedAt     string  `json:"createdAt"`
	UpdatedAt     string  `json:"updatedAt"`
}
type statisticsOutput struct {
	Body struct {
		Count          int `json:"count"`
		FirstWeightG   int `json:"firstWeightG"`
		LastWeightG    int `json:"lastWeightG"`
		ChangeG        int `json:"changeG"`
		AverageWeightG int `json:"averageWeightG"`
	}
}

func RegisterRoutes(api huma.API, service *Service, authService *auth.Service) {
	huma.Register(api, protectedOperation(huma.Operation{OperationID: "get-profile", Method: http.MethodGet, Path: "/api/v1/profile", Summary: "读取资料", Tags: []string{"资料"}}), func(_ context.Context, input *accessInput) (*profileOutput, error) {
		userID, err := auth.UserIDFromAuthorization(authService, input.Authorization)
		if err != nil {
			return nil, unauthorized()
		}
		profile, err := service.Profile(userID)
		if err != nil {
			return nil, huma.Error500InternalServerError("无法读取资料")
		}
		return profileResponse(profile), nil
	})
	huma.Register(api, retryableWriteOperation(huma.Operation{OperationID: "update-profile", Method: http.MethodPatch, Path: "/api/v1/profile", Summary: "更新资料", Tags: []string{"资料"}}), func(_ context.Context, input *profileInput) (*profileOutput, error) {
		userID, err := auth.UserIDFromAuthorization(authService, input.Authorization)
		if err != nil {
			return nil, unauthorized()
		}
		profile, err := service.SaveProfile(userID, Profile{HeightMM: input.Body.HeightMM, TargetWeightG: input.Body.TargetWeightG, PreferredUnit: input.Body.PreferredUnit, Timezone: input.Body.Timezone})
		if err != nil {
			if unavailable := storageUnavailable(err); unavailable != nil {
				return nil, unavailable
			}
			return nil, huma.Error400BadRequest("资料字段无效")
		}
		return profileResponse(profile), nil
	})
	huma.Register(api, conflictableMeasurementWriteOperation(huma.Operation{OperationID: "create-measurement-by-date", Method: http.MethodPut, Path: "/api/v1/measurements/by-date/{date}", Summary: "创建当天记录", Tags: []string{"记录"}}), func(_ context.Context, input *measurementInput) (*measurementOutput, error) {
		userID, err := auth.UserIDFromAuthorization(authService, input.Authorization)
		if err != nil {
			return nil, unauthorized()
		}
		measurement, err := service.CreateByDate(userID, MeasurementInput{RecordedOn: input.Date, WeightG: input.Body.WeightG, WaistMM: input.Body.WaistMM, Note: input.Body.Note, ClientMutationID: input.Body.ClientMutationID})
		return writeMeasurement(measurement, err)
	})
	huma.Register(api, retryableWriteOperation(huma.Operation{OperationID: "import-healthkit-measurement", Method: http.MethodPost, Path: "/api/v1/measurements/healthkit", Summary: "导入 HealthKit 每日记录", Tags: []string{"HealthKit"}}), func(_ context.Context, input *healthKitMeasurementInput) (*measurementOutput, error) {
		userID, err := auth.UserIDFromAuthorization(authService, input.Authorization)
		if err != nil {
			return nil, unauthorized()
		}
		measurement, err := service.ImportHealthKitByDate(userID, HealthKitMeasurementInput{RecordedOn: input.Body.RecordedOn, WeightG: input.Body.WeightG, WaistMM: input.Body.WaistMM, HealthKitUUID: input.Body.HealthKitUUID, ClientMutationID: input.Body.ClientMutationID})
		if err != nil {
			if unavailable := storageUnavailable(err); unavailable != nil {
				return nil, unavailable
			}
			return nil, huma.Error400BadRequest("HealthKit 记录字段无效")
		}
		return measurementResponse(measurement), nil
	})
	huma.Register(api, protectedOperation(huma.Operation{OperationID: "list-measurements", Method: http.MethodGet, Path: "/api/v1/measurements", Summary: "列出记录", Tags: []string{"记录"}}), func(_ context.Context, input *listMeasurementsInput) (*measurementListOutput, error) {
		userID, err := auth.UserIDFromAuthorization(authService, input.Authorization)
		if err != nil {
			return nil, unauthorized()
		}
		items, err := service.List(userID, input.From, input.To, input.Limit)
		if err != nil {
			return nil, huma.Error500InternalServerError("无法读取记录")
		}
		output := &measurementListOutput{}
		output.Body.Measurements = make([]measurementBody, 0, len(items))
		for _, item := range items {
			output.Body.Measurements = append(output.Body.Measurements, measurementBodyFrom(item))
		}
		return output, nil
	})
	huma.Register(api, protectedOperation(huma.Operation{OperationID: "list-measurement-changes", Method: http.MethodGet, Path: "/api/v1/measurement-changes", Summary: "按游标拉取记录变更", Tags: []string{"同步"}}), func(_ context.Context, input *listChangesInput) (*measurementChangesOutput, error) {
		userID, err := auth.UserIDFromAuthorization(authService, input.Authorization)
		if err != nil {
			return nil, unauthorized()
		}
		page, err := service.ListChanges(userID, input.Cursor, input.Limit)
		if err != nil {
			return nil, huma.Error400BadRequest("同步游标无效")
		}
		output := &measurementChangesOutput{}
		output.Body.Measurements = make([]measurementBody, 0, len(page.Measurements))
		for _, item := range page.Measurements {
			output.Body.Measurements = append(output.Body.Measurements, measurementBodyFrom(item))
		}
		output.Body.NextCursor = page.NextCursor
		output.Body.HasMore = page.HasMore
		return output, nil
	})
	huma.Register(api, protectedOperation(huma.Operation{OperationID: "get-measurement", Method: http.MethodGet, Path: "/api/v1/measurements/{id}", Summary: "读取记录", Tags: []string{"记录"}}), func(_ context.Context, input *measurementIDInput) (*measurementOutput, error) {
		userID, err := auth.UserIDFromAuthorization(authService, input.Authorization)
		if err != nil {
			return nil, unauthorized()
		}
		item, err := service.Get(userID, input.ID)
		if errors.Is(err, ErrNotFound) {
			return nil, huma.Error404NotFound("记录不存在")
		}
		if err != nil {
			return nil, huma.Error500InternalServerError("无法读取记录")
		}
		return measurementResponse(item), nil
	})
	huma.Register(api, conflictableMeasurementWriteOperation(huma.Operation{OperationID: "update-measurement", Method: http.MethodPatch, Path: "/api/v1/measurements/{id}", Summary: "编辑记录", Tags: []string{"记录"}}), func(_ context.Context, input *updateMeasurementInput) (*measurementOutput, error) {
		userID, err := auth.UserIDFromAuthorization(authService, input.Authorization)
		if err != nil {
			return nil, unauthorized()
		}
		item, err := service.Update(userID, input.ID, input.Body.ExpectedVersion, MeasurementInput{WeightG: input.Body.WeightG, WaistMM: input.Body.WaistMM, Note: input.Body.Note, ClientMutationID: input.Body.ClientMutationID})
		return writeMeasurement(item, err)
	})
	huma.Register(api, conflictableMeasurementWriteOperation(huma.Operation{OperationID: "delete-measurement", Method: http.MethodDelete, Path: "/api/v1/measurements/{id}", Summary: "删除记录", Tags: []string{"记录"}}), func(_ context.Context, input *deleteMeasurementInput) (*struct{}, error) {
		userID, err := auth.UserIDFromAuthorization(authService, input.Authorization)
		if err != nil {
			return nil, unauthorized()
		}
		err = service.Delete(userID, input.ID, input.Body.ExpectedVersion, input.Body.ClientMutationID)
		if errors.Is(err, ErrNotFound) {
			return nil, huma.Error404NotFound("记录不存在")
		}
		if errors.Is(err, ErrConflict) {
			return nil, huma.Error409Conflict("记录已在其他设备更新")
		}
		if unavailable := storageUnavailable(err); unavailable != nil {
			return nil, unavailable
		}
		if err != nil {
			return nil, huma.Error500InternalServerError("无法删除记录")
		}
		return &struct{}{}, nil
	})
	huma.Register(api, protectedOperation(huma.Operation{OperationID: "measurement-statistics", Method: http.MethodGet, Path: "/api/v1/statistics", Summary: "读取统计", Tags: []string{"统计"}}), func(_ context.Context, input *statisticsInput) (*statisticsOutput, error) {
		userID, err := auth.UserIDFromAuthorization(authService, input.Authorization)
		if err != nil {
			return nil, unauthorized()
		}
		from := statisticsFrom(input.Range, time.Now().UTC())
		stats, err := service.Statistics(userID, from)
		if err != nil {
			return nil, huma.Error500InternalServerError("无法读取统计")
		}
		output := &statisticsOutput{}
		output.Body.Count = stats.Count
		output.Body.FirstWeightG = stats.FirstWeightG
		output.Body.LastWeightG = stats.LastWeightG
		output.Body.ChangeG = stats.ChangeG
		output.Body.AverageWeightG = stats.AverageWeightG
		return output, nil
	})
	huma.Register(api, protectedOperation(huma.Operation{OperationID: "export-account-data", Method: http.MethodGet, Path: "/api/v1/account/export.csv", Summary: "导出记录 CSV", Tags: []string{"账号"}}), func(_ context.Context, input *accessInput) (*exportOutput, error) {
		userID, err := auth.UserIDFromAuthorization(authService, input.Authorization)
		if err != nil {
			return nil, unauthorized()
		}
		contents, err := service.ExportCSV(userID)
		if err != nil {
			return nil, huma.Error500InternalServerError("无法导出记录")
		}
		return &exportOutput{ContentDisposition: "attachment; filename=measuretrail-export.csv", RawBody: contents}, nil
	})
}

func unauthorized() error { return huma.Error401Unauthorized("access token 无效") }

func protectedOperation(operation huma.Operation) huma.Operation {
	operation.Security = []map[string][]string{{"bearerAuth": {}}}
	operation.Errors = append(operation.Errors, http.StatusUnauthorized)
	return operation
}
func retryableWriteOperation(operation huma.Operation) huma.Operation {
	operation = protectedOperation(operation)
	operation.Errors = append(operation.Errors, http.StatusServiceUnavailable)
	return operation
}

func conflictableMeasurementWriteOperation(operation huma.Operation) huma.Operation {
	operation = retryableWriteOperation(operation)
	operation.Errors = append(operation.Errors, http.StatusConflict)
	return operation
}
func storageUnavailable(err error) error {
	if errors.Is(err, ErrStorageBusy) {
		return huma.Error503ServiceUnavailable("数据存储暂时繁忙，请稍后重试")
	}
	return nil
}
func profileResponse(profile Profile) *profileOutput {
	output := &profileOutput{Body: profileBody{HeightMM: profile.HeightMM, TargetWeightG: profile.TargetWeightG, PreferredUnit: profile.PreferredUnit, Timezone: profile.Timezone}}
	if !profile.UpdatedAt.IsZero() {
		output.Body.UpdatedAt = profile.UpdatedAt.UTC().Format(time.RFC3339)
	}
	return output
}
func measurementResponse(measurement Measurement) *measurementOutput {
	return &measurementOutput{Body: measurementBodyFrom(measurement)}
}
func measurementBodyFrom(measurement Measurement) measurementBody {
	output := measurementBody{ID: measurement.ID, RecordedOn: measurement.RecordedOn, WeightG: measurement.WeightG, WaistMM: measurement.WaistMM, Note: measurement.Note, Source: measurement.Source, HealthKitUUID: measurement.HealthKitUUID, Version: measurement.Version, CreatedAt: measurement.CreatedAt.UTC().Format(time.RFC3339), UpdatedAt: measurement.UpdatedAt.UTC().Format(time.RFC3339)}
	if measurement.DeletedAt != nil {
		deletedAt := measurement.DeletedAt.UTC().Format(time.RFC3339)
		output.DeletedAt = &deletedAt
	}
	return output
}
func writeMeasurement(measurement Measurement, err error) (*measurementOutput, error) {
	if errors.Is(err, ErrNotFound) {
		return nil, huma.Error404NotFound("记录不存在")
	}
	if errors.Is(err, ErrConflict) {
		return nil, huma.Error409Conflict("记录已在其他设备更新")
	}
	if unavailable := storageUnavailable(err); unavailable != nil {
		return nil, unavailable
	}
	if err != nil {
		return nil, huma.Error400BadRequest("记录字段无效")
	}
	return measurementResponse(measurement), nil
}
func statisticsFrom(rangeValue string, now time.Time) string {
	switch rangeValue {
	case "7d":
		return now.AddDate(0, 0, -6).Format(time.DateOnly)
	case "90d":
		return now.AddDate(0, 0, -89).Format(time.DateOnly)
	case "all":
		return ""
	default:
		return now.AddDate(0, 0, -29).Format(time.DateOnly)
	}
}
