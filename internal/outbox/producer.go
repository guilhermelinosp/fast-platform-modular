package outbox

import (
	"encoding/json"
	"fmt"

	"github.com/guilhermelinosp/fast-platform-modular/internal/orders"
	"github.com/guilhermelinosp/hellnet-lib-kafka/kafka"
)

// requestedPublisher and acceptedPublisher are the Kafka producer ports.
type requestedPublisher interface {
	Publish(orders.OrderRequested) error
}
type acceptedPublisher interface {
	Publish(orders.OrderAccepted) error
}

// Producer handles Kafka publishing of outbox events.
type Producer struct {
	requested requestedPublisher
	accepted  acceptedPublisher
}

// NewProducer creates a producer that publishes outbox events to Kafka.
func NewProducer(requested *kafka.Producer[orders.OrderRequested], accepted *kafka.Producer[orders.OrderAccepted]) *Producer {
	return &Producer{
		requested: requested,
		accepted:  accepted,
	}
}

// Publish decodes and publishes a single outbox event to Kafka.
func (p *Producer) Publish(event Event) error {
	switch event.EventType {
	case (orders.OrderRequested{}).MessageType():
		var message orders.OrderRequested
		if err := json.Unmarshal(event.Payload, &message); err != nil {
			return fmt.Errorf("decode order requested event: %w", err)
		}
		return p.requested.Publish(message)
	case (orders.OrderAccepted{}).MessageType():
		var message orders.OrderAccepted
		if err := json.Unmarshal(event.Payload, &message); err != nil {
			return fmt.Errorf("decode order accepted event: %w", err)
		}
		return p.accepted.Publish(message)
	default:
		return fmt.Errorf("unsupported outbox event type %q", event.EventType)
	}
}
