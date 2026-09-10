// Package main bootstraps the API.
//
// Deliberately tiny: it only wires explicit dependencies in lifecycle order
// and owns the shutdown sequence. Every real decision lives inside packages:
//
//	context → config → telemetry → api adapter → http server → shutdown
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/guilhermelinosp/hellnet-lib-api/adapter"
	"github.com/guilhermelinosp/hellnet-lib-api/api"
	"github.com/guilhermelinosp/hellnet-lib-api/config"
	"github.com/guilhermelinosp/hellnet-lib-api/server"
	"github.com/guilhermelinosp/hellnet-lib-telemetry/telemetry"

	"github.com/guilhermelinosp/fast-platform-modular/internal/ride"
)

// Build metadata injected via -ldflags (see Makefile, Containerfile, CI).
var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", slog.Any("error", err))
		os.Exit(1)
	}
}

func run() error {
	// 1. Application context — owns the HTTP server lifecycle and shutdown.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// 2. Configuration (HELLNET_*; telemetry envs stay with the library).
	cfg, err := config.New()
	if err != nil {
		return err
	}

	// 3. Telemetry — the library owns environment loading and its base context.
	tel, err := telemetry.New()
	if err != nil {
		return err
	}
	logger := tel.Logger

	// 4. Business dependencies (composition, no DI framework).
	helloHandler := ride.NewHandler(ride.NewService(logger))

	// 5. HTTP boundary: Gin adapter + platform + business routes.
	router := adapter.New(cfg, logger)
	api.RegisterPlatform(router, api.ServiceInfo{
		Name:    cfg.Name,
		Version: version,
		Commit:  commit,
		BuiltAt: date,
	}, api.Deps{
		Platform: api.PlatformHandlers{
			Live:   tel.Live(),
			Ready:  tel.Ready(),
			Health: tel.Health(),
		},
		Routes: helloHandler.Routes(),
	})

	// 6. Instrument the complete handler tree once so platform and business
	// routes share the library's HTTP logs, metrics, and tracing.
	httpHandler := telemetry.Middleware(tel, router)

	srv := server.New(cfg, logger, httpHandler)
	if err := srv.Run(ctx); err != nil {
		logger.Error("runtime error", slog.Any("error", err))
	}

	// 7. Shutdown telemetry last so providers and profiling flush after the
	// HTTP server has drained in-flight requests.
	logger.Info("shutting down: flushing telemetry")
	if err := tel.Shutdown(); err != nil {
		logger.Warn("telemetry shutdown reported errors", slog.Any("error", err))
	}
	return nil
}
