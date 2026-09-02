// Package config centralizes application-owned configuration.
//
// It intentionally knows nothing about telemetry: OpenTelemetry/OTLP settings
// belong exclusively to hellnet-lib-telemetry and are read from the
// HELLNET_TELEMETRY_* environment variables by the library itself
// (see internal/observability for the single integration point).
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Default values applied when the corresponding environment variable is not set.
const (
	defaultName           = "golang-api-template"
	defaultEnv            = "development"
	defaultPort           = "8080"
	defaultShutdown       = 10 * time.Second
	defaultReadHeader     = 10 * time.Second
	defaultReadTimeout    = 15 * time.Second
	defaultWriteTimeout   = 30 * time.Second
	defaultIdleTimeout    = 120 * time.Second
	envProduction         = "production"
	unknownBuild          = "unknown"
	maxUntrustedBodyBytes = 1 << 20 // 1 MiB request body limit (security default)
)

// Build carries build metadata injected via -ldflags at compile time.
// A single source of truth: never duplicated inside other layers.
type Build struct {
	Version string
	Commit  string
	Date    string
}

// Config holds every application-specific setting used by the template.
type Config struct {
	Name              string        // APP_NAME — also used as fallback telemetry service name
	Env               string        // APP_ENV — controls runtime tuning (e.g. Gin release mode)
	Port              string        // APP_PORT
	ShutdownTimeout   time.Duration // APP_SHUTDOWN_TIMEOUT
	ReadTimeout       time.Duration // APP_READ_TIMEOUT
	WriteTimeout      time.Duration // APP_WRITE_TIMEOUT
	IdleTimeout       time.Duration // APP_IDLE_TIMEOUT
	ReadHeaderTimeout time.Duration // APP_READ_HEADER_TIMEOUT

	CORSAllowedOrigins []string // APP_CORS_ALLOWED_ORIGINS (comma-separated; empty disables CORS)

	BodyLimit int64 // request body cap in bytes (derived; not env-tunable on purpose)

	Build Build // ldflags-injected metadata (version/commit/date)
}

// Load reads configuration from the process environment applying secure defaults.
func Load(build Build) (*Config, error) {
	c := &Config{
		Name:              str("APP_NAME", defaultName),
		Env:               str("APP_ENV", defaultEnv),
		Port:              str("APP_PORT", defaultPort),
		ShutdownTimeout:   defaultShutdown,
		ReadTimeout:       defaultReadTimeout,
		WriteTimeout:      defaultWriteTimeout,
		IdleTimeout:       defaultIdleTimeout,
		ReadHeaderTimeout: defaultReadHeader,
		BodyLimit:         maxUntrustedBodyBytes,
		Build:             build,
	}

	var err error
	if c.ShutdownTimeout, err = duration("APP_SHUTDOWN_TIMEOUT", c.ShutdownTimeout); err != nil {
		return nil, err
	}
	if c.ReadTimeout, err = duration("APP_READ_TIMEOUT", c.ReadTimeout); err != nil {
		return nil, err
	}
	if c.WriteTimeout, err = duration("APP_WRITE_TIMEOUT", c.WriteTimeout); err != nil {
		return nil, err
	}
	if c.IdleTimeout, err = duration("APP_IDLE_TIMEOUT", c.IdleTimeout); err != nil {
		return nil, err
	}
	if c.ReadHeaderTimeout, err = duration("APP_READ_HEADER_TIMEOUT", c.ReadHeaderTimeout); err != nil {
		return nil, err
	}
	c.CORSAllowedOrigins = parseList(str("APP_CORS_ALLOWED_ORIGINS", ""))

	if err := c.Validate(); err != nil {
		return nil, err
	}
	return c, nil
}

// Validate enforces invariants that must hold before bootstrapping anything else.
func (c *Config) Validate() error {
	port, err := strconv.Atoi(c.Port)
	switch {
	case err != nil:
		return fmt.Errorf("config: APP_PORT %q is not numeric", c.Port)
	case port < 1 || port > 65535:
		return fmt.Errorf("config: APP_PORT %d outside valid range [1,65535]", port)
	case c.Name == "":
		return fmt.Errorf("config: APP_NAME cannot be empty")
	case c.ShutdownTimeout <= 0 || c.ReadTimeout <= 0 ||
		c.WriteTimeout <= 0 || c.IdleTimeout <= 0 || c.ReadHeaderTimeout <= 0:
		return fmt.Errorf("config: all timeouts must be positive")
	}
	return nil
}

// IsProduction reports whether the service runs with production semantics.
func (c *Config) IsProduction() bool { return c.Env == envProduction }

func str(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func duration(key string, def time.Duration) (time.Duration, error) {
	raw := os.Getenv(key)
	if raw == "" {
		return def, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("config: %s=%q is not a valid duration: %w", key, raw, err)
	}
	return d, nil
}

// parseList parses a comma-separated environment value into a trimmed list.
func parseList(raw string) []string {
	if raw == "" {
		return nil
	}
	out := make([]string, 0, 4)
	for _, part := range strings.Split(raw, ",") {
		if piece := strings.TrimSpace(part); piece != "" {
			out = append(out, piece)
		}
	}
	return out
}
