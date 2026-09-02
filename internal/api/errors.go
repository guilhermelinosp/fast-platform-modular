package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
)

// Error is the canonical application error translated to predictable HTTP
// responses by every adapter. Only Code and Message ever reach the client —
// cause chains stay internal so stack traces, secrets and driver details can
// never leak through error responses.
type Error struct {
	Status  int    `json:"-"`
	Code    string `json:"code"`
	Message string `json:"message"`
	cause   error  // internal only; logged centrally, never serialized
}

// Error implements the error interface.
func (e *Error) Error() string { return e.Code + ": " + e.Message }

// Unwrap exposes the internal cause to errors.Is/As consumers (logging,
// tests) while keeping it out of HTTP bodies.
func (e *Error) Unwrap() error { return e.cause }

// New creates an application error bound to an HTTP status.
func New(status int, code, message string) *Error {
	return &Error{Status: status, Code: code, Message: message}
}

// Wrap attaches an internal cause to an application error.
func Wrap(err *Error, cause error) *Error {
	return &Error{Status: err.Status, Code: err.Code, Message: err.Message, cause: cause}
}

// ─────────────────────── Constructors per class ───────────────────────
//
// Error taxonomy: domain semantics below; anything unexpected degrades to
// Internal in MapError. Constructors exist for clarity at call sites and to
// keep codes consistent across the codebase.

// Validation builds a 400 for a malformed field value.
func Validation(field, reason string) *Error {
	return New(http.StatusBadRequest, "VALIDATION_ERROR", fmt.Sprintf("field %q %s", field, reason))
}

// BadRequest builds a 400 with a client-supplied explanation.
func BadRequest(reason string) *Error {
	return New(http.StatusBadRequest, "BAD_REQUEST", reason)
}

// NotFound builds a 404 naming the missing resource.
func NotFound(resource string) *Error {
	return New(http.StatusNotFound, "NOT_FOUND", fmt.Sprintf("%s not found", resource))
}

// Unauthorized builds a 401 for missing or invalid credentials.
func Unauthorized() *Error {
	return New(http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
}

// Forbidden builds a 403 for insufficient permissions.
func Forbidden() *Error {
	return New(http.StatusForbidden, "FORBIDDEN", "access denied")
}

// Conflict builds a 409 describing the conflicting state.
func Conflict(message string) *Error {
	return New(http.StatusConflict, "CONFLICT", message)
}

// TooManyRequests builds a 429 rate-limit rejection.
func TooManyRequests() *Error {
	return New(http.StatusTooManyRequests, "RATE_LIMITED", "too many requests")
}

// ServiceUnavailable builds a 503 for temporary upstream unavailability.
func ServiceUnavailable() *Error {
	return New(http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "service temporarily unavailable")
}

// Internal builds a 500 whose client-facing message is fixed; supply the real
// cause via Wrap so operators see it in logs without exposing internals.
func Internal(cause error) *Error {
	return &Error{
		Status:  http.StatusInternalServerError,
		Code:    "INTERNAL_ERROR",
		Message: "internal server error",
		cause:   cause,
	}
}

// MapError converts any error into its wire representation. Application
// errors pass through untouched; context cancellation maps to the client's
// own disconnect semantics; everything else collapses into a sanitized 500.
func MapError(err error) *Error {
	if err == nil {
		return nil
	}
	var appErr *Error
	if errors.As(err, &appErr) {
		return appErr // already a wire error (with or without wrapped cause)
	}
	switch {
	case errors.Is(err, context.Canceled):
		// 499 has no net/http constant by design (nginx convention for
		// client-side disconnects); keep the literal named and documented.
		return New(statusClientClosedRequest, "CLIENT_CLOSED_REQUEST", "request canceled by client")
	case errors.Is(err, context.DeadlineExceeded):
		return New(http.StatusGatewayTimeout, "TIMEOUT", "upstream deadline exceeded")
	default:
		return Internal(err)
	}
}

// statusClientClosedRequest documents client-gone-away semantics.
const statusClientClosedRequest = 499

// Cause returns the wrapped internal cause (for centralized logging).
func Cause(e *Error) error { return e.cause }
