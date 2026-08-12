package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/auth"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/database"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/version"
)

func TestAuthenticationHTTPFlow(t *testing.T) {
	db, err := database.Open(context.Background(), filepath.Join(t.TempDir(), "api.db"))
	if err != nil { t.Fatal(err) }
	defer db.Close()
	tokens, err := auth.NewTokenManager([]byte("01234567890123456789012345678901"), 15*time.Minute)
	if err != nil { t.Fatal(err) }
	service, err := auth.NewService(auth.NewSQLiteRepository(db), tokens, 30*24*time.Hour)
	if err != nil { t.Fatal(err) }
	if _, err := service.CreateUser(context.Background(), "owner", "Owner", "correct horse battery staple", auth.RoleAdmin); err != nil { t.Fatal(err) }
	handler := New(Dependencies{Build: version.Info{Version: "test"}, Auth: service, LoginRateLimit: 5, Ready: func() error { return nil }})

	login := performJSON(handler, http.MethodPost, "/api/v1/auth/login", map[string]any{"username": "OWNER", "password": "correct horse battery staple", "device": map[string]any{"id": "device", "name": "PC", "platform": "windows", "appVersion": "1.0.0"}}, "")
	if login.Code != http.StatusOK { t.Fatalf("login status = %d, body = %s", login.Code, login.Body.String()) }
	var pair tokenResponse
	if err := json.Unmarshal(login.Body.Bytes(), &pair); err != nil { t.Fatal(err) }
	if pair.AccessToken == "" || pair.RefreshToken == "" || pair.RequestID == "" { t.Fatalf("login response = %#v", pair) }

	sessions := performJSON(handler, http.MethodGet, "/api/v1/auth/sessions", nil, pair.AccessToken)
	if sessions.Code != http.StatusOK { t.Fatalf("sessions status = %d, body = %s", sessions.Code, sessions.Body.String()) }
	logout := performJSON(handler, http.MethodPost, "/api/v1/auth/logout", nil, pair.AccessToken)
	if logout.Code != http.StatusOK { t.Fatalf("logout status = %d", logout.Code) }
	revoked := performJSON(handler, http.MethodGet, "/api/v1/auth/sessions", nil, pair.AccessToken)
	if revoked.Code != http.StatusUnauthorized { t.Fatalf("revoked status = %d", revoked.Code) }
}

func performJSON(handler http.Handler, method, path string, body any, accessToken string) *httptest.ResponseRecorder {
	var encoded []byte
	if body != nil { encoded, _ = json.Marshal(body) }
	request := httptest.NewRequest(method, path, bytes.NewReader(encoded))
	request.RemoteAddr = "192.0.2.1:1234"
	if accessToken != "" { request.Header.Set("Authorization", "Bearer "+accessToken) }
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

