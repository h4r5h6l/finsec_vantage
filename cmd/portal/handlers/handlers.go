// Package handlers serves the Portal HTML dashboard and health endpoint.
package handlers

import (
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/h4r5h6l/kubernetes_demo/cmd/portal/config"
)

// Handler serves the server-rendered Portal interface.
type Handler struct {
	cfg    config.Config
	pages  *template.Template
	static fs.FS
}

// snapshotData is the view model for the dashboard and its refreshable status
// panel.
type snapshotData struct {
	Title          string
	AppEnv         string
	TaskManagerURL string
	Now            time.Time
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
	}

	pages, err := template.New("portal").Funcs(funcs).ParseFS(assets, "templates/*.html")
	if err != nil {
		return nil, fmt.Errorf("parse templates: %w", err)
	}

	staticFS, err := fs.Sub(assets, "static")
	if err != nil {
		return nil, fmt.Errorf("prepare static assets: %w", err)
	}

	return &Handler{cfg: cfg, pages: pages, static: staticFS}, nil
}

// RegisterRoutes mounts the dashboard, status partial, health probe, and static
// assets.
func (h *Handler) RegisterRoutes(r gin.IRouter) {
	r.GET("/", h.Dashboard)
	r.GET("/partials/status", h.Status)
	r.GET("/health", h.Health)
	r.StaticFS("/static", http.FS(h.static))
}

// Health is a liveness probe for the Portal process. It deliberately does not
// call the task manager: upstream health belongs in the readiness probe added
// in M3.
func (h *Handler) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "ok",
		"service": "portal",
	})
}

// Dashboard renders the full HTML dashboard.
func (h *Handler) Dashboard(c *gin.Context) {
	h.render(c, http.StatusOK, "layout", h.snapshot("Dashboard"))
}

// Status renders the refreshable status panel used by htmx.
func (h *Handler) Status(c *gin.Context) {
	h.render(c, http.StatusOK, "status", h.snapshot(""))
}

func (h *Handler) snapshot(title string) snapshotData {
	return snapshotData{
		Title:          title,
		AppEnv:         h.cfg.AppEnv,
		TaskManagerURL: h.cfg.TaskManagerURL,
		Now:            time.Now(),
	}
}

func (h *Handler) render(c *gin.Context, status int, name string, data any) {
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.Status(status)
	if err := h.pages.ExecuteTemplate(c.Writer, name, data); err != nil {
		log.Printf("portal: render %s: %v", name, err)
	}
}
