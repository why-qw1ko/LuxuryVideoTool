package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

type errorEnvelope struct { Error APIError `json:"error"` }
type APIError struct {
	Code      string         `json:"code"`
	Message   string         `json:"message"`
	Retryable bool           `json:"retryable"`
	RequestID string         `json:"requestId"`
	Details   map[string]any `json:"details"`
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		slog.Error("encode response failed", "service", "api", "event", "response_encode_failed", "error_code", "RESPONSE_ENCODE_FAILED")
	}
}

func writeError(w http.ResponseWriter, r *http.Request, status int, code, message string, retryable bool) {
	writeJSON(w, status, errorEnvelope{Error: APIError{Code: code, Message: message, Retryable: retryable, RequestID: RequestID(r.Context()), Details: map[string]any{}}})
}
