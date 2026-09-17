package sockets

import (
	"context"
	"fmt"

	"github.com/guilhermelinosp/fast-platform-modular/internal/rides"
	"github.com/guilhermelinosp/hellnet-lib-kafka/kafka"
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

// RequestedEmitter publishes requested rides to connected driver clients.
type RequestedEmitter interface{ EmitRequested(rides.Requested) error }

// AcceptedEmitter publishes accepted rides to the subscribed rider client.
type AcceptedEmitter interface{ EmitAccepted(rides.Accepted) error }

// NewConsumer consumes the ride-requested topic and emits each event to the
// driver Socket.IO namespace.
func NewConsumer(emitter RequestedEmitter) (*kafka.Consumer[rides.Requested], error) {
	if emitter == nil {
		return nil, fmt.Errorf("sockets: requested emitter is nil")
	}
	var handler kafka.HandlerFunc[rides.Requested] = func(ctx context.Context, event rides.Requested, _ kafka.Ctx) error {
		return emitter.EmitRequested(event)
	}
	return kafka.NewConsumer(handler, kafka.HandlerSpec{Group: ConsumerGroup()})
}

// NewAcceptedConsumer consumes the ride-accepted topic and emits each event to
// its Socket.IO ride room.
func NewAcceptedConsumer(emitter AcceptedEmitter) (*kafka.Consumer[rides.Accepted], error) {
	if emitter == nil {
		return nil, fmt.Errorf("sockets: accepted emitter is nil")
	}
	var handler kafka.HandlerFunc[rides.Accepted] = func(ctx context.Context, event rides.Accepted, _ kafka.Ctx) error {
		return emitter.EmitAccepted(event)
	}
	return kafka.NewConsumer(handler, kafka.HandlerSpec{Group: AcceptedConsumerGroup()})
}
