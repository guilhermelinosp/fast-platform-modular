// Package main bootstraps the API.
//
// Deliberately tiny: it only wires explicit dependencies in lifecycle order
// and owns the shutdown sequence. Every real decision lives inside packages:
//
//	telemetry → platform (config + adapter + server) → routes → run → shutdown
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/guilhermelinosp/hellnet-lib-api/api"
	"github.com/guilhermelinosp/hellnet-lib-api/platform"
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

	// 2. Telemetry — the library owns environment loading and its base context.
	tel, err := telemetry.New()
	if err != nil {
		return err
	}

	// 3. Platform — single entry point: config + adapter + server wired once.
	app, err := platform.New(tel)
	if err != nil {
		return err
	}

	// 4. Business dependencies (composition, no DI framework).
	helloHandler := ride.NewHandler(ride.NewService(tel.Logger))

	// 5. Register platform + business routes on the wired router.
	app.Register(api.ServiceInfo{
		Name:    app.Config.Name,
		Version: version,
		Commit:  commit,
		BuiltAt: date,
	}, api.Deps{
		Platform: app.PlatformHandlers(),
		Routes:   helloHandler.Routes(),
	})

	// 6. Serve until signal, then drain connections gracefully.
	if err := app.Run(ctx); err != nil {
		tel.Logger.Error("runtime error", slog.Any("error", err))
	}

	// 7. Shutdown telemetry last so providers and profiling flush after the
	// HTTP server has drained in-flight requests.
	logger := app.Logger
	logger.Info("shutting down: flushing telemetry")
	if err := app.Shutdown(); err != nil {
		logger.Warn("telemetry shutdown reported errors", slog.Any("error", err))
	}
	return nil
}
