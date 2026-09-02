package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Request is the read-side port the adapter hands to handlers. It exposes
// just enough for typical JSON APIs; for everything beyond (multipart,
// websockets, server-sent events) use Raw and drop to net/http deliberately.
type Request interface {
	// Param returns a path wildcard value ("/hello/{name}" → Param("name")).
	Param(name string) string
	// Query returns the first query-string parameter with that name.
	Query(name string) string
	// Header returns the canonicalized request header.
	Header(name string) string
	// Bind decodes the JSON body into v strictly: unknown fields are rejected
	// so typos fail fast as validation errors instead of silent data loss.
	Bind(v any) error
	// Raw exposes the underlying *http.Request. Escape hatch — using it in
	// business handlers reintroduces transport coupling; prefer the ports above.
	Raw() *http.Request
}

// BindInto decodes a strictly-typed JSON body shared by adapter
// implementations: unknown fields are rejected and failures map to a
// VALIDATION_ERROR with 400 semantics.
func BindInto(body io.Reader, v any) error {
	if body == nil {
		return Validation("body", "is required")
	}
	dec := json.NewDecoder(io.LimitReader(body, maxBodyInspection))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		if IsTooLargeRequestBody(err) {
			return New(http.StatusRequestEntityTooLarge, "REQUEST_TOO_LARGE",
				"request body exceeds allowed size")
		}
		return Validation("body", fmt.Sprintf("invalid JSON: %v", err))
	}
	return nil
}

const maxBodyInspection = 1 << 20 // guard when reading bodies inside Bind

// IsTooLargeRequestBody recognizes oversized-payload signals. The
// *http.MaxBytesError check handles direct hits; the string signal covers
// encoding/json, which embeds the raw reader error without exposing its type
// (net/http has no exported sentinel for it).
func IsTooLargeRequestBody(err error) bool {
	var mbe *http.MaxBytesError
	if errors.As(err, &mbe) {
		return true
	}
	return err != nil && strings.Contains(err.Error(), "request body too large")
}
