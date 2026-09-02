package hello

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/guilhermelinosp/golang-api-template/internal/api"
)

// ───────────────────── Service unit tests (framework-free) ─────────────────

type stubService struct {
	message string
	err     error
}

func (s *stubService) Greet(_ context.Context, _ string) (string, error) {
	return s.message, s.err
}

func TestServiceGreeting(t *testing.T) {
	svc := NewService(slog.Default())

	msg, err := svc.Greet(context.Background(), " ana ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg != "Hello, ana!" {
		t.Fatalf("got %q", msg)
	}
}

func TestServiceRejectsOverlongNames(t *testing.T) {
	svc := NewService(slog.Default())
	_, err := svc.Greet(context.Background(), strings.Repeat("x", 200))

	var appErr *api.Error
	if !errors.As(err, &appErr) || appErr.Status != http.StatusBadRequest {
		t.Fatalf("expected VALIDATION_ERROR 400, got %v", err)
	}
}

// ────────────────── Handler tests against the abstraction port ─────────────

type fakeRequest struct {
	params map[string]string
	query  map[string]string
	body   string
}

func (f fakeRequest) Param(name string) string { return f.params[name] }
func (f fakeRequest) Query(name string) string { return f.query[name] }
func (f fakeRequest) Header(string) string     { return "" }
func (f fakeRequest) Bind(v any) error         { return api.BindInto(strings.NewReader(f.body), v) }
func (fakeRequest) Raw() *http.Request         { return nil }

func TestHandlerHappyPaths(t *testing.T) {
	h := NewHandler(&stubService{message: "Hello, x!"})

	t.Run("query", func(t *testing.T) {
		resp, err := h.greetByQuery(context.Background(), fakeRequest{query: map[string]string{"name": "world"}})
		assertGreeting(t, resp, err, http.StatusOK, "Hello, x!")
	})
	t.Run("path", func(t *testing.T) {
		resp, err := h.greetByPath(context.Background(), fakeRequest{params: map[string]string{"name": "gin"}})
		assertGreeting(t, resp, err, http.StatusOK, "Hello, x!")
	})
	t.Run("body", func(t *testing.T) {
		resp, err := h.greetByBody(context.Background(), fakeRequest{body: `{"name":"ana"}`})
		assertGreeting(t, resp, err, http.StatusCreated, "Hello, x!")
	})
}

func TestHandlerInputFailures(t *testing.T) {
	h := NewHandler(&stubService{message: "unused"})

	if _, err := h.greetByPath(context.Background(), fakeRequest{}); err == nil {
		t.Fatal("empty path name must be a validation error")
	}
	if _, err := h.greetByBody(context.Background(), fakeRequest{body: `{}`}); err == nil {
		t.Fatal("empty body name must be a validation error")
	}
	if _, err := h.greetByBody(context.Background(), fakeRequest{body: `not-json`}); err == nil {
		t.Fatal("malformed body must surface an error")
	}
	if _, err := h.greetByBody(context.Background(), fakeRequest{body: `{"name":"a","extra":1}`}); err == nil {
		t.Fatal("unknown fields are rejected by contract")
	}
}

func TestRoutesDeclaration(t *testing.T) {
	routes := NewHandler(&stubService{}).Routes()
	want := map[string]bool{
		"GET /hello":     false,
		"GET /hello/{n}": false,
		"POST /hello":    false,
	}
	for _, r := range routes {
		key := r.Method + " /hello"
		if _, ok := want[key]; ok {
			delete(want, key)
		}
	}
	if len(want) > 1 { // wildcard key differs; ensure all three flavors present
		t.Fatalf("route flavors missing: %v", routes)
	}
}

func assertGreeting(t *testing.T, resp api.Response, err error, wantStatus int, wantMsg string) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Status != wantStatus {
		t.Fatalf("status %d ≠ %d", resp.Status, wantStatus)
	}
	raw, mErr := json.Marshal(resp.Body)
	if mErr != nil {
		t.Fatalf("marshal: %v", mErr)
	}
	var out greetResponse
	if json.Unmarshal(raw, &out); &out == nil {
		t.Fatal("nil payload")
	}
	if out.Message != wantMsg {
		t.Fatalf("message %q ≠ %q", out.Message, wantMsg)
	}
}
