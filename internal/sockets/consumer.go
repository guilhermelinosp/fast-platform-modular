package sockets

import (
	"context"
	"net/http"

	"github.com/guilhermelinosp/fast-platform-modular/internal/orders"
	"github.com/guilhermelinosp/hellnet-lib-api/errors"
	"github.com/guilhermelinosp/hellnet-lib-environments/environments"
	"github.com/guilhermelinosp/hellnet-lib-kafka/kafka"
	"github.com/guilhermelinosp/hellnet-lib-telemetry/telemetry"
)

var (
	requestedHandlerSpec = kafka.HandlerSpec{Group: environments.Get("HELLNET_KAFKA_SOCKETIO_DRIVER_NOTIFICATIONS_GROUP")}
	acceptedHandlerSpec  = kafka.HandlerSpec{Group: environments.Get("HELLNET_KAFKA_SOCKETIO_ORDER_NOTIFICATIONS_GROUP")}
)

// RequestedEmitter publishes requested riders to connected driver clients.
type RequestedEmitter interface {
	EmitRequested(orders.OrderRequested) error
}

// AcceptedEmitter publishes accepted riders to the subscribed rider client.
type AcceptedEmitter interface {
	EmitAccepted(orders.OrderAccepted) error
}

// NewOrderRequestConsumer consumes the order-requested topic and emits each event to the
// driver Socket.IO namespace.
func NewOrderRequestConsumer(ops telemetry.Client, emitter RequestedEmitter) (*kafka.Consumer[orders.OrderRequested], error) {
	if emitter == nil {
		return nil, errors.New(http.StatusInternalServerError, "INTERNAL", "sockets: requested emitter is nil")
	}
	var handler kafka.HandlerFunc[orders.OrderRequested] = func(ctx context.Context, event orders.OrderRequested, _ kafka.Ctx) error {
		ops.Info("kafka.consume.order_requested", "order_id", event.OrderID, "event_id", event.EventID)
		if c, err := ops.Metric().Counter("socket.kafka.consume.order_requested.total"); err == nil {
			c.Add(ctx, 1)
		}
		return emitter.EmitRequested(event)
	}
	return kafka.NewConsumer(handler, requestedHandlerSpec)
}

// NewOrderAcceptedConsumer consumes the order-accepted topic and emits each event to
// its Socket.IO order room.
func NewOrderAcceptedConsumer(ops telemetry.Client, emitter AcceptedEmitter) (*kafka.Consumer[orders.OrderAccepted], error) {
	if emitter == nil {
		return nil, errors.New(http.StatusInternalServerError, "INTERNAL", "sockets: accepted emitter is nil")
	}
	var handler kafka.HandlerFunc[orders.OrderAccepted] = func(ctx context.Context, event orders.OrderAccepted, _ kafka.Ctx) error {
		ops.Info("kafka.consume.order_accepted", "order_id", event.OrderID, "driver_id", event.DriverID, "event_id", event.EventID)
		if c, err := ops.Metric().Counter("socket.kafka.consume.order_accepted.total"); err == nil {
			c.Add(ctx, 1)
		}
		return emitter.EmitAccepted(event)
	}
	return kafka.NewConsumer(handler, acceptedHandlerSpec)
}
