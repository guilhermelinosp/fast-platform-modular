// Package observability is the SINGLE integration point between this
// application and github.com/guilhermelinosp/hellnet-lib-telemetry.
//
// Design rules enforced here:
//
//   - telemetry.New is called exactly once, from Init — no other file in the
//     repository imports the library's constructor surface;
//   - HELLNET_TELEMETRY_* remains the only source of telemetry configuration
//     (no parallel APP_OTEL_* variables are invented);
//   - without HELLNET_TELEMETRY_ENDPOINT the SDK runs disabled so the template
//     boots with zero configuration (health/metrics endpoints still respond);
//   - Shutdown is delegated entirely to the library (it owns provider order
//     and per-provider budgets).
package observability

import (
	"context"
	"log/slog"
	"net/http"
	"os"

	"github.com/guilhermelinosp/hellnet-lib-telemetry/telemetry"
	"go.opentelemetry.io/otel/attribute"

	"github.com/guilhermelinosp/golang-api-template/internal/config"
)

// Telemetry aliases the library type so the rest of the app can hold a
// *Telemetry without importing the library package directly. The alias keeps
// a single seam: if the library type changes shape, only this file adapts.
type Telemetry = telemetry.Telemetry

// Init builds the process-wide Telemetry instance.
//
// Behavior:
//   - Options come from HELLNET_TELEMETRY_* via the library's LoadFromEnv
//     (the .env dev file is loaded by the library itself);
//   - ServiceName falls back to cfg.Name so a missing
//     HELLNET_TELEMETRY_SERVICE is not fatal for local runs while production
//     configurations keep explicit precedence;
//   - signals start enabled iff an OTLP endpoint is configured OR
//     HELLNET_TELEMETRY_ENABLED is set ("true"/"false" as an explicit opt);
//   - when disabled, Logger/Meter fall back to safe defaults handled by the
//     library (slog.Default / noop meter) and no OTLP export happens —
//     this is also the mode used by unit tests.
func Init(ctx context.Context, cfg *config.Config) (*Telemetry, error) {
	opts := telemetry.LoadFromEnv()
	if opts.ServiceName == "" {
		opts.ServiceName = cfg.Name
	}
	// Build metadata injected at compile time travels to every backend as the
	// resource attribute service.version (overrides the lib default).
	opts.ResourceAttrs = append(opts.ResourceAttrs,
		attribute.String("service.version", cfg.Build.Version))
	opts.RedactSensitive = true

	switch os.Getenv("HELLNET_TELEMETRY_ENABLED") {
	case "true":
		opts.Enabled = true
	case "false":
		opts.Enabled = false
	default:
		opts.Enabled = opts.OTLPEndpoint != ""
	}

	return telemetry.New(ctx, opts)
}

// Logger returns the structured logger to use across the application.
// With telemetry disabled the library leaves Logger nil; slog.Default keeps
// behavior uniform (single logger interface everywhere, still leveled).
func Logger(tel *Telemetry) *slog.Logger {
	if tel == nil || tel.Logger == nil {
		return slog.Default()
	}
	return tel.Logger
}

// RequestTelemetry wraps an HTTP handler with the library's net/http
// instrumentation: request logs correlated to trace_id, HTTP metrics and
// inbound trace extraction. Applied ONCE around the whole router so every
// route (business + health + metrics) is observed without per-route code.
func RequestTelemetry(tel *Telemetry, next http.Handler) http.Handler {
	if tel == nil {
		return next
	}
	return telemetry.Middleware(tel, next)
}

// Platform exposes the library's infrastructure endpoints as ready-to-mount
// handlers. Composition layers wire these into RegisterPlatform so routing
// decisions stay template-owned while implementations stay library-owned.
func Platform(tel *Telemetry) PlatformEndpoints {
	if tel == nil {
		return PlatformEndpoints{}
	}
	return PlatformEndpoints{
		Live:    tel.Live(),
		Ready:   tel.Ready(),
		Health:  tel.Health(),
		Metrics: tel.MetricsHandler(),
	}
}

// PlatformEndpoints mirrors api.PlatformHandlers without importing it,
// keeping this file the only library-aware file in the repository.
type PlatformEndpoints struct {
	Live    http.Handler
	Ready   http.Handler
	Health  http.Handler
	Metrics http.Handler
}
