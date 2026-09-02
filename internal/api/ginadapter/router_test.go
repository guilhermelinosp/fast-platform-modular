package ginadapter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/guilhermelinosp/golang-api-template/internal/api"
)

type pingRequest struct{}

func (pingRequest) Handle(_ context.Context, req api.Request) (api.Response, error) {
	name := req.Query("name")
	if name == "" {
		name = strings.TrimSpace(req.Param("name"))
	}
	body, _ := json.Marshal(map[string]string{"message": "hi " + name})
	return api.JSON(http.StatusOK, json.RawMessage(body)), nil
}

func newTestRouter(t *testing.T, extra func(r *Router)) http.Handler {
	t.Helper()
	t.Setenv("GIN_MODE", "release") // quiet output; mode semantics tested implicitly
	r := New(Config{Logger: slog.Default()})
	api.RegisterPlatform(r, api.ServiceInfo{Name: "test"}, api.Deps{
		// Fake platform handlers keep this suite focused on routing of
		// mounted raw endpoints; the real ones come from hellnet-lib-telemetry.
		Platform: api.PlatformHandlers{
			Live:    okHandler("live"),
			Ready:   okHandler("ready"),
			Health:  okHandler("health"),
			Metrics: okHandler("metrics"),
		},
	})
	if extra != nil {
		extra(r)
	}
	return r
}

func okHandler(name string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Fake", name)
		w.WriteHeader(http.StatusOK)
	})
}

func do(t *testing.T, h http.Handler, method, target string, body io.Reader) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, target, body)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// ───────────────────────── Platform endpoints ─────────────────────────

func TestPlatformEndpointsRespond(t *testing.T) {
	h := newTestRouter(t, nil)

	for _, path := range []string{"/live", "/ready", "/health", "/metrics"} {
		rec := do(t, h, http.MethodGet, path, nil)
		if rec.Code != http.StatusOK {
			t.Errorf("%s: expected 200 got %d", path, rec.Code)
		}
	}
}

func TestRootReturnsServiceInfo(t *testing.T) {
	rec := do(t, newTestRouter(t, nil), http.MethodGet, "/", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d", rec.Code)
	}
	var payload map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("root must return JSON: %v", err)
	}
	if payload["service"] != "test" {
		t.Fatalf("unexpected service: %v", payload)
	}
}

// ───────────────────────── Business route flavors ─────────────────────

func TestWildcardAndQueryRoutes(t *testing.T) {
	h := newTestRouter(t, func(r *Router) {
		v1 := r.Group("/api/v1")
		v1.Handle(http.MethodGet, "/ping", pingRequest{})
		v1.Handle(http.MethodGet, "/hello/{name}", pingRequest{})
	})

	got := do(t, h, http.MethodGet, "/api/v1/hello/gin", nil)
	if !strings.Contains(got.Body.String(), "hi gin") {
		t.Fatalf("wildcard failed: %s", got.Body.String())
	}

	got = do(t, h, http.MethodGet, "/api/v1/ping?name=query", nil)
	if !strings.Contains(got.Body.String(), "hi query") {
		t.Fatalf("query failed: %s", got.Body.String())
	}
}

// ───────────────────────── Error handling ──────────────────────────────

