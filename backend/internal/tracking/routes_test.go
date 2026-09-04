package tracking

import (
	"testing"

	"github.com/danielgtaylor/huma/v2"
)

func TestWriteMeasurementReturnsServiceUnavailableForBusyStorage(t *testing.T) {
	_, err := writeMeasurement(Measurement{}, ErrStorageBusy)
	status, ok := err.(huma.StatusError)
	if !ok {
		t.Fatalf("错误没有 HTTP 状态: %v", err)
	}
	if status.GetStatus() != 503 {
		t.Fatalf("存储繁忙状态=%d, want 503", status.GetStatus())
	}
}
