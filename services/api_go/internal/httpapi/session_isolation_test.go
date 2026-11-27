package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/auth"
)

func TestPasswordLoginAndRefreshReplayAreIsolatedBetweenDevices(t *testing.T) {
	service, _, closeDB := testAuthService(t)
	defer closeDB()
	if _, err := service.CreateUser(context.Background(), "owner", "Owner", "OwnerPass!", auth.RoleUser); err != nil {
		t.Fatal(err)
	}
	handler := New(Dependencies{Auth: service, LoginRateLimit: 20})
	login := func(device auth.Device) (*httptest.ResponseRecorder, tokenResponse) {
		t.Helper()
		response := performJSON(handler, http.MethodPost, "/api/v1/auth/login", map[string]any{
			"username": "owner", "password": "OwnerPass!", "device": device,
		}, "")
		if response.Code != http.StatusOK {
			t.Fatalf("login %s status = %d: %s", device.ID, response.Code, response.Body.String())
		}
		var pair tokenResponse
		if err := json.Unmarshal(response.Body.Bytes(), &pair); err != nil {
			t.Fatal(err)
		}
		return response, pair
	}
	assertAccess := func(token string, want int) {
		t.Helper()
		response := performJSON(handler, http.MethodGet, "/api/v1/auth/sessions", nil, token)
		if response.Code != want {
			t.Fatalf("sessions status = %d, want %d: %s", response.Code, want, response.Body.String())
		}
	}
	refreshWeb := func(cookies []*http.Cookie, want int) (*httptest.ResponseRecorder, tokenResponse) {
		t.Helper()
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, webRequest(http.MethodPost, "/api/v1/auth/refresh", nil, cookies))
		if response.Code != want {
			t.Fatalf("web refresh status = %d, want %d: %s", response.Code, want, response.Body.String())
		}
		var pair tokenResponse
		if response.Code == http.StatusOK {
			if err := json.Unmarshal(response.Body.Bytes(), &pair); err != nil {
				t.Fatal(err)
			}
		}
		return response, pair
	}

	_, android := login(auth.Device{ID: "phone", Name: "Android", Platform: "android", AppVersion: "1.0"})
	webResponse, web := login(auth.Device{ID: "browser", Name: "Web", Platform: "windows", AppVersion: "web-test"})
	assertAccess(android.AccessToken, http.StatusOK)
	assertAccess(web.AccessToken, http.StatusOK)
	if android.RefreshToken == "" || web.RefreshToken != "" {
		t.Fatal("native refresh token and web cookie must remain separate")
	}
	oldCookies := webResponse.Result().Cookies()
	rotatedResponse, _ := refreshWeb(oldCookies, http.StatusOK)
	latestResponse, latestWeb := refreshWeb(rotatedResponse.Result().Cookies(), http.StatusOK)
	_, otherBrowser := login(auth.Device{ID: "other-browser", Name: "Other Web", Platform: "windows", AppVersion: "web-test"})

	// A duplicated request from an old browser tab revokes its own rotated chain.
	refreshWeb(oldCookies, http.StatusUnauthorized)
	assertAccess(latestWeb.AccessToken, http.StatusUnauthorized)
	refreshWeb(latestResponse.Result().Cookies(), http.StatusUnauthorized)
	// Independent logins, including the phone, keep both access and refresh working.
	assertAccess(android.AccessToken, http.StatusOK)
	assertAccess(otherBrowser.AccessToken, http.StatusOK)
	refreshedAndroid := performJSON(handler, http.MethodPost, "/api/v1/auth/refresh", map[string]any{"refreshToken": android.RefreshToken}, "")
	if refreshedAndroid.Code != http.StatusOK {
		t.Fatalf("phone refresh status = %d: %s", refreshedAndroid.Code, refreshedAndroid.Body.String())
	}
}

func TestCDKRotationAndRevocationStillInvalidateEveryDevice(t *testing.T) {
	for _, operation := range []string{"rotate", "revoke"} {
		t.Run(operation, func(t *testing.T) {
			service, _, closeDB := testAuthService(t)
			defer closeDB()
			ctx := context.Background()
			keys, err := service.CreateCDKs(ctx, 1, 10)
			if err != nil {
				t.Fatal(err)
			}
			first, err := service.LoginCDK(ctx, keys[0].Code, auth.Device{ID: "phone", Name: "Android", Platform: "android", AppVersion: "1"})
			if err != nil {
				t.Fatal(err)
			}
			second, err := service.LoginCDK(ctx, keys[0].Code, auth.Device{ID: "web", Name: "Web", Platform: "windows", AppVersion: "web-test"})
			if err != nil {
				t.Fatal(err)
			}
			for _, pair := range []auth.TokenPair{first, second} {
				if _, err := service.ValidateAccess(ctx, pair.AccessToken); err != nil {
					t.Fatalf("independent CDK login invalidated a device: %v", err)
				}
			}
			if operation == "rotate" {
				_, err = service.RotateCDK(ctx, keys[0].ID)
			} else {
				_, err = service.RevokeCDK(ctx, keys[0].ID)
			}
			if err != nil {
				t.Fatal(err)
			}
			for _, pair := range []auth.TokenPair{first, second} {
				if _, err := service.ValidateAccess(ctx, pair.AccessToken); err == nil {
					t.Fatal("CDK operation left a device session active")
				}
			}
		})
	}
}
