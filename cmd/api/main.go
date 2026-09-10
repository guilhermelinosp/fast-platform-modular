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

	"github.com/guilhermelinosp/fast-platform-modular/internal/rider"
	"github.com/guilhermelinosp/hellnet-lib-api/adapter"
	"github.com/guilhermelinosp/hellnet-lib-api/api"
	"github.com/guilhermelinosp/hellnet-lib-api/config"
	"github.com/guilhermelinosp/hellnet-lib-api/server"

	"github.com/guilhermelinosp/hellnet-lib-database/database"
	"github.com/guilhermelinosp/hellnet-lib-kafka/kafka"
	"github.com/guilhermelinosp/hellnet-lib-telemetry/telemetry"
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

	tel, err := telemetry.New()
	if err != nil {
		return err
	}

	requested, err := kafka.NewProducer[rider.Requested]()
	if err != nil {
		return err
	}
	defer func() { _ = requested.Close() }()

	accepted, err := kafka.NewProducer[rider.Accepted]()
	if err != nil {
		return err
	}
	defer func() { _ = accepted.Close() }()

	db, err := database.New(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()

	repository := rider.NewDatabase(db)

	producers := rider.NewPublisher(requested, accepted)

	handler := rider.NewHandler(rider.NewService(tel.Logger, repository, producers))

	router := adapter.New(cfg, tel.Logger)

	api.RegisterPlatform(router, api.Deps{
		Routes: handler.Routes(),
		Platform: api.PlatformHandlers{
			Live:   tel.Live(),
			Ready:  tel.Ready(),
			Health: tel.Health(),
		},
	})

	srv := server.New(cfg, tel.Logger, telemetry.Middleware(tel, router))
	if err := srv.Run(ctx); err != nil {
		tel.Logger.Error("runtime error", slog.Any("error", err))
		return err
	}
	return nil
}
