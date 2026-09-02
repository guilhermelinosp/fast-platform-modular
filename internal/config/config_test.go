package config

import (
	"testing"
	"time"
)

func TestDefaultsWhenEnvEmpty(t *testing.T) {
	cfg, err := Load(Build{Version: "v9.9.9", Commit: "abc", Date: "today"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Name != defaultName || cfg.Env != defaultEnv || cfg.Port != defaultPort {
		t.Fatalf("defaults wrong: %+v", cfg)
	}
	if cfg.ShutdownTimeout != 10*time.Second ||
		cfg.ReadHeaderTimeout != 10*time.Second ||
		cfg.ReadTimeout != 15*time.Second ||
		cfg.WriteTimeout != 30*time.Second ||
		cfg.IdleTimeout != 120*time.Second {
		t.Fatalf("timeout defaults wrong: %+v", cfg)
	}
	if cfg.IsProduction() {
		t.Fatal("development expected by default")
	}
	if len(cfg.CORSAllowedOrigins) != 0 {
		t.Fatalf("CORS must be disabled by default")
	}
}

func TestOverridesAndProduction(t *testing.T) {
	t.Setenv("APP_NAME", "orders-api")
	t.Setenv("APP_ENV", "production")
	t.Setenv("APP_PORT", "9000")
	t.Setenv("APP_SHUTDOWN_TIMEOUT", "3s")
	t.Setenv("APP_CORS_ALLOWED_ORIGINS", "https://a.com, https://b.com ,,")

	cfg, err := Load(Build{Version: "x"})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !cfg.IsProduction() {
		t.Fatal("production not detected")
	}
	if cfg.Port != "9000" || cfg.ShutdownTimeout != 3*time.Second {
		t.Fatalf("overrides lost: %+v", cfg)
	}
	if len(cfg.CORSAllowedOrigins) != 2 || cfg.CORSAllowedOrigins[1] != "https://b.com" {
		t.Fatalf("CORS parsing wrong: %q", cfg.CORSAllowedOrigins)
	}
}

func TestInvalidValuesFailFast(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
	}{
		{"bad port", map[string]string{"APP_PORT": "http"}},
		{"port range", map[string]string{"APP_PORT": "70000"}},
		{"bad duration", map[string]string{"APP_SHUTDOWN_TIMEOUT": "soon"}},
		{"negative timeout", map[string]string{"APP_READ_TIMEOUT": "-5s"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			if _, err := Load(Build{}); err == nil {
				t.Fatalf("expected failure for %+v", tc.env)
			}
		})
	}
}

func TestPortBoundariesValid(t *testing.T) {
	for _, p := range []string{"1", "65535"} {
		t.Setenv("APP_PORT", p)
		if _, err := Load(Build{}); err != nil {
			t.Errorf("port %s should be valid: %v", p, err)
		}
	}
}
