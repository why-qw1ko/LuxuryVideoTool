package httpapi

import (
	"context"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/auth"
)

func TestAPICatalogMatchesRegisteredRoutes(t *testing.T) {
	mux := newRouteMux(Dependencies{Auth: &auth.Service{}})
	registered := explicitAPIRoutePatterns(t)
	seen := make(map[string]bool, len(apiCatalog))
	parameter := regexp.MustCompile(`\{[^}]+\}`)
	for _, operation := range apiCatalog {
		pattern := operation.Method + " " + operation.Path
		if seen[pattern] {
			t.Errorf("duplicate catalog operation %s", pattern)
		}
		seen[pattern] = true
		if operation.Group == "" || operation.Summary == "" {
			t.Errorf("missing group or summary for %s", pattern)
		}
		switch operation.Auth {
		case apiAuthPublic, apiAuthBearer, apiAuthAdmin, apiAuthRefresh, apiAuthSigned:
		default:
			t.Errorf("invalid auth type %q for %s", operation.Auth, pattern)
		}
		request := httptest.NewRequest(operation.Method, parameter.ReplaceAllString(operation.Path, "catalog-test-id"), nil)
		if _, matched := mux.Handler(request); matched != pattern {
			t.Errorf("router matched %q for catalog operation %q", matched, pattern)
		}
		if !registered[pattern] {
			t.Errorf("catalog operation %q has no explicit route registration", pattern)
		}
	}
	for pattern := range registered {
		if !seen[pattern] {
			t.Errorf("registered API route %q is missing from the catalog", pattern)
		}
	}
}

func TestAPICatalogIncludesCurlExamples(t *testing.T) {
	examples := make(map[string]string)
	for _, operation := range apiCatalog {
		if operation.CurlExample != "" {
			examples[operation.Method+" "+operation.Path] = operation.CurlExample
		}
	}
	for pattern, want := range map[string]string{
		"POST /api/v1/jobs":                                    curlCreateJob,
		"GET /api/v1/jobs/{id}":                                curlRequest,
		"GET /api/v1/jobs/{id}/images/archive":                 curlDownloadZIP,
		"POST /api/v1/jobs/{id}/images/archive-download":       curlRequest,
		"POST /api/v1/admin/jobs/{id}/images/archive-download": curlRequest,
		"GET /api/v1/archive-download/{id}":                    curlSignedZIP,
	} {
		if got := examples[pattern]; got != want {
			t.Errorf("curl example for %s = %q, want %q", pattern, got, want)
		}
	}
}

func explicitAPIRoutePatterns(t *testing.T) map[string]bool {
	t.Helper()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	patterns := make(map[string]bool)
	fileSet := token.NewFileSet()
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fileSet, name, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok || len(call.Args) == 0 {
				return true
			}
			switch function := call.Fun.(type) {
			case *ast.SelectorExpr:
				if function.Sel.Name != "Handle" && function.Sel.Name != "HandleFunc" {
					return true
				}
			case *ast.Ident:
				if function.Name != "register" {
					return true
				}
			default:
				return true
			}
			literal, ok := call.Args[0].(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				return true
			}
			pattern, err := strconv.Unquote(literal.Value)
			if err != nil {
				t.Fatal(err)
			}
			_, path, ok := strings.Cut(pattern, " ")
			if ok && (strings.HasPrefix(path, "/api/") || strings.HasPrefix(path, "/health/")) {
				patterns[pattern] = true
			}
			return true
		})
	}
	return patterns
}

func TestAPICatalogAccess(t *testing.T) {
	service, _, closeDB := testAuthService(t)
	t.Cleanup(closeDB)
	handler := New(Dependencies{Auth: service})
	ctx := context.Background()
	const password = "CatalogTest!"
	tokenFor := func(username string, role auth.Role) string {
		t.Helper()
		if _, err := service.CreateUser(ctx, username, username, password, role); err != nil {
			t.Fatal(err)
		}
		pair, err := service.Login(ctx, auth.LoginInput{Username: username, Password: password, Device: auth.Device{
			ID: "catalog-test-device", Name: "Catalog test", Platform: "windows", AppVersion: "1",
		}})
		if err != nil {
			t.Fatal(err)
		}
		return pair.AccessToken
	}
	const path = "/api/v1/admin/api-catalog"
	for _, access := range []struct {
		name   string
		token  string
		status int
	}{
		{name: "anonymous", status: http.StatusUnauthorized},
		{name: "user", token: tokenFor("catalog-user", auth.RoleUser), status: http.StatusForbidden},
		{name: "admin", token: tokenFor("catalog-admin", auth.RoleAdmin), status: http.StatusOK},
	} {
		t.Run(access.name, func(t *testing.T) {
			response := performJSON(handler, http.MethodGet, path, nil, access.token)
			if response.Code != access.status {
				t.Fatalf("catalog status = %d, want %d", response.Code, access.status)
			}
			if access.status != http.StatusOK {
				if strings.Contains(response.Body.String(), `"operations"`) {
					t.Fatal("unauthorized response included catalog operations")
				}
				return
			}
			if got := response.Header().Get("Cache-Control"); got != "private, no-store" {
				t.Errorf("catalog Cache-Control = %q", got)
			}
			var payload struct {
				Operations []map[string]string `json:"operations"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
				t.Fatal(err)
			}
			if len(payload.Operations) != len(apiCatalog) {
				t.Fatalf("catalog operations = %d, want %d", len(payload.Operations), len(apiCatalog))
			}
			for i, operation := range apiCatalog {
				item := payload.Operations[i]
				if item["method"] != operation.Method || item["path"] != operation.Path ||
					item["group"] != operation.Group || item["summary"] != operation.Summary ||
					item["auth"] != operation.Auth || item["details"] != operation.Details ||
					item["curlExample"] != operation.CurlExample {
					t.Errorf("invalid catalog JSON for %s %s: %v", operation.Method, operation.Path, item)
				}
			}
		})
	}
}
