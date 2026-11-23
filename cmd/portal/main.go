package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/h4r5h6l/kubernetes_demo/cmd/portal/config"
	"github.com/h4r5h6l/kubernetes_demo/cmd/portal/handlers"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("portal: %v", err)
	}
}

func run() error {
	healthcheckOnly := flag.Bool("healthcheck", false, "request the probe endpoint on localhost:$PORT and exit")
	healthcheckPath := flag.String("healthcheck-path", "/health", "probe path used with -healthcheck")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	if *healthcheckOnly {
		return checkHealth(cfg.Port, *healthcheckPath)
	}

	if cfg.IsProduction() {
		gin.SetMode(gin.ReleaseMode)
	}

	portalHandler, err := handlers.New(cfg, assets)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery(), securityHeaders)
	portalHandler.RegisterRoutes(router)

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		log.Printf("portal listening on %s (env=%s, task_manager=%s)", srv.Addr, cfg.AppEnv, cfg.TaskManagerURL)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	select {
	case err := <-serverErr:
		return fmt.Errorf("http server: %w", err)
	case <-ctx.Done():
		log.Println("shutdown signal received")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown: %w", err)
	}

	log.Println("portal stopped")
	return nil
}

// securityHeaders applies conservative defaults to every response. The Portal
// has no inline scripts and no framing requirements.
func securityHeaders(c *gin.Context) {
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("X-Frame-Options", "DENY")
	c.Header("Referrer-Policy", "no-referrer")
	c.Next()
}

func checkHealth(port, path string) error {
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://127.0.0.1:" + port + path)
	if err != nil {
		return fmt.Errorf("healthcheck: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("healthcheck: unexpected status %d", resp.StatusCode)
	}
	return nil
}
