// Package config loads the Portal runtime configuration from environment
// variables.
package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
)

// Config holds the runtime configuration for the Portal.
type Config struct {
	AppEnv         string
	Port           string
	TaskManagerURL string
}

// Load reads configuration from the environment and validates it.
//
// TASK_MANAGER_URL defaults to the local task manager so `make portal-run`
// works without extra setup. The Kubernetes ConfigMap overrides it with the
// in-cluster Service URL.
func Load() (Config, error) {
	cfg := Config{
		AppEnv:         getenv("APP_ENV", "development"),
		Port:           getenv("PORT", "8080"),
		TaskManagerURL: getenv("TASK_MANAGER_URL", "http://localhost:8080"),
	}

	if err := cfg.validate(); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func (c Config) validate() error {
	port, err := strconv.Atoi(c.Port)
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("invalid PORT %q: must be a number between 1 and 65535", c.Port)
	}

	parsed, err := url.Parse(c.TaskManagerURL)
	if err != nil {
		return fmt.Errorf("invalid TASK_MANAGER_URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("invalid TASK_MANAGER_URL scheme %q: expected http or https", parsed.Scheme)
	}
	if parsed.Host == "" {
		return fmt.Errorf("invalid TASK_MANAGER_URL %q: host is required", c.TaskManagerURL)
	}

	return nil
}

// IsProduction reports whether the Portal is running with APP_ENV=production.
func (c Config) IsProduction() bool {
	return c.AppEnv == "production"
}

func getenv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok && value != "" {
		return value
	}
	return fallback
}
