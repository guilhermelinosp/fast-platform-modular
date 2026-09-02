// Package server owns the network lifecycle: an explicitly configured
// http.Server plus graceful shutdown. Gin lives INSIDE as a plain
// http.Handler — the server never knows which framework produced it.
package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/guilhermelinosp/golang-api-template/internal/config"
)

// Server wraps net/http with template defaults (secure timeouts) and an
// ordered shutdown routine.
type Server struct {
	http            *http.Server
	shutdownTimeout time.Duration
	logger          *slog.Logger
}

// New builds the HTTP server. handler is the fully assembled router
// (ginadapter.Router satisfies http.Handler).
func New(cfg *config.Config, logger *slog.Logger, handler http.Handler) *Server {
	return &Server{
		http: &http.Server{
			Addr:              ":" + cfg.Port,
			Handler:           handler,
			ReadHeaderTimeout: cfg.ReadHeaderTimeout, // slowloris guard — never zero
			ReadTimeout:       cfg.ReadTimeout,
			WriteTimeout:      cfg.WriteTimeout,
			IdleTimeout:       cfg.IdleTimeout,
			MaxHeaderBytes:    1 << 20,
		},
		shutdownTimeout: cfg.ShutdownTimeout,
		logger:          logger,
	}
}

// Run serves until ctx is canceled (SIGINT/SIGTERM via signal.NotifyContext)
// or ListenAndServe fails, then drains in-flight requests up to the
// configured shutdown timeout. Returns nil on clean signal-driven exits.
func (s *Server) Run(ctx context.Context) error {
	serveErr := make(chan error, 1)
	go func() {
		s.logger.Info("HTTP server listening",
			slog.String("addr", s.http.Addr),
			slog.String("read_timeout", s.http.ReadTimeout.String()),
		)
		serveErr <- s.http.ListenAndServe()
	}()

	select {
	case err := <-serveErr:
		if errors.Is(err, http.ErrServerClosed) {
			return nil // shutdown already driven elsewhere; nothing to do
		}
		return fmt.Errorf("server: listenAndServe failed: %w", err)

	case <-ctx.Done():
		drainCtx, cancel := context.WithTimeout(context.Background(), s.shutdownTimeout)
		defer cancel()
		if err := s.http.Shutdown(drainCtx); err != nil {
			return fmt.Errorf("server: graceful shutdown incomplete: %w", err)
		}
		s.logger.Info("HTTP server drained connections gracefully")
		return nil
	}
}
