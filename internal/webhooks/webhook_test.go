package webhooks

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/guilhermelinosp/fast-platform-modular/internal/rides"
)

func TestBuildNotificationPayload(t *testing.T) {
	event := rides.Requested{EventID: "event-1", RideID: "ride-1", RiderID: "rider-1", PickupLatitude: 1.2}
	body, err := BuildNotificationPayload(event, "")
	if err != nil {
		t.Fatalf("BuildNotificationPayload() error = %v", err)
	}

	var got NotificationPayload
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("payload is not JSON: %v", err)
	}
	if got.Event != WebhookEvent || got.EventID != event.EventID {
		t.Fatalf("envelope = %+v, want event %q and ID %q", got, WebhookEvent, event.EventID)
	}
	if got.DriverID != "" {
		t.Errorf("driver_id = %q, want omitted/empty", got.DriverID)
	}
	if got.Ride.RideID != event.RideID || got.Ride.PickupLatitude != event.PickupLatitude {
		t.Errorf("ride = %+v, want %+v", got.Ride, event)
	}

	body, err = BuildNotificationPayload(event, "driver-1")
	if err != nil {
		t.Fatalf("BuildNotificationPayload() with driver error = %v", err)
	}
	if string(body) == "" || !containsJSONField(body, "driver_id", "driver-1") {
		t.Fatalf("payload does not contain supplied driver_id: %s", body)
	}
}

func TestSignPayload(t *testing.T) {
	body := []byte(`{"event":"ride.requested.v1"}`)
	secret := "test-secret"
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	want := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if got := SignPayload(body, secret); got != want {
		t.Fatalf("SignPayload() = %q, want %q", got, want)
	}
}

func TestDispatcherRetriesTransientHTTPStatus(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get(SignatureHeader) == "" || r.Header.Get(EventIDHeader) != "event-1" {
			t.Errorf("missing webhook headers: signature=%q event_id=%q", r.Header.Get(SignatureHeader), r.Header.Get(EventIDHeader))
		}
		if calls.Load() < 3 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	dispatcher, err := NewDispatcherWithConfig(Config{URL: server.URL, Secret: "secret", MaxRetries: 2, Backoff: 0, Timeout: time.Second})
	if err != nil {
		t.Fatalf("NewDispatcher() error = %v", err)
	}
	if err := dispatcher.Notify(context.Background(), rides.Requested{EventID: "event-1"}, ""); err != nil {
		t.Fatalf("Notify() error = %v", err)
	}
	if got := calls.Load(); got != 3 {
		t.Fatalf("webhook calls = %d, want 3", got)
	}
}

func TestDispatcherRetriesRateLimitAndPreservesSignedPayload(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("content type = %q, want application/json", r.Header.Get("Content-Type"))
		}
		if got, want := r.Header.Get(SignatureHeader), SignPayload(body, "secret"); got != want {
			t.Errorf("signature = %q, want %q", got, want)
		}
		var payload NotificationPayload
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Errorf("decode payload: %v", err)
		}
		if payload.Ride.RideID != "ride-1" {
			t.Errorf("ride id = %q, want ride-1", payload.Ride.RideID)
		}
		if calls.Load() == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	dispatcher, err := NewDispatcherWithConfig(Config{
		URL:        server.URL,
		Secret:     "secret",
		MaxRetries: 1,
		Backoff:    0,
		Timeout:    time.Second,
	})
	if err != nil {
		t.Fatalf("NewDispatcher() error = %v", err)
	}
	if err := dispatcher.Notify(context.Background(), rides.Requested{
		EventID: "event-1",
		RideID:  "ride-1",
	}, ""); err != nil {
		t.Fatalf("Notify() error = %v", err)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("webhook calls = %d, want 2", got)
	}
}

