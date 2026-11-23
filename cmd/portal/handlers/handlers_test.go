package handlers

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/gin-gonic/gin"

	"github.com/h4r5h6l/kubernetes_demo/cmd/portal/config"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	os.Exit(m.Run())
}

func newTestHandler(t *testing.T) *gin.Engine {
	t.Helper()

	// The test runs from cmd/portal/handlers, so ../ is cmd/portal: the same
	// templates and static files embedded into the production binary.
	h, err := New(config.Config{
		AppEnv:         "test",
		Port:           "8080",
		TaskManagerURL: "http://taskmanager:80",
	}, os.DirFS("../"))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	router := gin.New()
	h.RegisterRoutes(router)
	return router
}

func get(t *testing.T, router http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestDashboardRendersHTML(t *testing.T) {
	rec := get(t, newTestHandler(t), "/")

	if rec.Code != http.StatusOK {
		t.Fatalf("GET / status = %d, want %d", rec.Code, http.StatusOK)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}

	body := rec.Body.String()
	for _, want := range []string{
		"FinOps",
		"SecOps Portal",
		"http://taskmanager:80",
		"/static/htmx.min.js",
		"Security scans",
		"Drift detection",
		"Cost estimate",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("dashboard body missing %q", want)
		}
	}
}

func TestStatusPartialRendersHXAttributes(t *testing.T) {
	rec := get(t, newTestHandler(t), "/partials/status")

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /partials/status status = %d, want %d", rec.Code, http.StatusOK)
	}

	body := rec.Body.String()
	for _, want := range []string{
		`id="snapshot"`,
		`hx-get="/partials/status"`,
		`hx-swap="outerHTML"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("status partial missing %q", want)
		}
	}
	if strings.Contains(body, "FinOps & SecOps Portal") {
		t.Error("status partial rendered the full page layout, want fragment only")
	}
}

func TestHealth(t *testing.T) {
	rec := get(t, newTestHandler(t), "/health")

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /health status = %d, want %d", rec.Code, http.StatusOK)
	}
	if body := rec.Body.String(); !strings.Contains(body, `"status":"ok"`) {
		t.Errorf("body = %q, want status ok", body)
	}
}

func TestStaticAssetIsServed(t *testing.T) {
	rec := get(t, newTestHandler(t), "/static/htmx.min.js")

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /static/htmx.min.js status = %d, want %d", rec.Code, http.StatusOK)
	}
	if rec.Body.Len() < 1000 {
		t.Errorf("htmx asset length = %d, want a vendored library", rec.Body.Len())
	}
}

func TestUnknownRouteReturns404(t *testing.T) {
	rec := get(t, newTestHandler(t), "/does-not-exist")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("GET /does-not-exist status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestNewRejectsMissingTemplates(t *testing.T) {
	_, err := New(config.Config{}, fstest.MapFS{})
	if err == nil {
		t.Fatal("New() with no templates succeeded, want error")
	}
}
