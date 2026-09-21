package outbox

import (
	"encoding/json"

	"github.com/guilhermelinosp/fast-platform-modular/internal/orders"
	"github.com/guilhermelinosp/hellnet-lib-api/errors"
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
			return errors.Wrap(errors.New(500, "OUTBOX_DECODE", "decode order requested event"), err)
		}
		return p.requested.Publish(message)
	case (orders.OrderAccepted{}).MessageType():
		var message orders.OrderAccepted
		if err := json.Unmarshal(event.Payload, &message); err != nil {
			return errors.Wrap(errors.New(500, "OUTBOX_DECODE", "decode order accepted event"), err)
		}
		return p.accepted.Publish(message)
	default:
		return errors.New(500, "UNSUPPORTED_EVENT", "unsupported outbox event type "+event.EventType)
	}
}
