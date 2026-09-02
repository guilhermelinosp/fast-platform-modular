// Package ginadapter is the ONLY place in this repository allowed to know
// about Gin (github.com/gin-gonic/gin). It translates the transport-neutral
// contracts of internal/api into gin primitives:
//
//	internal/api (abstraction) ──► ginadapter (here) ──► gin
//
// Business packages never import this package nor gin itself.
package ginadapter

import (
	"log/slog"
	"net/http"
	"os"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/guilhermelinosp/golang-api-template/internal/api"
)

// wildcardPattern matches web-style path parameters like "{name}".
var wildcardPattern = regexp.MustCompile(`\{([a-zA-Z0-9_]+)\}`)

// Config carries everything the adapter needs from composition time.
type Config struct {
	Logger             *slog.Logger
	ReleaseMode        bool             // true → gin.SetMode(gin.ReleaseMode)
	CORSAllowedOrigins []string         // empty disables CORS processing entirely
	BodyLimit          int64            // max accepted request body bytes (0 → 1 MiB)
	GlobalMiddleware   []api.Middleware // applied to every registered business route
}

// Router implements api.Router backed by *gin.Engine.
type Router struct {
	engine          *gin.Engine
	root            *gin.RouterGroup
	logger          *slog.Logger
	bodyLimit       int64
	groupMiddleware []api.Middleware // inherited by every route of this subtree
}

// compile-time proof that the adapter satisfies the abstraction.
var _ api.Router = (*Router)(nil)

// New builds the Gin-backed router applying template-wide middleware order:
//
//	RequestID → SecurityHeaders → CORS (when enabled) → Recovery
//
// The returned value is simultaneously an api.Router and an http.Handler,
// so it plugs straight into net/http's Server lifecycle.
func New(cfg Config) *Router {
	switch {
	case cfg.ReleaseMode:
		gin.SetMode(gin.ReleaseMode)
	case os.Getenv("GIN_MODE") == "":
		gin.SetMode(gin.DebugMode)
		// else: GIN_MODE already set externally — leave it alone (tests)
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	limit := cfg.BodyLimit
	if limit <= 0 {
		limit = defaultBodyLimit
	}

	engine := gin.New() // deliberately bare: gin.Logger/Recovery replaced below
	engine.HandleMethodNotAllowed = true

	engine.Use(requestID())
	engine.Use(securityHeaders())
	if len(cfg.CORSAllowedOrigins) > 0 {
		engine.Use(cors(cfg.CORSAllowedOrigins))
	}
	engine.Use(recovery(logger))

	// Transport-agnostic middleware chain runs innermost around every
	// business handler (kept portable & unit-testable without Gin).
	cfg.GlobalMiddleware = append([]api.Middleware{}, cfg.GlobalMiddleware...)

	engine.NoRoute(func(c *gin.Context) { writeError(c, logger, api.NotFound("route")) })
	engine.NoMethod(func(c *gin.Context) {
		writeError(c, logger, api.New(http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED",
			"method not allowed for this resource"))
	})

	return &Router{
		engine:    engine,
		root:      &engine.RouterGroup,
		logger:    logger,
		bodyLimit: limit,
	}
}

// Handle implements api.Router for business endpoints.
func (r *Router) Handle(method string, path string, handler api.Handler, middlewares ...api.Middleware) {
	r.root.Handle(method, translate(path), r.wrap(handler, middlewares))
}

// Mount implements api.Router for infrastructure endpoints exposed verbatim
// (hellnet-lib-telemetry health/metrics arrive here). Raw handlers still sit
// inside the standard middleware chain (request-id, security headers...).
func (r *Router) Mount(method string, path string, rawHandler api.HTTPHandler) {
	r.root.Handle(method, translate(path), func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, r.bodyLimit)
		rawHandler.ServeHTTP(c.Writer, c.Request)
	})
}

// Group implements api.Router, delegating prefix handling to gin.RouterGroup.
func (r *Router) Group(prefix string, middlewares ...api.Middleware) api.Router {
	group := &Router{
		engine:    r.engine,
		root:      r.root.Group(translate(prefix)),
		logger:    r.logger,
		bodyLimit: r.bodyLimit,
	}
	group.groupMiddleware = append(group.groupMiddleware, middlewares...)
	return group
}

// ServeHTTP exposes the assembled engine as a plain http.Handler.
func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	req.Body = http.MaxBytesReader(w, req.Body, r.bodyLimit)
	r.engine.ServeHTTP(w, req)
}

// groupMiddleware declared on Group(...) calls is prepended to every route
// registered on that subtree (see Router.groupMiddleware).

func (r *Router) wrap(handler api.Handler, routeMiddlewares []api.Middleware) gin.HandlerFunc {
	chain := make([]api.Middleware, 0, len(r.groupMiddleware)+len(routeMiddlewares))
	chain = append(chain, r.groupMiddleware...)
	chain = append(chain, routeMiddlewares...)

	wrapped := handler
	for i := len(chain) - 1; i >= 0; i-- {
		wrapped = chain[i](wrapped)
	}

	handlerFunc := func(c *gin.Context) {
		ctx := c.Request.Context()
		resp, err := wrapped.Handle(ctx, &ginRequest{ctx: c})
		if err != nil {
			writeError(c, r.logger, err)
			return
		}
		writeResponse(c, resp)
	}
	return handlerFunc
}

// translate converts web-style wildcards "/hello/{name}" to gin syntax
// "/hello/:name" so application code stays portable across HTTP stacks.
func translate(path string) string {
	if !strings.Contains(path, "{") {
		return path
	}
	return wildcardPattern.ReplaceAllString(path, ":$1")
}
