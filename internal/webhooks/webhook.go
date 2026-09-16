package webhooks

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/guilhermelinosp/fast-platform-modular/internal/rides"
	"github.com/guilhermelinosp/hellnet-lib-environments/environments"
)

// FAST_NOTIFICATION_WEBHOOK_* variables configure webhook delivery. The URL
// enables the notification consumer; the secret must accompany it.
const (
	WebhookURLEnv = "FAST_NOTIFICATION_WEBHOOK_URL"
	//nolint:gosec // G101: env var name, not a credential value
	WebhookSecretEnv      = "FAST_NOTIFICATION_WEBHOOK_SECRET"
	WebhookTimeoutEnv     = "FAST_NOTIFICATION_WEBHOOK_TIMEOUT"
	WebhookRetriesEnv     = "FAST_NOTIFICATION_WEBHOOK_MAX_RETRIES"
	WebhookBackoffEnv     = "FAST_NOTIFICATION_WEBHOOK_BACKOFF"
	WebhookEvent          = "ride.requested.v1"
	SignatureHeader       = "X-Notification-Signature"
	EventIDHeader         = "X-Notification-Event-ID"
	defaultWebhookTimeout = 5 * time.Second
	defaultWebhookRetries = 3
	defaultWebhookBackoff = 250 * time.Millisecond
	maxWebhookRetries     = 10
	maxWebhookBackoff     = 30 * time.Second
)

// Config controls delivery of notifications to the external webhook.
// MaxRetries is the number of retries after the first HTTP attempt.
type Config struct {
	URL        string
	Secret     string
	Timeout    time.Duration
	MaxRetries int
	Backoff    time.Duration
	HTTPClient *http.Client
}

// NotificationPayload is the stable outbound contract. DriverID is omitted
// until the consumed event or a future routing layer actually supplies one.
type NotificationPayload struct {
	Event    string          `json:"event"`
	EventID  string          `json:"event_id"`
	DriverID string          `json:"driver_id,omitempty"`
	Ride     rides.Requested `json:"ride"`
}

// Dispatcher posts ride-requested notifications to the configured webhook.
type Dispatcher struct {
	config Config
	client *http.Client
}

