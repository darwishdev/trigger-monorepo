package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	State          string `mapstructure:"STATE"`
	Port           string `mapstructure:"PORT"`
	BaseURL        string `mapstructure:"BASE_URL"`
	DatabaseURL    string `mapstructure:"DATABASE_URL"`
	WorkOSAPIKey   string `mapstructure:"WORKOS_API_KEY"`
	WorkOSClientID string `mapstructure:"WORKOS_CLIENT_ID"`
	// CRM config drives the default tenant's client until the identity
	// domain lands and supplies per-tenant configs. Optional here so config
	// loading stays decoupled from CRM availability.
	CRMProvider string `mapstructure:"CRM_PROVIDER"`
	CRMBaseURL  string `mapstructure:"CRM_BASE_URL"`
	CRMAPIKey   string `mapstructure:"CRM_API_KEY"`
}

func LoadConfig(configDir string) (Config, error) {
	stateReader := viper.New()
	stateReader.SetConfigType("env")

	if err := readEnvFile(stateReader, filepath.Join(configDir, "state.env")); err != nil {
		return Config{}, fmt.Errorf("read state.env: %w", err)
	}

	state := strings.ToLower(strings.TrimSpace(stateReader.GetString("STATE")))
	if state == "" {
		return Config{}, fmt.Errorf("state.env must define STATE")
	}

	loader := viper.New()
	loader.SetConfigType("env")
	loader.SetDefault("PORT", "8080")
	loader.SetDefault("CRM_PROVIDER", "odoo")
	loader.AutomaticEnv()
	for _, key := range []string{"STATE", "PORT", "BASE_URL", "DATABASE_URL", "WORKOS_API_KEY", "CRM_PROVIDER", "CRM_BASE_URL", "CRM_API_KEY"} {
		if err := loader.BindEnv(key); err != nil {
			return Config{}, fmt.Errorf("bind env %s: %w", key, err)
		}
	}

	if err := mergeEnvFile(loader, filepath.Join(configDir, "state.env"), true); err != nil {
		return Config{}, err
	}
	if err := mergeEnvFile(loader, filepath.Join(configDir, "shared.env"), true); err != nil {
		return Config{}, err
	}

	stateFile := strings.ToLower(state) + ".env"
	if err := mergeEnvFile(loader, filepath.Join(configDir, stateFile), true); err != nil {
		return Config{}, fmt.Errorf("read %s: %w", stateFile, err)
	}

	var cfg Config
	if err := loader.Unmarshal(&cfg); err != nil {
		return Config{}, fmt.Errorf("unmarshal config: %w", err)
	}

	cfg.State = strings.ToLower(strings.TrimSpace(cfg.State))
	if cfg.State == "" {
		cfg.State = state
	}
	if cfg.Port == "" {
		return Config{}, fmt.Errorf("PORT is required")
	}
	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}
	if cfg.BaseURL == "" {
		return Config{}, fmt.Errorf("BASE_URL is required")
	}
	if cfg.WorkOSAPIKey == "" {
		return Config{}, fmt.Errorf("WORKOS_API_KEY is required")
	}

	return cfg, nil
}

func mergeEnvFile(loader *viper.Viper, path string, required bool) error {
	if err := readEnvFile(loader, path); err != nil {
		if required {
			return fmt.Errorf("read %s: %w", filepath.Base(path), err)
		}
		return nil
	}
	return nil
}

func readEnvFile(loader *viper.Viper, path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	return loader.MergeConfig(file)
}
