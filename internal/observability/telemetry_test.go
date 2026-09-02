package observability

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/guilhermelinosp/golang-api-template/internal/config"
)

// TestInitWithoutEndpointRunsDisabled is the contract for tests and
// zero-config local runs: no OTLP endpoint configured → SDK disabled,
// but every helper still returns usable values.
func TestInitWithoutEndpointRunsDisabled(t *testing.T) {
	t.Setenv("HELLNET_TELEMETRY_ENDPOINT", "")
	t.Setenv("HELLNET_TELEMETRY_ENABLED", "")
	t.Setenv("HELLNET_TELEMETRY_SERVICE", "")

	cfg, err := config.Load(config.Build{Version: "test"})
	if err != nil {
		t.Fatalf("config: %v", err)
	}

	tel, err := Init(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Init must not fail without telemetry envs: %v", err)
	}
	defer func() { _ = tel.Shutdown() }() // disabled mode: no-op-ish, fast

	logger := Logger(tel)
	if logger == nil {
		t.Fatal("Logger must never return nil")
	}
	logger.Info("logging while disabled must be safe")
}

func TestExplicitEnableOverride(t *testing.T) {
	t.Setenv("HELLNET_TELEMETRY_ENDPOINT", "")
	t.Setenv("HELLNET_TELEMETRY_ENABLED", "true")

	cfg, err := config.Load(config.Build{})
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	tel, err := Init(context.Background(), cfg)
	if err != nil {
		t.Fatalf("explicit enable with empty endpoint should still construct (exports just fail): %v", err)
	}
	_ = tel.Shutdown()
}

func TestRequestTelemetryNilSafe(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(234) })
	wrapped := RequestTelemetry(nil, handler)

	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != 234 {
		t.Fatalf("passthrough broken: got %d", rec.Code)
	}
}
