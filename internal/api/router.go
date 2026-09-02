package api

import "net/http"

// Router is the abstraction the application registers routes against.
// Adapters (currently internal/api/ginadapter) translate it to concrete
// framework calls; business code never touches gin.Engine/RouterGroup.
type Router interface {
	// Handle registers a business route. Path uses web-style wildcards:
	// "/hello/{name}". Extra middleware wraps this handler only.
	Handle(method string, path string, handler Handler, middlewares ...Middleware)

	// Mount attaches a raw net/http.Handler (telemetry health/metrics
	// endpoints arrive here). It bypasses abstraction serialization by design.
	Mount(method string, path string, rawHandler HTTPHandler)

	// Group returns a sub-router sharing a path prefix and default middleware.
	Group(prefix string, middlewares ...Middleware) Router

	// ServeHTTP exposes the fully assembled router as a standard handler so
	// http.Server — not Gin — owns the network lifecycle.
	ServeHTTP(w http.ResponseWriter, r *http.Request)
}

// HTTPHandler aliases net/http.Handler purely for documentation parity with
// Router.Mount; adapters wire it verbatim into the engine.
type HTTPHandler = http.Handler

// Middleware decorates handlers at the abstraction level: portable,
// testable without any HTTP framework (auth, tenancy, feature flags...).
type Middleware func(Handler) Handler
