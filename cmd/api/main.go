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

	"github.com/guilhermelinosp/fast-platform-modular/internal/drives"
	"github.com/guilhermelinosp/fast-platform-modular/internal/outbox"
	"github.com/guilhermelinosp/fast-platform-modular/internal/rides"
	"github.com/guilhermelinosp/fast-platform-modular/internal/sockets"
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

	db, err := database.New()
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()

	requested, err := kafka.NewProducer[rides.Requested]()
	if err != nil {
		return err
	}
	defer func() { _ = requested.Close() }()

	accepted, err := kafka.NewProducer[rides.Accepted]()
	if err != nil {
		return err
	}
	defer func() { _ = accepted.Close() }()

	producer := outbox.NewProducer(requested, accepted)
	listener, err := outbox.NewListener(db, producer)
	if err != nil {
		return err
	}
	defer listener.Close()

	riderService := rides.NewService(tel.Logger, rides.NewRepository(db))
	driverService := drives.NewService(tel.Logger, drives.NewRepository(db))

	rideHandler := rides.NewHandler(riderService)
	driveHandler := drives.NewHandler(driverService)

	router := adapter.New(cfg, tel.Logger)

	// Mount Socket.IO handler at /socket.io/
	socketServer := sockets.NewServer()
	router.Mount("GET", "/socket.io/", socketServer.Handler())
	router.Mount("POST", "/socket.io/", socketServer.Handler())

	notificationConsumer, err := sockets.NewConsumer(socketServer)
	if err != nil {
		return err
	}
	defer func() {
		if err := notificationConsumer.Close(); err != nil {
			tel.Logger.Warn("notification consumer close failed", slog.Any("error", err))
		}
	}()

	acceptedConsumer, err := sockets.NewAcceptedConsumer(socketServer)
	if err != nil {
		return err
	}
	defer func() {
		if err := acceptedConsumer.Close(); err != nil {
			tel.Logger.Warn("accepted consumer close failed", slog.Any("error", err))
		}
	}()

	api.RegisterPlatform(router, api.Deps{
		Routes: append(rideHandler.Routes(), driveHandler.Routes()...),
		Platform: api.PlatformHandlers{
			Live:   tel.Live(),
			Ready:  tel.Ready(),
			Health: tel.Health(),
		},
	})

	if notificationConsumer != nil {
		go func() {
			if err := notificationConsumer.RunContext(ctx); err != nil && ctx.Err() == nil {
				tel.Logger.Error("notification consumer stopped", slog.Any("error", err))
			}
		}()
	}

	srv := server.New(cfg, tel.Logger, telemetry.Middleware(tel, router))
	if err := srv.Run(ctx); err != nil {
		tel.Logger.Error("runtime error", slog.Any("error", err))
		return err
	}
	return nil
}
