package webhooks

import (
	"context"
	"fmt"

	"github.com/guilhermelinosp/fast-platform-modular/internal/rides"
	"github.com/guilhermelinosp/hellnet-lib-kafka/kafka"
)

const (
	consumerGroupEnv     = "HELLNET_KAFKA_DRIVER_BFF_CONSUMER_GROUP"
	defaultConsumerGroup = "fast-driver-bff"
)

// ConsumerGroup returns the dedicated group used by the driver BFF. It must
// not reuse the matching consumer group, otherwise offers would compete with
// matching for the same ride-requested records.
func ConsumerGroup() string {
	if group := envString(consumerGroupEnv); group != "" {
		return group
	}
	return defaultConsumerGroup
}

// NotificationSink receives a requested ride for outbound delivery.
type NotificationSink interface {
	Notify(context.Context, rides.Requested, string) error
}

// NewConsumer consumes the exact topic resolved by rides.Requested.MessageType
// and the Kafka library's configured topic prefix. No ride topic is renamed by
// the BFF. Each Kafka event is passed to the notification sink for webhook or
// push delivery.
func NewConsumer(sink NotificationSink) (*kafka.Consumer[rides.Requested], error) {
	if sink == nil {
		return nil, fmt.Errorf("driverbff: notification sink is nil")
	}
	var handler kafka.HandlerFunc[rides.Requested] = func(ctx context.Context, event rides.Requested, _ kafka.Ctx) error {
		return sink.Notify(ctx, event, "")
	}
	return kafka.NewConsumer(handler, kafka.HandlerSpec{Group: ConsumerGroup()})
}
