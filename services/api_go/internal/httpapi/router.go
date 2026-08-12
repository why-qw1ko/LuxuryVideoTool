package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/version"
)

type healthResponse struct {
	Status  string       `json:"status"`
	Version version.Info `json:"version"`
}

func New(build version.Info) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health/live", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(healthResponse{Status: "ok", Version: build})
	})
	return mux
}

