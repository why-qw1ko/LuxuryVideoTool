package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/version"
)

func TestLiveHealth(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/health/live", nil)

	New(version.Info{Version: "test"}).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if body := recorder.Body.String(); !strings.Contains(body, `"status":"ok"`) {
		t.Fatalf("body = %q, want status ok", body)
	}
}