func TestDispatcherStopsWhenContextIsCanceled(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()

	dispatcher, err := NewDispatcherWithConfig(Config{
		URL:        server.URL,
		Secret:     "secret",
		MaxRetries: 5,
		Backoff:    time.Second,
		Timeout:    time.Second,
	})
	if err != nil {
		t.Fatalf("NewDispatcher() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := dispatcher.Notify(ctx, rides.Requested{EventID: "event-1"}, ""); err == nil {
		t.Fatal("Notify() error = nil, want canceled context")
	}
	if got := calls.Load(); got != 0 {
		t.Fatalf("webhook calls = %d, want 0", got)
	}
}

func TestDispatcherDoesNotRetryPermanentHTTPStatus(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	dispatcher, err := NewDispatcherWithConfig(Config{URL: server.URL, Secret: "secret", MaxRetries: 5, Backoff: 0, Timeout: time.Second})
	if err != nil {
		t.Fatalf("NewDispatcher() error = %v", err)
	}
	if err := dispatcher.Notify(context.Background(), rides.Requested{EventID: "event-1"}, ""); err == nil {
		t.Fatal("Notify() error = nil, want HTTP status error")
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("webhook calls = %d, want 1", got)
	}
}

func TestDispatcherTimeoutIsBoundedAndRetried(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		time.Sleep(50 * time.Millisecond)
	}))
	defer server.Close()

	dispatcher, err := NewDispatcherWithConfig(Config{URL: server.URL, Secret: "secret", MaxRetries: 1, Backoff: 0, Timeout: 10 * time.Millisecond})
	if err != nil {
		t.Fatalf("NewDispatcher() error = %v", err)
	}
	started := time.Now()
	if err := dispatcher.Notify(context.Background(), rides.Requested{EventID: "event-1"}, ""); err == nil {
		t.Fatal("Notify() error = nil, want timeout error")
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("webhook calls = %d, want 2", got)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("timeout retries took %s, want bounded execution", elapsed)
	}
}

func TestConfigFromEnv(t *testing.T) {
	t.Setenv(WebhookURLEnv, "https://notifications.example.test/hook")
	t.Setenv(WebhookSecretEnv, "secret")
	t.Setenv(WebhookTimeoutEnv, "2s")
	t.Setenv(WebhookRetriesEnv, "4")
	t.Setenv(WebhookBackoffEnv, "100ms")

	config, enabled, err := ConfigFromEnv()
	if err != nil {
		t.Fatalf("ConfigFromEnv() error = %v", err)
	}
	if !enabled || config.URL != "https://notifications.example.test/hook" || config.Timeout != 2*time.Second || config.MaxRetries != 4 || config.Backoff != 100*time.Millisecond {
		t.Fatalf("config = %+v, enabled = %v", config, enabled)
	}
}

func TestConfigFromEnvRequiresSecretWhenEnabled(t *testing.T) {
	t.Setenv(WebhookURLEnv, "https://notifications.example.test/hook")
	t.Setenv(WebhookSecretEnv, "")
	if _, _, err := ConfigFromEnv(); err == nil {
		t.Fatal("ConfigFromEnv() error = nil, want missing secret error")
	}
}

func TestConfigFromEnvDisabledWithoutURL(t *testing.T) {
	t.Setenv(WebhookURLEnv, "")
	config, enabled, err := ConfigFromEnv()
	if err != nil || enabled || config.URL != "" {
		t.Fatalf("ConfigFromEnv() = (%+v, %v, %v), want disabled", config, enabled, err)
	}
}

func TestConfigFromEnvRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		secret  string
		timeout string
		retries string
		backoff string
	}{
		{name: "relative URL", url: "/notify", secret: "secret"},
		{name: "unsupported scheme", url: "ftp://example.test/hook", secret: "secret"},
		{name: "negative timeout", url: "https://example.test/hook", secret: "secret", timeout: "-1s"},
		{name: "too many retries", url: "https://example.test/hook", secret: "secret", retries: "11"},
		{name: "negative backoff", url: "https://example.test/hook", secret: "secret", backoff: "-1ms"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(WebhookURLEnv, tt.url)
			t.Setenv(WebhookSecretEnv, tt.secret)
			t.Setenv(WebhookTimeoutEnv, tt.timeout)
			t.Setenv(WebhookRetriesEnv, tt.retries)
			t.Setenv(WebhookBackoffEnv, tt.backoff)
			if _, _, err := ConfigFromEnv(); err == nil {
				t.Fatal("ConfigFromEnv() error = nil, want validation error")
			}
		})
	}
}

func containsJSONField(body []byte, key, want string) bool {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return false
	}
	value, ok := payload[key].(string)
	return ok && value == want
}
