package server

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/guilhermelinosp/golang-api-template/internal/config"
)

func testConfig(t *testing.T, port string) *config.Config {
	t.Helper()
	t.Setenv("APP_PORT", port)
	cfg, err := config.Load(config.Build{Version: "test"})
	if err != nil {
		t.Fatalf("config load: %v", err)
	}
	return cfg
}

// freePort grabs an available local port to avoid cross-test collisions.
func freePort(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("freePort: %v", err)
	}
	defer ln.Close()
	return itoa(ln.Addr().(*net.TCPAddr).Port)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

func TestRunServesAndShutsDownGracefully(t *testing.T) {
	port := freePort(t)
	srv := New(testConfig(t, port), slog.Default(), http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- srv.Run(ctx) }()

	// Wait for readiness then exercise a real request through the stack.
	deadline := time.Now().Add(3 * time.Second)
	for {
		resp, err := http.Get("http://127.0.0.1:" + port + "/anything")
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusTeapot {
				t.Fatalf("unexpected status: %d", resp.StatusCode)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("server never became ready: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("graceful shutdown returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after context cancellation")
	}
}

func TestRunFailsFastOnInvalidState(t *testing.T) {
	port := freePort(t)

	// Occupy the wildcard port first; the server under test (which binds all
	// interfaces via ":PORT") must surface the bind error instead of hanging.
	blocker, err := net.Listen("tcp", ":"+port)
	if err != nil {
		t.Fatalf("blocker listen: %v", err)
	}
	defer blocker.Close()

	srv := New(testConfig(t, port), slog.Default(), http.NotFoundHandler())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Run(ctx) }()

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("expected bind failure to propagate as error")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run hung instead of failing fast")
	}
}
