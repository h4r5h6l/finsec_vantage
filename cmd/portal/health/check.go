// Package health aggregates the Portal's own status with the task manager's
// liveness and database readiness.
package health

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Component status values.
const (
	StatusOK          = "ok"
	StatusReady       = "ready"
	StatusUnavailable = "unavailable"
	StatusDegraded    = "degraded"
)

// Component is the health of one part of the system.
type Component struct {
	Status     string `json:"status"`
	HTTPStatus int    `json:"http_status,omitempty"`
	Error      string `json:"error,omitempty"`
}

// Report is the aggregate health payload returned by /api/portal/health and
// rendered on the dashboard.
type Report struct {
	Status         string    `json:"status"`
	Portal         Component `json:"portal"`
	Application    Component `json:"application"`
	Database       Component `json:"database"`
	TaskManagerURL string    `json:"task_manager_url"`
	CheckedAt      time.Time `json:"checked_at"`
}

// Checker probes the task manager's health endpoints.
type Checker struct {
	taskManagerURL string
	client         *http.Client
}

// New returns a Checker for the given task manager base URL. Each upstream
// request has a two-second timeout so a hung task manager cannot hang the
// Portal dashboard.
func New(taskManagerURL string) *Checker {
	return &Checker{
		taskManagerURL: strings.TrimRight(taskManagerURL, "/"),
		client: &http.Client{
			Timeout: 2 * time.Second,
		},
	}
}

// Check probes /health and /ready concurrently. The application is healthy
// when /health returns 2xx; the database is ready when /ready returns 2xx.
func (c *Checker) Check(ctx context.Context) Report {
	var application, database Component

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		application = c.probe(ctx, "/health", StatusOK)
	}()
	go func() {
		defer wg.Done()
		database = c.probe(ctx, "/ready", StatusReady)
	}()
	wg.Wait()

	status := StatusOK
	if application.Status != StatusOK || database.Status != StatusReady {
		status = StatusDegraded
	}

	return Report{
		Status:         status,
		Portal:         Component{Status: StatusOK},
		Application:    application,
		Database:       database,
		TaskManagerURL: c.taskManagerURL,
		CheckedAt:      time.Now().UTC(),
	}
}

func (c *Checker) probe(ctx context.Context, path, healthyStatus string) Component {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.taskManagerURL+path, nil)
	if err != nil {
		return Component{Status: StatusUnavailable, Error: err.Error()}
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return Component{Status: StatusUnavailable, Error: err.Error()}
	}
	defer resp.Body.Close()

	// Drain a small amount so the connection can be reused, without trusting
	// or storing remote response bodies.
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return Component{
			Status:     healthyStatus,
			HTTPStatus: resp.StatusCode,
		}
	}
	if resp.StatusCode == http.StatusServiceUnavailable {
		return Component{
			Status:     StatusUnavailable,
			HTTPStatus: resp.StatusCode,
		}
	}
	return Component{
		Status:     StatusUnavailable,
		HTTPStatus: resp.StatusCode,
		Error:      fmt.Sprintf("unexpected status %d", resp.StatusCode),
	}
}
