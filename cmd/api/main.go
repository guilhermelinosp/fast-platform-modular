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

	"github.com/guilhermelinosp/golang-api-template/internal/api"
	"github.com/guilhermelinosp/golang-api-template/internal/api/ginadapter"
	"github.com/guilhermelinosp/golang-api-template/internal/config"
	"github.com/guilhermelinosp/golang-api-template/internal/hello"
	"github.com/guilhermelinosp/golang-api-template/internal/observability"
	"github.com/guilhermelinosp/golang-api-template/internal/server"
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
	// 1. Application context — created ONCE here; everything below inherits it
	//    (telemetry uses it as the root for its internal spans/logs).
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// 2. Configuration (APP_*; telemetry envs stay with the library).
	cfg, err := config.Load(config.Build{Version: version, Commit: commit, Date: date})
	if err != nil {
		return err
	}

	// 3. Telemetry — single integration point with hellnet-lib-telemetry.
	tel, err := observability.Init(ctx, cfg)
	if err != nil {
		return err
	}
	logger := observability.Logger(tel)

	// 4. Business dependencies (composition, no DI framework).
	helloHandler := hello.NewHandler(hello.NewService(logger))

	// 5. HTTP boundary: Gin adapter + platform + business routes.
	router := ginadapter.New(ginadapter.Config{
		Logger:             logger,
		ReleaseMode:        cfg.IsProduction(),
		CORSAllowedOrigins: cfg.CORSAllowedOrigins,
		BodyLimit:          cfg.BodyLimit,
	})
	platform := observability.Platform(tel)
	api.RegisterPlatform(router, api.ServiceInfo{
		Name:    cfg.Name,
		Version: cfg.Build.Version,
		Commit:  cfg.Build.Commit,
		BuiltAt: cfg.Build.Date,
	}, api.Deps{
		Platform: api.PlatformHandlers{
			Live:   platform.Live,
			Ready:  platform.Ready,
			Health: platform.Health,
		},
		Routes: helloHandler.Routes(),
	})

	// 6. Server owns the network; telemetry middleware is applied by the
	//    library wrapper around the whole handler tree (logs/metrics/traces).
	httpHandler := observability.RequestTelemetry(tel, router)

	srv := server.New(cfg, logger, httpHandler)
	if err := srv.Run(ctx); err != nil {
		logger.Error("runtime error", slog.Any("error", err))
	}

	// 7. Telemetry shuts down LAST so final logs/traces/metrics still flush.
	logger.Info("shutting down: flushing telemetry")
	if err := tel.Shutdown(); err != nil {
		logger.Warn("telemetry shutdown reported errors", slog.Any("error", err))
	}
	return nil
}
