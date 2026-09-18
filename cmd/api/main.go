// Package main bootstraps the API.
//
// Deliberately tiny: it only wires explicit dependencies in lifecycle order
// and owns the shutdown sequence. Every real decision lives in packages:
//
//	context → config → telemetry → api adapter → http server → shutdown
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/guilhermelinosp/fast-platform-modular/internal/drivers"
	"github.com/guilhermelinosp/fast-platform-modular/internal/orders"
	"github.com/guilhermelinosp/fast-platform-modular/internal/outbox"
	"github.com/guilhermelinosp/fast-platform-modular/internal/sockets"
	"github.com/guilhermelinosp/hellnet-lib-api/adapter"
	"github.com/guilhermelinosp/hellnet-lib-api/api"
	"github.com/guilhermelinosp/hellnet-lib-api/config"
	"github.com/guilhermelinosp/hellnet-lib-api/server"

	"github.com/guilhermelinosp/hellnet-lib-database/database"
	"github.com/guilhermelinosp/hellnet-lib-kafka/kafka"
	"github.com/guilhermelinosp/hellnet-lib-telemetry/telemetry"
	"go.opentelemetry.io/otel/attribute"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "error", err)
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
	defer func() { _ = tel.Shutdown() }()

	db, err := database.New()
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()

	orderRequested, err := kafka.NewProducer[orders.OrderRequested]()
	if err != nil {
		return err
	}
	defer func() { _ = orderRequested.Close() }()

	orderAccepted, err := kafka.NewProducer[orders.OrderAccepted]()
	if err != nil {
		return err
	}
	defer func() { _ = orderAccepted.Close() }()

	producer := outbox.NewProducer(orderRequested, orderAccepted)
	listener, err := outbox.NewListener(db, producer, tel)
	if err != nil {
		return err
	}
	defer listener.Close()

	riderService := orders.NewService(tel, orders.NewRepository(db))
	driverService := drivers.NewService(tel, drivers.NewRepository(db))

	orderHandler := orders.NewHandler(riderService)
	driverHandler := drivers.NewHandler(driverService)

	router := adapter.New(cfg, tel.Logger)

	api.RegisterPlatform(router, api.Deps{
		Routes: append(orderHandler.Routes(), driverHandler.Routes()...),
		Platform: api.PlatformHandlers{
			Live:   tel.Live(),
			Ready:  tel.Ready(),
			Health: tel.Health(),
		},
	})

	// Mount Socket.IO handler at /socket.io/ BEFORE telemetry middleware
	// (needs http.Hijacker for WebSocket upgrades)
	socket := sockets.NewServer(tel)
	mux := http.NewServeMux()
	mux.Handle("/socket.io/", socket.Handler())
	// Prometheus metrics endpoint
	mux.Handle("GET /metrics", tel.MetricsHandler())
	// telemetry.Middleware adds OpenTelemetry metrics (request count, duration, inflight, etc.)
	mux.Handle("/", telemetry.Middleware(tel, router))

	orderRequestConsumer, err := sockets.NewOrderRequestConsumer(tel, socket)
	if err != nil {
		return err
	}
	defer func() {
		if err := orderRequestConsumer.Close(); err != nil {
			tel.Log().Warn("order request consumer close failed", "error", err)
		}
	}()

	orderAcceptedConsumer, err := sockets.NewOrderAcceptedConsumer(tel, socket)
	if err != nil {
		return err
	}
	defer func() {
		if err := orderAcceptedConsumer.Close(); err != nil {
			tel.Log().Warn("order accepted consumer close failed", "error", err)
		}
	}()

	// Start both consumers with Worker for correlated spans/metrics
	go tel.Worker("socket.consume.order_requested", func(ctx context.Context) error {
		return orderRequestConsumer.RunContext(ctx)
	}, attribute.String("consumer", "order-requested"))

	go tel.Worker("socket.consume.order_accepted", func(ctx context.Context) error {
		return orderAcceptedConsumer.RunContext(ctx)
	}, attribute.String("consumer", "order-accepted"))

	srv := server.New(cfg, tel.Logger, mux)
	if err := srv.Run(ctx); err != nil {
		tel.Log().Error("runtime error", "error", err)
		return err
	}
	return nil
}
