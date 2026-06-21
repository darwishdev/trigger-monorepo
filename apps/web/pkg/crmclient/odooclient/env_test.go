package odooclient

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// loadEnv walks up from the working directory until it finds the app's
// config/dev.env, parses KEY=VALUE lines, and sets them as env vars without
// overriding values already present in the real environment. Reusing the app's
// own config keeps a single source of truth for the Odoo connection (the
// CRM_* keys) instead of maintaining a separate test-only env file.
func loadEnv() {
	dir, err := os.Getwd()
	if err != nil {
		return
	}
	for {
		path := filepath.Join(dir, "config", "dev.env")
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			parseEnv(path)
			return
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return
		}
		dir = parent
	}
}

func parseEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		val = strings.Trim(val, `"'`)
		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, val)
		}
	}
}
