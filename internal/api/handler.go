// Package api defines the transport-agnostic HTTP contract used by all
// application layers.
//
// Application code (handlers, services, repositories) depends ONLY on this
// package. Gin is an implementation detail hidden behind internal/api/ginadapter:
//
//	Application ──► internal/api (this package) ──► ginadapter ──► gin
//
// The abstraction intentionally stays small: routing, handler signature,
// request reading, response writing and error semantics. Anything beyond that
// would recreate the framework it is supposed to abstract away.
package api

import (
	"context"
	"net/http"
)

// Handler is the transport-agnostic request processor implemented by every
// endpoint. Gin never appears in signatures — the adapter converts
// *gin.Context into the Request port and serializes the returned Response.
type Handler interface {
	Handle(ctx context.Context, req Request) (Response, error)
}

// HandlerFunc adapts plain functions to the Handler interface.
type HandlerFunc func(ctx context.Context, req Request) (Response, error)

// Handle implements Handler.
func (f HandlerFunc) Handle(ctx context.Context, req Request) (Response, error) {
	return f(ctx, req)
}

// Route declares one endpoint. Exactly one of Handler/Raw must be set:
//
//   - Handler  → business endpoint written against this abstraction;
//   - Raw      → instrumentation-grade net/http.Handler mounted verbatim
//     (telemetry health/metrics handlers come in through here so the
//     hellnet-lib-telemetry endpoints are exposed without re-implementations).
type Route struct {
	Method  string       // HTTP verb; use the constants below
	Path    string       // web-style wildcards: "/hello/{name}"
	Handler Handler      // abstraction handler
	Raw     http.Handler // mounted as-is (escape hatch for infra endpoints)
}

// MethodGet/… exist for readability at call sites; values mirror net/http.
const (
	MethodGet     = http.MethodGet
	MethodPost    = http.MethodPost
	MethodPut     = http.MethodPut
	MethodPatch   = http.MethodPatch
	MethodDelete  = http.MethodDelete
	MethodHead    = http.MethodHead
	MethodOptions = http.MethodOptions
)
