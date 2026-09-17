package outbox

import (
	"encoding/json"
	"fmt"

	"github.com/guilhermelinosp/fast-platform-modular/internal/rides"
	"github.com/guilhermelinosp/hellnet-lib-kafka/kafka"
)

// requestedPublisher and acceptedPublisher are the Kafka producer ports.
type requestedPublisher interface{ Publish(rides.Requested) error }
type acceptedPublisher interface{ Publish(rides.Accepted) error }

// Producer handles Kafka publishing of outbox events.
type Producer struct {
	requested requestedPublisher
	accepted  acceptedPublisher
}

// NewProducer creates a producer that publishes outbox events to Kafka.
func NewProducer(requested *kafka.Producer[rides.Requested], accepted *kafka.Producer[rides.Accepted]) *Producer {
	return &Producer{
		requested: requested,
		accepted:  accepted,
	}
}

// Publish decodes and publishes a single outbox event to Kafka.
func (p *Producer) Publish(event Event) error {
	switch event.EventType {
	case (rides.Requested{}).MessageType():
		var message rides.Requested
		if err := json.Unmarshal(event.Payload, &message); err != nil {
			return fmt.Errorf("decode requested event: %w", err)
		}
		return p.requested.Publish(message)
	case (rides.Accepted{}).MessageType():
		var message rides.Accepted
		if err := json.Unmarshal(event.Payload, &message); err != nil {
			return fmt.Errorf("decode accepted event: %w", err)
		}
		return p.accepted.Publish(message)
	default:
		return fmt.Errorf("unsupported outbox event type %q", event.EventType)
	}
}
