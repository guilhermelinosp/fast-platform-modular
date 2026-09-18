package sockets

import (
	"context"
	"fmt"

	"github.com/guilhermelinosp/fast-platform-modular/internal/orders"
	"github.com/guilhermelinosp/hellnet-lib-kafka/kafka"
	"github.com/guilhermelinosp/hellnet-lib-telemetry/telemetry"
)

const (
	consumerGroupEnv             = "HELLNET_KAFKA_DRIVER_BFF_CONSUMER_GROUP"
	defaultConsumerGroup         = "fast-driver-bff"
	acceptedConsumerGroupEnv     = "HELLNET_KAFKA_DRIVER_BFF_ACCEPTED_CONSUMER_GROUP"
	defaultAcceptedConsumerGroup = "fast-driver-bff-accepted"
)

// ConsumerGroup returns the dedicated group used by the driver BFF.
func ConsumerGroup() string {
	if group := envString(consumerGroupEnv); group != "" {
		return group
	}
	return defaultConsumerGroup
}

// AcceptedConsumerGroup returns the dedicated group used by the accepted BFF consumer.
func AcceptedConsumerGroup() string {
	if group := envString(acceptedConsumerGroupEnv); group != "" {
		return group
	}
	return defaultAcceptedConsumerGroup
}

// RequestedEmitter publishes requested orders to connected driver clients.
type RequestedEmitter interface {
	EmitRequested(orders.OrderRequested) error
}

// AcceptedEmitter publishes accepted orders to the subscribed rider client.
type AcceptedEmitter interface {
	EmitAccepted(orders.OrderAccepted) error
}

// NewOrderRequestConsumer consumes the order-requested topic and emits each event to the
// driver Socket.IO namespace.
func NewOrderRequestConsumer(tel telemetry.Client, emitter RequestedEmitter) (*kafka.Consumer[orders.OrderRequested], error) {
	if emitter == nil {
		return nil, fmt.Errorf("sockets: requested emitter is nil")
	}
	var handler kafka.HandlerFunc[orders.OrderRequested] = func(ctx context.Context, event orders.OrderRequested, _ kafka.Ctx) error {
		tel.Log().Info("kafka.consume.order_requested", "order_id", event.OrderID, "event_id", event.EventID)
		if c, err := tel.Metric().Counter("socket.kafka.consume.order_requested.total"); err == nil {
			c.Add(ctx, 1)
		}
		return emitter.EmitRequested(event)
	}
	return kafka.NewConsumer(handler, kafka.HandlerSpec{Group: ConsumerGroup()})
}

// NewOrderAcceptedConsumer consumes the order-accepted topic and emits each event to
// its Socket.IO order room.
func NewOrderAcceptedConsumer(tel telemetry.Client, emitter AcceptedEmitter) (*kafka.Consumer[orders.OrderAccepted], error) {
	if emitter == nil {
		return nil, fmt.Errorf("sockets: accepted emitter is nil")
	}
	var handler kafka.HandlerFunc[orders.OrderAccepted] = func(ctx context.Context, event orders.OrderAccepted, _ kafka.Ctx) error {
		tel.Log().Info("kafka.consume.order_accepted", "order_id", event.OrderID, "driver_id", event.DriverID, "event_id", event.EventID)
		if c, err := tel.Metric().Counter("socket.kafka.consume.order_accepted.total"); err == nil {
			c.Add(ctx, 1)
		}
		return emitter.EmitAccepted(event)
	}
	return kafka.NewConsumer(handler, kafka.HandlerSpec{Group: AcceptedConsumerGroup()})
}
