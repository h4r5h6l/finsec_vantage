package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/gin-gonic/gin"

	"github.com/h4r5h6l/kubernetes_demo/cmd/portal/config"
	"github.com/h4r5h6l/kubernetes_demo/cmd/portal/health"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	os.Exit(m.Run())
}

// newUpstream starts a fake task manager that answers /health and /ready with
// the provided status codes.
func newUpstream(t *testing.T, healthCode, readyCode int) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(healthCode)
	})
	mux.HandleFunc("/ready", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(readyCode)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func newTestHandlerFor(t *testing.T, taskManagerURL string) *gin.Engine {
	t.Helper()

	// The test runs from cmd/portal/handlers, so ../ is cmd/portal: the same
	// templates and static files embedded into the production binary.
	h, err := New(config.Config{
		AppEnv:         "test",
		Port:           "8080",
		TaskManagerURL: taskManagerURL,
	}, os.DirFS("../"))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	router := gin.New()
	h.RegisterRoutes(router)
	return router
}

func newTestHandler(t *testing.T) (*gin.Engine, *httptest.Server) {
	t.Helper()

	srv := newUpstream(t, http.StatusOK, http.StatusOK)
	return newTestHandlerFor(t, srv.URL), srv
}

func get(t *testing.T, router http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestDashboardRendersHTML(t *testing.T) {
	router, upstream := newTestHandler(t)

	rec := get(t, router, "/")

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
		upstream.URL,
		"/static/htmx.min.js",
		"Security scans",
		"Drift detection",
		"Cost estimate",
		`badge-ok">ok`,
		`badge-ok">ready`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("dashboard body missing %q", want)
		}
	}
}

func TestDashboardShowsDegradedWhenDatabaseIsDown(t *testing.T) {
	upstream := newUpstream(t, http.StatusOK, http.StatusServiceUnavailable)
	router := newTestHandlerFor(t, upstream.URL)

	rec := get(t, router, "/")

	if rec.Code != http.StatusOK {
		t.Fatalf("GET / status = %d, want %d", rec.Code, http.StatusOK)
	}

	body := rec.Body.String()
	for _, want := range []string{
		"degraded",
		`badge-bad">unavailable`,
		"HTTP 503",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("dashboard body missing %q", want)
		}
	}
	if strings.Contains(body, `badge-ok">ready`) {
		t.Error("dashboard reports ready while the database is down")
	}
}

func TestStatusPartialRendersHXAttributes(t *testing.T) {
	router, _ := newTestHandler(t)

	rec := get(t, router, "/partials/status")

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /partials/status status = %d, want %d", rec.Code, http.StatusOK)
	}

	body := rec.Body.String()
	for _, want := range []string{
		`id="status-panel"`,
		`hx-get="/partials/status"`,
		`hx-swap="outerHTML"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("status partial missing %q", want)
		}
	}
	if strings.Contains(body, "FinOps &amp; SecOps Portal") {
		t.Error("status partial rendered the full page layout, want fragment only")
	}
}

func TestHealth(t *testing.T) {
	router, _ := newTestHandler(t)

	rec := get(t, router, "/health")

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /health status = %d, want %d", rec.Code, http.StatusOK)
	}
	if body := rec.Body.String(); !strings.Contains(body, `"status":"ok"`) {
		t.Errorf("body = %q, want status ok", body)
	}
}

func TestHealthDoesNotCallUpstream(t *testing.T) {
	// A closed server URL makes any upstream call fail; liveness must still
	// return 200 because it has no dependencies.
	upstream := newUpstream(t, http.StatusOK, http.StatusOK)
	url := upstream.URL
	upstream.Close()

	router := newTestHandlerFor(t, url)
	rec := get(t, router, "/health")

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /health status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestReadyHealthy(t *testing.T) {
	router, _ := newTestHandler(t)

	rec := get(t, router, "/ready")

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /ready status = %d, want %d", rec.Code, http.StatusOK)
	}
	if body := rec.Body.String(); !strings.Contains(body, `"status":"ready"`) {
		t.Errorf("body = %q, want status ready", body)
	}
}

func TestReadyWhenDatabaseIsDown(t *testing.T) {
	upstream := newUpstream(t, http.StatusOK, http.StatusServiceUnavailable)
	router := newTestHandlerFor(t, upstream.URL)

	rec := get(t, router, "/ready")

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("GET /ready status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
	if body := rec.Body.String(); !strings.Contains(body, `"status":"unavailable"`) {
		t.Errorf("body = %q, want status unavailable", body)
	}
}

func TestReadyWhenTaskManagerIsUnreachable(t *testing.T) {
	upstream := newUpstream(t, http.StatusOK, http.StatusOK)
	url := upstream.URL
	upstream.Close()

	router := newTestHandlerFor(t, url)
	rec := get(t, router, "/ready")

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("GET /ready status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}

func TestAPIHealthHealthy(t *testing.T) {
	router, upstream := newTestHandler(t)

	rec := get(t, router, "/api/portal/health")

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/portal/health status = %d, want %d", rec.Code, http.StatusOK)
	}

	var report health.Report
	if err := json.Unmarshal(rec.Body.Bytes(), &report); err != nil {
		t.Fatalf("decode health report: %v", err)
	}

	if report.Status != health.StatusOK {
		t.Errorf("Status = %q, want %q", report.Status, health.StatusOK)
	}
	if report.Portal.Status != health.StatusOK {
		t.Errorf("Portal.Status = %q, want %q", report.Portal.Status, health.StatusOK)
	}
	if report.Application.Status != health.StatusOK {
		t.Errorf("Application.Status = %q, want %q", report.Application.Status, health.StatusOK)
	}
	if report.Database.Status != health.StatusReady {
		t.Errorf("Database.Status = %q, want %q", report.Database.Status, health.StatusReady)
	}
	if report.TaskManagerURL != upstream.URL {
		t.Errorf("TaskManagerURL = %q, want %q", report.TaskManagerURL, upstream.URL)
	}
}

func TestAPIHealthDegradedWhenDatabaseIsDown(t *testing.T) {
	upstream := newUpstream(t, http.StatusOK, http.StatusServiceUnavailable)
	router := newTestHandlerFor(t, upstream.URL)

	rec := get(t, router, "/api/portal/health")

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/portal/health status = %d, want %d (reports stay 200)", rec.Code, http.StatusOK)
	}

	var report health.Report
	if err := json.Unmarshal(rec.Body.Bytes(), &report); err != nil {
		t.Fatalf("decode health report: %v", err)
	}

	if report.Status != health.StatusDegraded {
		t.Errorf("Status = %q, want %q", report.Status, health.StatusDegraded)
	}
	if report.Application.Status != health.StatusOK {
		t.Errorf("Application.Status = %q, want %q", report.Application.Status, health.StatusOK)
	}
	if report.Database.Status != health.StatusUnavailable {
		t.Errorf("Database.Status = %q, want %q", report.Database.Status, health.StatusUnavailable)
	}
}

func TestStaticAssetIsServed(t *testing.T) {
	router, _ := newTestHandler(t)

	rec := get(t, router, "/static/htmx.min.js")

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /static/htmx.min.js status = %d, want %d", rec.Code, http.StatusOK)
	}
	if rec.Body.Len() < 1000 {
		t.Errorf("htmx asset length = %d, want a vendored library", rec.Body.Len())
	}
}

func TestUnknownRouteReturns404(t *testing.T) {
	router, _ := newTestHandler(t)

	rec := get(t, router, "/does-not-exist")

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
