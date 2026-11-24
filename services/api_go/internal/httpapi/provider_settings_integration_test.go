package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/asr"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/auth"
	ownedfiles "github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/files"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/jobs"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/settings"
)

func TestProviderManagementAndJobSelection(t *testing.T) {
	ctx := context.Background()
	authService, db, closeDB := testAuthService(t)
	defer closeDB()
	login := func(name string, role auth.Role) string {
		t.Helper()
		const password = "ASRProvider!"
		if _, err := authService.CreateUser(ctx, name, name, password, role); err != nil {
			t.Fatal(err)
		}
		pair, err := authService.Login(ctx, auth.LoginInput{
			Username: name, Password: password,
			Device: auth.Device{ID: "pc", Name: "PC", Platform: "windows", AppVersion: "1"},
		})
		if err != nil {
			t.Fatal(err)
		}
		return pair.AccessToken
	}
	adminToken, memberToken := login("admin", auth.RoleAdmin), login("member", auth.RoleUser)
	signingKey := []byte("01234567890123456789012345678901")
	store, err := settings.New(db, signingKey)
	if err != nil {
		t.Fatal(err)
	}
	const deploymentKey, savedKey = "deployment-only-test-key", "saved-provider-test-key"
	store.SetFallback(settings.SiliconFlowKey, deploymentKey)
	store.SetFallback(settings.AliyunKey, deploymentKey)
	fake := &asr.Fake{}
	var tested settings.ProviderConfig
	signer := ownedfiles.NewSigner(signingKey, "https://media.example.com")
	handler := New(Dependencies{
		Auth: authService, Jobs: jobs.NewService(jobs.NewSQLiteRepository(db), nil), Settings: store,
		ASRSigner: signer, AllowInsecureProviderSettings: true,
		ASRProviderFactory: func(config settings.ProviderConfig) asr.Provider {
			tested = config
			return fake
		},
	})
	const providersPath = "/api/v1/admin/settings/providers"
	request := func(method, path string, body any, token string, want int) *httptest.ResponseRecorder {
		t.Helper()
		response := performJSON(handler, method, path, body, token)
		if response.Code != want {
			t.Fatalf("%s %s status=%d, want %d: %s", method, path, response.Code, want, response.Body.String())
		}
		if strings.Contains(response.Body.String(), deploymentKey) || strings.Contains(response.Body.String(), savedKey) {
			t.Fatal("provider response exposed credential")
		}
		return response
	}
	readStatus := func(response *httptest.ResponseRecorder) settings.ProviderStatusList {
		t.Helper()
		var payload struct{ Providers settings.ProviderStatusList }
		if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		return payload.Providers
	}
	request(http.MethodGet, providersPath, nil, "", http.StatusUnauthorized)
	request(http.MethodGet, providersPath, nil, memberToken, http.StatusForbidden)
	request(http.MethodPost, providersPath+"/siliconflow/test", nil, memberToken, http.StatusForbidden)
	request(http.MethodDelete, providersPath+"/siliconflow/key", nil, memberToken, http.StatusForbidden)
	first := request(http.MethodGet, providersPath, nil, adminToken, http.StatusOK)
	status := readStatus(first)
	if status.EffectiveDefaultProvider != settings.ProviderSiliconFlow || len(status.Items) != 3 ||
		!status.AliyunConfigured || status.AliyunAvailable || first.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("legacy and new status = %+v", status)
	}
	request(http.MethodPut, providersPath, map[string]any{
		"deepseekApiKey": savedKey, "defaultProvider": "deepseek", "enabledProviders": []string{"siliconflow"},
	}, adminToken, http.StatusBadRequest)
	request(http.MethodPost, providersPath+"/deepseek/test", nil, adminToken, http.StatusConflict)
	request(http.MethodPost, providersPath+"/aliyun/test", nil, adminToken, http.StatusConflict)
	status = readStatus(request(http.MethodPut, providersPath, map[string]any{
		"deepseekApiKey": savedKey, "defaultProvider": "deepseek", "enabledProviders": []string{"siliconflow", "deepseek"},
		"allowUserSelection": false,
	}, adminToken, http.StatusOK))
	if status.AllowUserSelection || status.DefaultProvider != "deepseek" || status.EffectiveDefaultProvider != "deepseek" {
		t.Fatalf("updated status = %+v", status)
	}

	response := request(http.MethodPost, providersPath+"/deepseek/test", nil, adminToken, http.StatusOK)
	if !strings.Contains(response.Body.String(), `"provider":"deepseek"`) || fake.Calls != 1 || tested.APIKey != savedKey {
		t.Fatal("connection probe did not invoke saved provider")
	}
	fake.Err = asr.ErrAuth
	response = request(http.MethodPost, providersPath+"/deepseek/test", nil, adminToken, http.StatusBadGateway)
	if fake.Calls != 2 || !strings.Contains(response.Body.String(), `"code":"ASR_TEST_AUTH"`) {
		t.Fatal("connection probe did not classify auth failure")
	}

	response = request(http.MethodGet, "/api/v1/jobs/usage", nil, memberToken, http.StatusOK)
	var usage struct {
		Usage struct{ ASR settings.ProviderSelection }
	}
	if err := json.Unmarshal(response.Body.Bytes(), &usage); err != nil || usage.Usage.ASR.DefaultProvider != "deepseek" ||
		usage.Usage.ASR.AllowUserSelection || len(usage.Usage.ASR.Providers) != 2 {
		t.Fatalf("job provider selection = %+v, err=%v", usage, err)
	}
	submit := func(action, provider, key string, want int) *httptest.ResponseRecorder {
		t.Helper()
		body, err := json.Marshal(map[string]any{
			"shareText": "https://www.douyin.com/video/123", "action": action,
			"options": map[string]string{"asrProvider": provider},
		})
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+memberToken)
		req.Header.Set("Idempotency-Key", key)
		result := httptest.NewRecorder()
		handler.ServeHTTP(result, req)
		if result.Code != want {
			t.Fatalf("job %q: status=%d, want %d: %s", key, result.Code, want, result.Body.String())
		}
		return result
	}
	submit("download", "deepseek", "invalid-action", http.StatusBadRequest)
	submit("full", "deepseek", "disabled-selection", http.StatusForbidden)
	submit("full", "", "default-allowed", http.StatusAccepted)
	request(http.MethodPut, providersPath, map[string]any{"allowUserSelection": true}, adminToken, http.StatusOK)
	response = submit("full", "deepseek", "explicit", http.StatusAccepted)
	if !strings.Contains(response.Body.String(), `"asrProvider":"deepseek"`) {
		t.Fatal("job did not store the explicit provider")
	}
	submit("full", "siliconflow", "explicit", http.StatusConflict)
	fake.Err = nil
	status = readStatus(request(http.MethodDelete, providersPath+"/deepseek/key", nil, adminToken, http.StatusOK))
	if status.DefaultProvider != "deepseek" || status.EffectiveDefaultProvider != "" {
		t.Fatalf("unavailable explicit default = %+v", status)
	}
	submit("full", "", "configured-default-unavailable", http.StatusForbidden)
	submit("full", "siliconflow", "alternative-allowed", http.StatusAccepted)
	request(http.MethodPut, providersPath, map[string]any{"allowUserSelection": false}, adminToken, http.StatusOK)
	submit("full", "", "locked-default-unavailable", http.StatusForbidden)
	submit("full", "siliconflow", "alternative-locked", http.StatusForbidden)
	request(http.MethodPut, providersPath, map[string]any{"deepseekApiKey": savedKey}, adminToken, http.StatusOK)
	request(http.MethodPut, providersPath, map[string]any{
		"defaultProvider": "", "enabledProviders": []string{"siliconflow"}, "allowUserSelection": true,
	}, adminToken, http.StatusOK)
	submit("full", "deepseek", "inactive-provider", http.StatusForbidden)
	if replay := submit("full", "deepseek", "explicit", http.StatusAccepted); !strings.Contains(replay.Body.String(), `"reused":true`) {
		t.Fatal("idempotent replay cannot be blocked by later settings")
	}
	request(http.MethodPost, providersPath+"/deepseek/test", nil, adminToken, http.StatusOK)
	request(http.MethodDelete, providersPath+"/siliconflow/key", nil, adminToken, http.StatusOK)

	afterRestart, err := settings.New(db, signingKey)
	if err != nil {
		t.Fatal(err)
	}
	afterRestart.SetFallback(settings.SiliconFlowKey, deploymentKey)
	preferences, err := afterRestart.Providers(ctx, false)
	if err != nil || preferences.Config("siliconflow").Configured {
		t.Fatalf("clear did not suppress environment fallback: %v", err)
	}
	submit("full", "", "no-service", http.StatusForbidden)

	sampleURL, err := url.Parse(signer.URL(asr.ConnectionSampleID, time.Now().Add(time.Minute)))
	if err != nil {
		t.Fatal(err)
	}
	request(http.MethodGet, sampleURL.Path, nil, "", http.StatusNotFound)
	sample := request(http.MethodGet, sampleURL.RequestURI(), nil, "", http.StatusOK)
	if sample.Header().Get("Content-Type") != "audio/wav" || !bytes.Equal(sample.Body.Bytes(), asr.ConnectionSampleWAV()) {
		t.Fatal("only the signed synthetic sample may be served")
	}
}