func TestValidationAndBindErrors(t *testing.T) {
	type in struct {
		Name string `json:"name"`
	}
	h := newTestRouter(t, func(r *Router) {
		v1 := r.Group("/api/v1")
		v1.Handle(http.MethodPost, "/echo", api.HandlerFunc(func(_ context.Context, req api.Request) (api.Response, error) {
			var v in
			if err := req.Bind(&v); err != nil {
				return api.Response{}, err
			}
			return api.JSON(http.StatusOK, v), nil
		}))
	})

	t.Run("valid body", func(t *testing.T) {
		rec := do(t, h, http.MethodPost, "/api/v1/echo", strings.NewReader(`{"name":"ana"}`))
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "ana") {
			t.Fatalf("got %d %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("unknown field rejected as validation error", func(t *testing.T) {
		rec := do(t, h, http.MethodPost, "/api/v1/echo", strings.NewReader(`{"name":"ana","typo":true}`))
		assertErrorEnvelope(t, rec, http.StatusBadRequest, "VALIDATION_ERROR")
	})

	t.Run("malformed JSON rejected", func(t *testing.T) {
		rec := do(t, h, http.MethodPost, "/api/v1/echo", strings.NewReader(`{oops`))
		assertErrorEnvelope(t, rec, http.StatusBadRequest, "VALIDATION_ERROR")
	})
}

func TestOversizedBodyBecomes413(t *testing.T) {
	type in struct{}
	h := New(Config{Logger: slog.Default(), BodyLimit: 64})
	h.Group("/api/v1").Handle(http.MethodPost, "/x", api.HandlerFunc(func(_ context.Context, req api.Request) (api.Response, error) {
		var v in
		if err := req.Bind(&v); err != nil {
			return api.Response{}, err
		}
		return api.NoContent(), nil
	}))

	big := strings.NewReader(fmt.Sprintf(`{"pad":"%s"}`, strings.Repeat("a", 512)))
	rec := do(t, h, http.MethodPost, "/api/v1/x", big)
	assertErrorEnvelope(t, rec, http.StatusRequestEntityTooLarge, "REQUEST_TOO_LARGE")
}

func TestHandlerErrorsBecomeEnvelope(t *testing.T) {
	h := newTestRouter(t, func(r *Router) {
		r.Group("/api/v1").Handle(http.MethodGet, "/boom",
			api.HandlerFunc(func(context.Context, api.Request) (api.Response, error) {
				return api.Response{}, errors.New("very secret internals")
			}))
	})
	rec := do(t, h, http.MethodGet, "/api/v1/boom", nil)
	assertErrorEnvelope(t, rec, http.StatusInternalServerError, "INTERNAL_ERROR")

	if strings.Contains(rec.Body.String(), "secret") {
		t.Fatal("internal cause leaked into response body")
	}
}

func TestNotFoundAndMethodNotAllowedAreJSON(t *testing.T) {
	h := newTestRouter(t, func(r *Router) {
		r.Group("/api/v1").Handle(api.MethodGet, "/only-get", pingRequest{})
	})

	assertErrorEnvelope(t, do(t, h, http.MethodGet, "/nowhere", nil), http.StatusNotFound, "NOT_FOUND")
	assertErrorEnvelope(t, do(t, h, http.MethodPost, "/api/v1/only-get", nil),
		http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED")
}

func TestRecoveryFromPanic(t *testing.T) {
	h := newTestRouter(t, func(r *Router) {
		r.Handle(http.MethodGet, "/panic", api.HandlerFunc(func(context.Context, api.Request) (api.Response, error) {
			panic("kaboom with secrets")
		}))
	})
	rec := do(t, h, http.MethodGet, "/panic", nil)
	assertErrorEnvelope(t, rec, http.StatusInternalServerError, "INTERNAL_ERROR")
	if strings.Contains(rec.Body.String(), "kaboom") {
		t.Fatal("panic value leaked into response")
	}
}

// ───────────────────────── Middleware behavior ─────────────────────────

func TestRequestIDGeneratedAndEchoed(t *testing.T) {
	h := newTestRouter(t, nil)

	rec := do(t, h, http.MethodGet, "/live", nil)
	if id := rec.Header().Get(requestIDHeader); len(id) != 32 { // hex(16 bytes)
		t.Fatalf("expected generated 32-char request id, got %q", id)
	}

	req := httptest.NewRequest(http.MethodGet, "/live", nil)
	req.Header.Set(requestIDHeader, "my-trace-42")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if got := rec.Header().Get(requestIDHeader); got != "my-trace-42" {
		t.Fatalf("expected incoming id honored, got %q", got)
	}

	// Malicious/oversized ids are replaced.
	req.Header.Set(requestIDHeader, "bad;DROP TABLE<script>")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	id := rec.Header().Get(requestIDHeader)
	if len(id) != 32 || strings.Contains(id, ";") {
		t.Fatalf("unsafe id not sanitized, got %q", id)
	}
}

func TestSecurityHeadersAlwaysApplied(t *testing.T) {
	rec := do(t, newTestRouter(t, nil), http.MethodGet, "/live", nil)
	for _, header := range []string{
		"X-Content-Type-Options",
		"X-Frame-Options",
		"Referrer-Policy",
		"Content-Security-Policy",
	} {
		if rec.Header().Get(header) == "" {
			t.Errorf("missing security header %s", header)
		}
	}
}

func TestCORSAllowedOriginFlow(t *testing.T) {
	cfg := Config{
		Logger:             slog.Default(),
		CORSAllowedOrigins: []string{"https://app.example.com"},
	}
	corsRouter := New(cfg)
	api.RegisterPlatform(corsRouter, api.ServiceInfo{}, api.Deps{})

	req := httptest.NewRequest(http.MethodOptions, "/", nil)
	req.Header.Set("Origin", "https://app.example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")
	rec := httptest.NewRecorder()
	corsRouter.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("preflight must short-circuit 204, got %d", rec.Code)
	}
	if rec.Header().Get("Access-Control-Allow-Origin") != "https://app.example.com" {
		t.Fatalf("allowed origin missing: %v", rec.Header())
	}

	// Not-allowlisted origin gets no CORS grant.
	req = httptest.NewRequest(http.MethodGet, "/live", nil)
	req.Header.Set("Origin", "https://evil.example.net")
	rec = httptest.NewRecorder()
	corsRouter.ServeHTTP(rec, req)
	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("unlisted origin must not receive CORS grant")
	}
}

// ───────────────────────── Helpers ─────────────────────────────────────

func assertErrorEnvelope(t *testing.T, rec *httptest.ResponseRecorder, wantStatus int, wantCode string) {
	t.Helper()
	if rec.Code != wantStatus {
		t.Fatalf("status: expected %d got %d (%s)", wantStatus, rec.Code, rec.Body.String())
	}
	var envelope struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("error responses must be JSON envelopes: %v (%s)", err, rec.Body.String())
	}
	if envelope.Error.Code != wantCode {
		t.Fatalf("code: expected %s got %s", wantCode, envelope.Error.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("content-type: expected application/json got %s", ct)
	}
}
