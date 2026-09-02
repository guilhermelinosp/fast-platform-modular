package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"
)

func TestMapErrorPassthrough(t *testing.T) {
	original := Validation("name", "is required")
	got := MapError(original)

	if got != original {
		t.Fatalf("expected passthrough of application errors")
	}
	if got.Status != http.StatusBadRequest || got.Code != "VALIDATION_ERROR" {
		t.Fatalf("unexpected fields: %+v", got)
	}
}

func TestMapErrorUnknownBecomesSanitizedInternal(t *testing.T) {
	sentinel := fmt.Errorf("db: connection refused root cause secret stuff")
	mapped := MapError(sentinel)

	if mapped.Status != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", mapped.Status)
	}
	if mapped.Message != "internal server error" {
		t.Fatalf("internal details leaked: %q", mapped.Message)
	}
	if Cause(mapped) == nil {
		t.Fatal("cause must be preserved internally for logging")
	}
	if !errors.Is(mapped, sentinel) {
		t.Fatal("errors.Is chain must survive wrapping")
	}
}

func TestMapContextSemantics(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"canceled", context.Canceled, 499, "CLIENT_CLOSED_REQUEST"},
		{"deadline", context.DeadlineExceeded, http.StatusGatewayTimeout, "TIMEOUT"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mapped := MapError(tc.err)
			if mapped.Status != tc.status || mapped.Code != tc.code {
				t.Fatalf("got %d/%s, want %d/%s", mapped.Status, mapped.Code, tc.status, tc.code)
			}
		})
	}
}

func TestWrapKeepsWireFields(t *testing.T) {
	base := NotFound("user")
	cause := fmt.Errorf("sql: no rows")
	wrapped := Wrap(base, cause)

	if wrapped.Status != base.Status || wrapped.Code != base.Code || wrapped.Message != base.Message {
		t.Fatalf("wire fields changed on wrap: %+v vs %+v", wrapped, base)
	}
	if !errors.Is(wrapped.Unwrap(), cause) {
		t.Fatal("wrapped cause lost")
	}
}

func TestTaxonomyStatuses(t *testing.T) {
	cases := map[string]struct {
		err    *Error
		status int
	}{
		"badRequest":     {BadRequest("nope"), http.StatusBadRequest},
		"notFound":       {NotFound("thing"), http.StatusNotFound},
		"unauthorized":   {Unauthorized(), http.StatusUnauthorized},
		"forbidden":      {Forbidden(), http.StatusForbidden},
		"conflict":       {Conflict("dup"), http.StatusConflict},
		"tooMany":        {TooManyRequests(), http.StatusTooManyRequests},
		"serviceUnavail": {ServiceUnavailable(), http.StatusServiceUnavailable},
		"internal":       {Internal(errors.New("x")), http.StatusInternalServerError},
	}
	for name, tc := range cases {
		if tc.err.Status != tc.status {
			t.Errorf("%s: expected %d got %d", name, tc.status, tc.err.Status)
		}
		if tc.err.Code == "" || tc.err.Message == "" {
			t.Errorf("%s: code and message must be populated", name)
		}
	}
}
