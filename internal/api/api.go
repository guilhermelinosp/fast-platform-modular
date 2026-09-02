package api

import (
	"context"
	"log/slog"
	"net/http"
	"runtime"
)

// Deps is the explicit dependency-injection surface for the HTTP boundary.
// No DI container — a plain struct constructed in cmd/api/main.go.
type Deps struct {
	Logger *slog.Logger

	// Platform carries the infrastructure handlers (liveness, readiness,
	// health and metrics). Bootstrap wires them from hellnet-lib-telemetry
	// so this package never imports the library.
	Platform PlatformHandlers

	// Routes are the business endpoints mounted under the versioned area
	// (/api/v1). Keep domain handlers here; platform endpoints are template-owned.
	Routes []Route
}

// PlatformHandlers groups the telemetry-provided net/http endpoints.
type PlatformHandlers struct {
	Live    HTTPHandler
	Ready   HTTPHandler
	Health  HTTPHandler
	Metrics HTTPHandler
}

// Valid reports whether all platform handlers were provided.
func (p PlatformHandlers) Valid() bool {
	return p.Live != nil && p.Ready != nil && p.Health != nil && p.Metrics != nil
}

// APIVersionPrefix hosts all versioned business endpoints. Versioning lives
// at the transport layer only — services and repositories stay version-free.
const APIVersionPrefix = "/api/v1"

// System endpoint paths. Their implementations come from hellnet-lib-telemetry;
// this package only decides where they are exposed.
const (
	PathRoot    = "/"
	PathLive    = "/live"
	PathReady   = "/ready"
	PathHealth  = "/health"
	PathMetrics = "/metrics"
)

// ServiceInfo carries the identity exposed by the root endpoint.
type ServiceInfo struct {
	Name    string
	Version string
	Commit  string
	BuiltAt string
}

// RegisterPlatform mounts everything every API built from this template gets
// for free onto any Router implementation:
//
//   - /live /ready /health /metrics — backed by hellnet-lib-telemetry handlers;
//   - GET / — service discovery payload;
//   - /api/v1 group receiving Deps.Routes.
//
// The caller owns router construction and middleware order (see ginadapter).
func RegisterPlatform(router Router, info ServiceInfo, deps Deps) {
	// Mount platform endpoints individually so a partial configuration still
	// exposes the handlers it was given (e.g. live/ready/health during
	// bootstrap, before metrics telemetry is wired). Each is mounted only when
	// provided, so missing handlers degrade gracefully instead of taking the
	// whole platform surface down.
	if deps.Platform.Live != nil {
		router.Mount(http.MethodGet, PathLive, deps.Platform.Live)
	}
	if deps.Platform.Ready != nil {
		router.Mount(http.MethodGet, PathReady, deps.Platform.Ready)
	}
	if deps.Platform.Health != nil {
		router.Mount(http.MethodGet, PathHealth, deps.Platform.Health)
	}
	if deps.Platform.Metrics != nil {
		router.Mount(http.MethodGet, PathMetrics, deps.Platform.Metrics)
	}

	router.Handle(http.MethodGet, PathRoot, serviceInfoHandler(info))

	v1 := router.Group(APIVersionPrefix)
	for _, r := range deps.Routes {
		switch {
		case r.Handler != nil:
			v1.Handle(r.Method, r.Path, r.Handler)
		case r.Raw != nil:
			v1.Mount(r.Method, r.Path, r.Raw)
		}
	}
}

// serviceInfoHandler answers GET / and doubles as the smallest reference
// implementation of a Handler in this codebase.
func serviceInfoHandler(info ServiceInfo) Handler {
	return HandlerFunc(func(_ context.Context, _ Request) (Response, error) {
		return JSON(http.StatusOK, map[string]string{
			"service": info.Name,
			"version": info.Version,
			"commit":  info.Commit,
			"builtAt": info.BuiltAt,
			"go":      runtime.Version(),
			"docs":    "openapi/openapi.yaml",
		}), nil
	})
}
