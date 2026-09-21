package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/guilhermelinosp/fast-platform-modular/internal/drivers"
	"github.com/guilhermelinosp/fast-platform-modular/internal/matching"
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
		os.Exit(1)
	}
}

func run() error {
	// 1. Application context — owns the HTTP server lifecycle and shutdown.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg, err := config.New()
	if err != nil {
		return err
	}
	ops, err := telemetry.New()
	if err != nil {
		return err
	}
	defer func() { _ = ops.Shutdown() }()

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
	listener, err := outbox.NewListener(ops, db, producer)
	if err != nil {
		return err
	}
	defer listener.Close()

	riderService := orders.NewService(ops, orders.NewRepository(db))
	driverService := drivers.NewService(ops, drivers.NewRepository(db))
	matchingService := matching.NewService(matching.NewRepository(db))

	orderHandler := orders.NewHandler(riderService)
	driverHandler := drivers.NewHandler(driverService)

	router := adapter.New(cfg, ops)

	api.RegisterPlatform(router, api.Deps{
		Routes: append(orderHandler.Routes(), driverHandler.Routes()...),
		Platform: api.PlatformHandlers{
			Live:   ops.Live(),
			Ready:  ops.Ready(),
			Health: ops.Health(),
		},
	})

	// Mount Socket.IO handler at /socket.io/ BEFORE telemetry middleware
	// (needs http.Hijacker for WebSocket upgrades)
	socket := sockets.NewServer(ops)
	mux := http.NewServeMux()
	mux.Handle("/socket.io/", socket.Handler())
	// Prometheus metrics endpoint
	mux.Handle("GET /metrics", ops.MetricsHandler())
	// telemetry.Middleware adds OpenTelemetry metrics (request count, duration, inflight, etc.)
	mux.Handle("/", telemetry.Middleware(ops, router))

	orderRequestConsumer, err := sockets.NewOrderRequestConsumer(ops, socket)
	if err != nil {
		return err
	}
	defer func() {
		if err := orderRequestConsumer.Close(); err != nil {
			ops.Warn("order request consumer close failed", "error", err)
		}
	}()

	orderAcceptedConsumer, err := sockets.NewOrderAcceptedConsumer(ops, socket)
	if err != nil {
		return err
	}
	defer func() {
		if err := orderAcceptedConsumer.Close(); err != nil {
			ops.Warn("order accepted consumer close failed", "error", err)
		}
	}()

	matchingConsumer, err := matching.NewConsumer(ops, matchingService)
	if err != nil {
		return err
	}
	defer func() {
		if err := matchingConsumer.Close(); err != nil {
			ops.Warn("matching consumer close failed", "error", err)
		}
	}()

	// Start Kafka consumers with Worker for correlated spans/metrics.
	go func() {
		_ = ops.Worker("matching.consume.order_requested", func(ctx context.Context) error {
			return matchingConsumer.RunContext(ctx)
		}, attribute.String("consumer", "matching"))
	}()

	go func() {
		_ = ops.Worker("socket.consume.order_requested", func(ctx context.Context) error {
			return orderRequestConsumer.RunContext(ctx)
		}, attribute.String("consumer", "order-requested"))
	}()

	go func() {
		_ = ops.Worker("socket.consume.order_accepted", func(ctx context.Context) error {
			return orderAcceptedConsumer.RunContext(ctx)
		}, attribute.String("consumer", "order-accepted"))
	}()

	if err := server.New(cfg, ops, mux).Run(ctx); err != nil {
		ops.Error("runtime error", "error", err)
		return err
	}
	return nil
}
