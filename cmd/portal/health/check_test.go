package health

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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

func TestCheckHealthy(t *testing.T) {
	srv := newUpstream(t, http.StatusOK, http.StatusOK)

	report := New(srv.URL).Check(context.Background())

	if report.Status != StatusOK {
		t.Errorf("Status = %q, want %q", report.Status, StatusOK)
	}
	if report.Portal.Status != StatusOK {
		t.Errorf("Portal.Status = %q, want %q", report.Portal.Status, StatusOK)
	}
	if report.Application.Status != StatusOK {
		t.Errorf("Application.Status = %q, want %q", report.Application.Status, StatusOK)
	}
	if report.Database.Status != StatusReady {
		t.Errorf("Database.Status = %q, want %q", report.Database.Status, StatusReady)
	}
	if report.Application.HTTPStatus != http.StatusOK || report.Database.HTTPStatus != http.StatusOK {
		t.Errorf("HTTP statuses = %d/%d, want 200/200",
			report.Application.HTTPStatus, report.Database.HTTPStatus)
	}
	if report.TaskManagerURL != srv.URL {
		t.Errorf("TaskManagerURL = %q, want %q", report.TaskManagerURL, srv.URL)
	}
	if report.CheckedAt.IsZero() {
		t.Error("CheckedAt is zero")
	}
}

func TestCheckDatabaseDown(t *testing.T) {
	srv := newUpstream(t, http.StatusOK, http.StatusServiceUnavailable)

	report := New(srv.URL).Check(context.Background())

	if report.Status != StatusDegraded {
		t.Errorf("Status = %q, want %q", report.Status, StatusDegraded)
	}
	if report.Application.Status != StatusOK {
		t.Errorf("Application.Status = %q, want %q", report.Application.Status, StatusOK)
	}
	if report.Database.Status != StatusUnavailable {
		t.Errorf("Database.Status = %q, want %q", report.Database.Status, StatusUnavailable)
	}
	if report.Database.HTTPStatus != http.StatusServiceUnavailable {
		t.Errorf("Database.HTTPStatus = %d, want %d", report.Database.HTTPStatus, http.StatusServiceUnavailable)
	}
}

func TestCheckApplicationDown(t *testing.T) {
	srv := newUpstream(t, http.StatusInternalServerError, http.StatusOK)

	report := New(srv.URL).Check(context.Background())

	if report.Status != StatusDegraded {
		t.Errorf("Status = %q, want %q", report.Status, StatusDegraded)
	}
	if report.Application.Status != StatusUnavailable {
		t.Errorf("Application.Status = %q, want %q", report.Application.Status, StatusUnavailable)
	}
	if !strings.Contains(report.Application.Error, "unexpected status 500") {
		t.Errorf("Application.Error = %q, want unexpected status detail", report.Application.Error)
	}
}

func TestCheckUnreachableTaskManager(t *testing.T) {
	srv := newUpstream(t, http.StatusOK, http.StatusOK)
	url := srv.URL
	srv.Close()

	report := New(url).Check(context.Background())

	if report.Status != StatusDegraded {
		t.Errorf("Status = %q, want %q", report.Status, StatusDegraded)
	}
	if report.Application.Status != StatusUnavailable {
		t.Errorf("Application.Status = %q, want %q", report.Application.Status, StatusUnavailable)
	}
	if report.Database.Status != StatusUnavailable {
		t.Errorf("Database.Status = %q, want %q", report.Database.Status, StatusUnavailable)
	}
}

func TestNewTrimsTrailingSlash(t *testing.T) {
	checker := New("http://taskmanager:80///")

	if checker.taskManagerURL != "http://taskmanager:80" {
		t.Errorf("taskManagerURL = %q, want http://taskmanager:80", checker.taskManagerURL)
	}
}
