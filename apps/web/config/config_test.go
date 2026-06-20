package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigLoadsDevState(t *testing.T) {
	clearConfigEnv(t)

	dir := writeConfigFiles(t, map[string]string{
		"state.env":  "STATE=dev\n",
		"shared.env": "PORT=8080\nBASE_URL=http://localhost:8080\n",
		"dev.env":    "DATABASE_URL=postgres://dev-user:dev-pass@localhost:5432/trigger_dev\nWORKOS_API_KEY=dev-workos-key\n",
		"prod.env":   "DATABASE_URL=postgres://prod-user:prod-pass@localhost:5432/trigger_prod\nWORKOS_API_KEY=prod-workos-key\n",
	})

	cfg, err := LoadConfig(dir)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	if cfg.State != "dev" {
		t.Fatalf("State = %q, want dev", cfg.State)
	}
	if cfg.Port != "8080" {
		t.Fatalf("Port = %q, want 8080", cfg.Port)
	}
	if cfg.BaseURL != "http://localhost:8080" {
		t.Fatalf("BaseURL = %q", cfg.BaseURL)
	}
	if cfg.DatabaseURL != "postgres://dev-user:dev-pass@localhost:5432/trigger_dev" {
		t.Fatalf("DatabaseURL = %q", cfg.DatabaseURL)
	}
	if cfg.WorkOSAPIKey != "dev-workos-key" {
		t.Fatalf("WorkOSAPIKey = %q", cfg.WorkOSAPIKey)
	}
}

func TestLoadConfigLoadsProdState(t *testing.T) {
	clearConfigEnv(t)

	dir := writeConfigFiles(t, map[string]string{
		"state.env":  "STATE=prod\n",
		"shared.env": "PORT=8080\nBASE_URL=https://trigger.example.com\n",
		"dev.env":    "DATABASE_URL=postgres://dev-user:dev-pass@localhost:5432/trigger_dev\nWORKOS_API_KEY=dev-workos-key\n",
		"prod.env":   "DATABASE_URL=postgres://prod-user:prod-pass@db:5432/trigger_prod\nWORKOS_API_KEY=prod-workos-key\n",
	})

	cfg, err := LoadConfig(dir)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	if cfg.State != "prod" {
		t.Fatalf("State = %q, want prod", cfg.State)
	}
	if cfg.DatabaseURL != "postgres://prod-user:prod-pass@db:5432/trigger_prod" {
		t.Fatalf("DatabaseURL = %q", cfg.DatabaseURL)
	}
	if cfg.WorkOSAPIKey != "prod-workos-key" {
		t.Fatalf("WorkOSAPIKey = %q", cfg.WorkOSAPIKey)
	}
}

func TestLoadConfigStateSpecificOverridesShared(t *testing.T) {
	clearConfigEnv(t)

	dir := writeConfigFiles(t, map[string]string{
		"state.env":  "STATE=dev\n",
		"shared.env": "PORT=8080\nBASE_URL=http://localhost:8080\nDATABASE_URL=postgres://shared\n",
		"dev.env":    "DATABASE_URL=postgres://dev\nWORKOS_API_KEY=dev-workos-key\n",
	})

	cfg, err := LoadConfig(dir)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	if cfg.DatabaseURL != "postgres://dev" {
		t.Fatalf("DatabaseURL = %q, want state override", cfg.DatabaseURL)
	}
}

func TestLoadConfigMissingStateFileFails(t *testing.T) {
	clearConfigEnv(t)

	dir := writeConfigFiles(t, map[string]string{
		"shared.env": "PORT=8080\nBASE_URL=http://localhost:8080\n",
		"dev.env":    "DATABASE_URL=postgres://dev\nWORKOS_API_KEY=dev-workos-key\n",
	})

	if _, err := LoadConfig(dir); err == nil {
		t.Fatal("LoadConfig() error = nil, want error")
	}
}

func TestLoadConfigMissingSelectedStateFileFails(t *testing.T) {
	clearConfigEnv(t)

	dir := writeConfigFiles(t, map[string]string{
		"state.env":  "STATE=prod\n",
		"shared.env": "PORT=8080\nBASE_URL=https://trigger.example.com\n",
		"dev.env":    "DATABASE_URL=postgres://dev\nWORKOS_API_KEY=dev-workos-key\n",
	})

	if _, err := LoadConfig(dir); err == nil {
		t.Fatal("LoadConfig() error = nil, want error")
	}
}

func writeConfigFiles(t *testing.T, files map[string]string) string {
	t.Helper()

	dir := t.TempDir()
	for name, contents := range files {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return dir
}

func clearConfigEnv(t *testing.T) {
	t.Helper()

	for _, key := range []string{"STATE", "PORT", "BASE_URL", "DATABASE_URL", "WORKOS_API_KEY", "CRM_PROVIDER", "CRM_BASE_URL", "CRM_API_KEY"} {
		t.Setenv(key, "")
	}
}
