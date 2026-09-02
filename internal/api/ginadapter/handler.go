package ginadapter

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/guilhermelinosp/golang-api-template/internal/api"
)

// defaultBodyLimit caps request bodies when not configured (1 MiB).
const defaultBodyLimit = 1 << 20

// ginRequest adapts *gin.Context to the api.Request port. It exists only
// inside this package — business code sees just the interface.
type ginRequest struct {
	ctx *gin.Context
}

func (r *ginRequest) Param(name string) string  { return r.ctx.Param(name) }
func (r *ginRequest) Query(name string) string  { return r.ctx.Query(name) }
func (r *ginRequest) Header(name string) string { return r.ctx.GetHeader(name) }
func (r *ginRequest) Raw() *http.Request        { return r.ctx.Request }
func (r *ginRequest) Bind(v any) error {
	err := api.BindInto(r.ctx.Request.Body, v)
	if isTooLarge(err) {
		return api.New(http.StatusRequestEntityTooLarge, "REQUEST_TOO_LARGE",
			"request body exceeds allowed size")
	}
	return err
}

// writeResponse serializes an abstraction Response onto the wire.
func writeResponse(c *gin.Context, resp api.Response) {
	status := resp.Status
	if status == 0 {
		status = http.StatusOK
	}
	for key, values := range resp.Header {
		for _, v := range values {
			c.Writer.Header().Add(key, v)
		}
	}
	if resp.Body == nil {
		c.Status(status)
		return
	}

	payload, err := json.Marshal(resp.Body)
	if err != nil {
		// Serialization failure must never produce a half-written 200.
		writeError(c, slog.Default(), api.Internal(err))
		return
	}
	c.Data(status, "application/json; charset=utf-8", payload)
}

// errorEnvelope is the single wire format for every failure mode.
type errorEnvelope struct {
	Error     errorDetail `json:"error"`
	RequestID string      `json:"requestId,omitempty"`
}

type errorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// writeError is THE funnel for failures: panics, framework 404/405 and
// handler errors all land here, guaranteeing uniform JSON responses while
// internal causes stay log-only.
func writeError(c *gin.Context, logger *slog.Logger, err error) {
	mapped := api.MapError(err)

	if cause := api.Cause(mapped); cause != nil && !errors.Is(cause, context.Canceled) {
		logger.ErrorContext(c.Request.Context(), "request failed",
			slog.String("method", c.Request.Method),
			slog.String("path", c.Request.URL.Path),
			slog.Int("status", mapped.Status),
			slog.String("code", mapped.Code),
			slog.Any("error", cause),
		)
	}

	body := errorEnvelope{
		Error:     errorDetail{Code: mapped.Code, Message: mapped.Message},
		RequestID: requestIDFrom(c),
	}

	payload, marshalErr := json.Marshal(body)
	if marshalErr != nil { // cannot practically happen with plain strings
		c.Writer.Header().Set("Content-Type", "application/json; charset=utf-8")
		c.Data(http.StatusInternalServerError, "application/json; charset=utf-8",
			[]byte(`{"error":{"code":"INTERNAL_ERROR","message":"internal server error"}}`))
		return
	}

	c.Header("Content-Type", "application/json; charset=utf-8")
	c.Data(mapped.Status, "application/json; charset=utf-8", payload)
}

// maxBytesError detects bodies exceeding the configured limit so oversized
// payloads surface as 413 instead of opaque decode errors.
func isTooLarge(err error) bool {
	var mbe *http.MaxBytesError
	return errors.As(err, &mbe)
}
