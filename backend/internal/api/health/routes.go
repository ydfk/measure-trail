package health

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"gorm.io/gorm"
)

type response struct {
	Status  string `json:"status" enum:"ok" doc:"服务状态"`
	Service string `json:"service" doc:"服务标识"`
	Version string `json:"version" doc:"API 版本"`
}

type output struct {
	Body response
}

func Register(api huma.API, db *gorm.DB) {
	huma.Register(api, huma.Operation{
		OperationID: "get-health",
		Method:      http.MethodGet,
		Path:        "/api/health",
		Summary:     "获取服务健康状态",
		Tags:        []string{"系统"},
	}, func(_ context.Context, _ *struct{}) (*output, error) {
		if err := db.Exec("SELECT 1").Error; err != nil {
			return nil, huma.Error503ServiceUnavailable("数据库不可用")
		}
		return &output{Body: response{Status: "ok", Service: "measuretrail-api", Version: "0.1.0"}}, nil
	})
}
