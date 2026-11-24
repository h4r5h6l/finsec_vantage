// Package handlers serves the Portal HTML dashboard and health endpoints.
package handlers

import (
	"context"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/h4r5h6l/kubernetes_demo/cmd/portal/config"
	"github.com/h4r5h6l/kubernetes_demo/cmd/portal/health"
)

// Handler serves the server-rendered Portal interface.
type Handler struct {
	cfg           config.Config
	pages         *template.Template
	static        fs.FS
	healthChecker *health.Checker
}

// snapshotData is the view model for the dashboard and its refreshable status
// panel.
type snapshotData struct {
	Title          string
	AppEnv         string
	TaskManagerURL string
	Health         health.Report
}

// New parses the embedded templates and prepares the static file system.
//
// The assets are embedded by package main and injected here so they can live at
// cmd/portal/templates and cmd/portal/static, matching the layout described in
// port.md. Tests use the same on-disk files.
func New(cfg config.Config, assets fs.FS) (*Handler, error) {
	funcs := template.FuncMap{
		"formatTime": func(t time.Time) string {
			if t.IsZero() {
				return "—"
			}
			return t.UTC().Format("Jan 2, 2006 15:04 UTC")
		},
		"statusClass": func(status string) string {
			switch status {
			case health.StatusOK, health.StatusReady:
				return "badge-ok"
			case health.StatusUnavailable, health.StatusDegraded:
				return "badge-bad"
			default:
				return "badge-pending"
			}
		},
	}

	pages, err := template.New("portal").Funcs(funcs).ParseFS(assets, "templates/*.html")
	if err != nil {
		return nil, fmt.Errorf("parse templates: %w", err)
	}

	staticFS, err := fs.Sub(assets, "static")
	if err != nil {
		return nil, fmt.Errorf("prepare static assets: %w", err)
	}

	return &Handler{
		cfg:           cfg,
		pages:         pages,
		static:        staticFS,
		healthChecker: health.New(cfg.TaskManagerURL),
	}, nil
}

// RegisterRoutes mounts the dashboard, status partial, health probes, and
// static assets.
func (h *Handler) RegisterRoutes(r gin.IRouter) {
	r.GET("/", h.Dashboard)
	r.GET("/partials/status", h.Status)
	r.GET("/health", h.Health)
	r.GET("/ready", h.Ready)
	r.GET("/api/portal/health", h.APIHealth)
	r.StaticFS("/static", http.FS(h.static))
}

// Health is a liveness probe for the Portal process. It deliberately does not
// call the task manager, so an upstream outage does not restart the Portal.
func (h *Handler) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "ok",
		"service": "portal",
	})
}

// Ready is the Portal readiness probe. It reports ready only when the task
// manager is reachable and its database-backed /ready endpoint is healthy.
func (h *Handler) Ready(c *gin.Context) {
	report := h.checkHealth(c.Request.Context())

	if report.Application.Status == health.StatusOK && report.Database.Status == health.StatusReady {
		c.JSON(http.StatusOK, gin.H{
			"status":      "ready",
			"application": report.Application,
			"database":    report.Database,
		})
		return
	}

	c.JSON(http.StatusServiceUnavailable, gin.H{
		"status":      "unavailable",
		"application": report.Application,
		"database":    report.Database,
	})
}

// APIHealth returns the aggregated Portal, application, and database health.
// It always returns 200 because it is a report, not a probe.
func (h *Handler) APIHealth(c *gin.Context) {
	c.JSON(http.StatusOK, h.checkHealth(c.Request.Context()))
}

// Dashboard renders the full HTML dashboard.
func (h *Handler) Dashboard(c *gin.Context) {
	report := h.checkHealth(c.Request.Context())
	h.render(c, http.StatusOK, "layout", h.snapshot("Dashboard", report))
}

// Status renders the refreshable status panel used by htmx.
func (h *Handler) Status(c *gin.Context) {
	report := h.checkHealth(c.Request.Context())
	h.render(c, http.StatusOK, "status", h.snapshot("", report))
}

func (h *Handler) checkHealth(ctx context.Context) health.Report {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return h.healthChecker.Check(ctx)
}

func (h *Handler) snapshot(title string, report health.Report) snapshotData {
	return snapshotData{
		Title:          title,
		AppEnv:         h.cfg.AppEnv,
		TaskManagerURL: h.cfg.TaskManagerURL,
		Health:         report,
	}
}

func (h *Handler) render(c *gin.Context, status int, name string, data any) {
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.Status(status)
	if err := h.pages.ExecuteTemplate(c.Writer, name, data); err != nil {
		log.Printf("portal: render %s: %v", name, err)
	}
}
