package rides

// requestedProducer and acceptedProducer are the Kafka producer ports. The
// concrete *kafka.Producer types satisfy them; tests inject fakes.
type requestedProducer interface{ Publish(Requested) error }
type acceptedProducer interface{ Publish(Accepted) error }

// Publisher publishes rider events to Kafka.
type Publisher struct {
	requested requestedProducer
	accepted  acceptedProducer
}

// NewPublisher creates a Kafka publisher.
func NewPublisher(requested requestedProducer, accepted acceptedProducer) *Publisher {
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
