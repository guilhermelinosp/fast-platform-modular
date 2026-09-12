package rides

import "github.com/guilhermelinosp/hellnet-lib-kafka/kafka"

// Publisher publishes rider events to Kafka.
type Publisher struct {
	requested *kafka.Producer[Requested]
	accepted  *kafka.Producer[Accepted]
}

// NewPublisher creates a Kafka publisher.
func NewPublisher(requested *kafka.Producer[Requested], accepted *kafka.Producer[Accepted]) *Publisher {
	return &Publisher{requested: requested, accepted: accepted}
}

// Requested publishes a ride requested event.
func (p *Publisher) Requested(event Requested) error {
	return p.requested.Publish(event)
}

// Accepted publishes a ride accepted event.
func (p *Publisher) Accepted(event Accepted) error {
	return p.accepted.Publish(event)
}
