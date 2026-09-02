package ginadapter

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/guilhermelinosp/golang-api-template/internal/api"
)

// requestIDHeader is echoed by the platform on every response.
const requestIDHeader = "X-Request-ID"

const requestIDContextKey = "template.requestID"

// requestID ensures every request/response pair carries a correlation id.
// A sanitized incoming header is honored (distributed tracing across hops);
// otherwise a random 128-bit id is generated. Values never reach logs here —
// hellnet-lib-telemetry already correlates requests via trace_id.
func requestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := sanitizeRequestID(c.GetHeader(requestIDHeader))
		if id == "" {
			id = generateRequestID()
		}
		c.Set(requestIDContextKey, id)
		c.Writer.Header().Set(requestIDHeader, id)
		c.Next()
	}
}

// RequestIDFrom returns the correlation id computed for this request.
func RequestIDFrom(c *gin.Context) string {
	return requestIDFrom(c)
}

func requestIDFrom(c *gin.Context) string {
	if v, ok := c.Get(requestIDContextKey); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func sanitizeRequestID(raw string) string {
	raw = strings.TrimSpace(raw)
	if len(raw) == 0 || len(raw) > 64 {
		return ""
	}
	for _, r := range raw {
		switch {
		case r >= '0' && r <= '9', r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z',
			r == '-', r == '_', r == '.':
		default:
			return ""
		}
	}
	return raw
}

func generateRequestID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "unavailable" // crypto/rand failure is catastrophic but non-fatal for ids
	}
	return hex.EncodeToString(buf)
}

// securityHeaders applies conservative defaults to EVERY response, including
// telemetry endpoints. HSTS is intentionally omitted: TLS termination
// normally happens at the ingress/LB layer in Kubernetes deployments.
func securityHeaders() gin.HandlerFunc {
	headers := map[string]string{
		"X-Content-Type-Options":       "nosniff",
		"X-Frame-Options":              "DENY",
		"Referrer-Policy":              "strict-origin-when-cross-origin",
		"Content-Security-Policy":      "default-src 'none'; frame-ancestors 'none'",
		"Cross-Origin-Resource-Policy": "same-origin",
	}
	return func(c *gin.Context) {
		for k, v := range headers {
			c.Writer.Header().Set(k, v)
		}
		c.Next()
	}
}

// cors implements the minimum viable CORS policy without an extra dependency:
// exact-origin allowlist (or "*" wildcard), standard preflight handling.
// When allowed origins is empty the middleware is not installed at all —
// same-origin deployments pay zero cost.
func cors(allowedOrigins []string) gin.HandlerFunc {
	wildcard := false
	allowed := make(map[string]struct{}, len(allowedOrigins))
	for _, o := range allowedOrigins {
		if o == "*" {
			wildcard = true
		}
		allowed[o] = struct{}{}
	}

	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin != "" && originAllowed(origin, wildcard, allowed) {
			h := c.Writer.Header()
			h.Set("Access-Control-Allow-Origin", origin)
			h.Set("Vary", "Origin")
			h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type, "+requestIDHeader)
			h.Set("Access-Control-Max-Age", "600")
		}
		if c.Request.Method == http.MethodOptions && c.GetHeader("Access-Control-Request-Method") != "" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

func originAllowed(origin string, wildcard bool, allowed map[string]struct{}) bool {
	if wildcard {
		return true
	}
	_, ok := allowed[origin]
	return ok
}

// recovery converts panics into the platform error envelope instead of gin's
// text/stacktrace default; stack details go to logs only (secure responses).
func recovery(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if rec := recover(); rec != nil {
				logger.ErrorContext(c.Request.Context(), "panic recovered",
					slog.String("method", c.Request.Method),
					slog.String("path", c.Request.URL.Path),
					slog.Any("panic", rec),
				)
				writeError(c, logger, api.Internal(nil))
			}
		}()
		c.Next()
	}
}
