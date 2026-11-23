package config

import (
	"strings"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	t.Setenv("APP_ENV", "")
	t.Setenv("PORT", "")
	t.Setenv("TASK_MANAGER_URL", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.AppEnv != "development" {
		t.Errorf("AppEnv = %q, want development", cfg.AppEnv)
	}
	if cfg.Port != "8080" {
		t.Errorf("Port = %q, want 8080", cfg.Port)
	}
	if cfg.TaskManagerURL != "http://localhost:8080" {
		t.Errorf("TaskManagerURL = %q, want http://localhost:8080", cfg.TaskManagerURL)
	}
	if cfg.IsProduction() {
		t.Error("IsProduction() = true, want false")
	}
}

func TestLoadCustomValues(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("PORT", "8082")
	t.Setenv("TASK_MANAGER_URL", "https://taskmanager.internal")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Port != "8082" {
		t.Errorf("Port = %q, want 8082", cfg.Port)
	}
	if cfg.TaskManagerURL != "https://taskmanager.internal" {
		t.Errorf("TaskManagerURL = %q, want https://taskmanager.internal", cfg.TaskManagerURL)
	}
	if !cfg.IsProduction() {
		t.Error("IsProduction() = false, want true")
	}
}

func TestLoadRejectsInvalidPort(t *testing.T) {
	for _, port := range []string{"0", "65536", "-1", "http"} {
		t.Run(port, func(t *testing.T) {
			t.Setenv("PORT", port)

			if _, err := Load(); err == nil {
				t.Fatalf("Load() with PORT=%q succeeded, want error", port)
			}
		})
	}
}

func TestLoadRejectsInvalidTaskManagerURL(t *testing.T) {
	t.Setenv("PORT", "8080")

	tests := map[string]string{
		"missing scheme": "taskmanager:80",
		"unsupported":    "ftp://taskmanager",
		"missing host":   "http://",
	}

	for name, value := range tests {
		t.Run(name, func(t *testing.T) {
			t.Setenv("TASK_MANAGER_URL", value)

			_, err := Load()
			if err == nil {
				t.Fatalf("Load() with TASK_MANAGER_URL=%q succeeded, want error", value)
			}
			if !strings.Contains(err.Error(), "TASK_MANAGER_URL") {
				t.Errorf("error = %q, want it to mention TASK_MANAGER_URL", err)
			}
		})
	}
}