// NewDispatcherWithConfig validates config and creates a webhook dispatcher.
// Application startup should normally use NewDispatcher so configuration is
// resolved from the environment first.
func NewDispatcherWithConfig(config Config) (*Dispatcher, error) {
	config.URL = strings.TrimSpace(config.URL)
	if err := validateConfig(config); err != nil {
		return nil, err
	}
	if config.Timeout <= 0 {
		config.Timeout = defaultWebhookTimeout
	}
	if config.MaxRetries < 0 {
		config.MaxRetries = defaultWebhookRetries
	}
	if config.Backoff < 0 {
		config.Backoff = defaultWebhookBackoff
	}
	client := config.HTTPClient
	if client == nil {
		client = &http.Client{
			Timeout: config.Timeout,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
	}
	return &Dispatcher{config: config, client: client}, nil
}

// NewDispatcher loads the webhook configuration. An empty URL disables
// the notification consumer, which keeps local development and tests free of
// an accidental outbound dependency.
func NewDispatcher() (*Dispatcher, error) {
	config, enabled, err := ConfigFromEnv()
	if err != nil {
		return nil, err
	}
	if !enabled {
		return nil, nil
	}
	return NewDispatcherWithConfig(config)
}

// ConfigFromEnv parses the FAST_NOTIFICATION_WEBHOOK_* settings. The boolean
// result is false when FAST_NOTIFICATION_WEBHOOK_URL is unset or blank.
func ConfigFromEnv() (Config, bool, error) {
	config := Config{
		URL:    envString(WebhookURLEnv),
		Secret: environments.GetString("", "", WebhookSecretEnv, ""),
	}
	if config.URL == "" {
		return Config{}, false, nil
	}

	var err error
	if config.Timeout, err = durationEnv(WebhookTimeoutEnv, defaultWebhookTimeout); err != nil {
		return Config{}, true, err
	}
	if config.Backoff, err = durationEnv(WebhookBackoffEnv, defaultWebhookBackoff); err != nil {
		return Config{}, true, err
	}
	config.MaxRetries, err = intEnv(WebhookRetriesEnv, defaultWebhookRetries)
	if err != nil {
		return Config{}, true, err
	}
	if err := validateConfig(config); err != nil {
		return Config{}, true, err
	}
	return config, true, nil
}

// BuildNotificationPayload marshals the webhook body. driverID is intentionally
// an explicit argument so callers cannot infer or invent a driver assignment.
func BuildNotificationPayload(event rides.Requested, driverID string) ([]byte, error) {
	return json.Marshal(NotificationPayload{
		Event:    WebhookEvent,
		EventID:  event.EventID,
		DriverID: strings.TrimSpace(driverID),
		Ride:     event,
	})
}

// SignPayload returns the HMAC-SHA256 signature used in SignatureHeader.
func SignPayload(payload []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(payload)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// Notify sends one ride-requested event. Temporary transport failures and 408,
// 429, and 5xx responses are retried with bounded exponential backoff.
func (d *Dispatcher) Notify(ctx context.Context, event rides.Requested, driverID string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	payload, err := BuildNotificationPayload(event, driverID)
	if err != nil {
		return fmt.Errorf("build notification payload: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt <= d.config.MaxRetries; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		lastErr = d.post(ctx, payload, event.EventID)
		if lastErr == nil {
			return nil
		}
		if !retryableNotificationError(lastErr) || attempt == d.config.MaxRetries {
			break
		}
		if err := sleepWebhookBackoff(ctx, d.config.Backoff, attempt); err != nil {
			return err
		}
	}
	return fmt.Errorf("notification webhook delivery failed after %d attempt(s): %w", d.config.MaxRetries+1, lastErr)
}

func (d *Dispatcher) post(ctx context.Context, payload []byte, eventID string) error {
	requestCtx, cancel := context.WithTimeout(ctx, d.config.Timeout)
	defer cancel()
	request, err := http.NewRequestWithContext(requestCtx, http.MethodPost, d.config.URL, strings.NewReader(string(payload)))
	if err != nil {
		return fmt.Errorf("create webhook request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(SignatureHeader, SignPayload(payload, d.config.Secret))
	if eventID != "" {
		request.Header.Set(EventIDHeader, eventID)
		request.Header.Set("Idempotency-Key", eventID)
	}

	response, err := d.client.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("webhook request: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64*1024))
	if response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices {
		return nil
	}
	return &statusError{code: response.StatusCode}
}

type statusError struct{ code int }

func (e *statusError) Error() string { return fmt.Sprintf("webhook returned HTTP %d", e.code) }

func retryableNotificationError(err error) bool {
	if status, ok := errors.AsType[*statusError](err); ok {
		return status.code == http.StatusRequestTimeout || status.code == http.StatusTooManyRequests || status.code >= 500
	}
	return !errors.Is(err, context.Canceled)
}

func sleepWebhookBackoff(ctx context.Context, base time.Duration, retry int) error {
	if base <= 0 {
		return nil
	}
	delay := base
	for i := 0; i < retry && delay < maxWebhookBackoff; i++ {
		delay *= 2
	}
	if delay > maxWebhookBackoff {
		delay = maxWebhookBackoff
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func validateConfig(config Config) error {
	parsed, err := url.Parse(config.URL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("%s must be an absolute http or https URL", WebhookURLEnv)
	}
	if config.Secret == "" {
		return fmt.Errorf("%s is required when %s is configured", WebhookSecretEnv, WebhookURLEnv)
	}
	if config.Timeout < 0 || config.Timeout > time.Minute {
		return fmt.Errorf("%s must be between 0 and 1m", WebhookTimeoutEnv)
	}
	if config.MaxRetries < 0 || config.MaxRetries > maxWebhookRetries {
		return fmt.Errorf("%s must be between 0 and %d", WebhookRetriesEnv, maxWebhookRetries)
	}
	if config.Backoff < 0 || config.Backoff > maxWebhookBackoff {
		return fmt.Errorf("%s must be between 0 and %s", WebhookBackoffEnv, maxWebhookBackoff)
	}
	return nil
}

func durationEnv(name string, fallback time.Duration) (time.Duration, error) {
	raw := envString(name)
	if raw == "" {
		return fallback, nil
	}
	duration, err := environments.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("%s must be a duration: %w", name, err)
	}
	return duration, nil
}

func intEnv(name string, fallback int) (int, error) {
	raw := envString(name)
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer: %w", name, err)
	}
	return value, nil
}

func envString(name string) string {
	return strings.TrimSpace(environments.GetString("", "", name, ""))
}
