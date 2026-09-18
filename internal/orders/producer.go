package orders

// requestedProducer and acceptedProducer are the Kafka producer ports. The
// concrete *kafka.Producer types satisfy them; tests inject fakes.
type requestedProducer interface{ Publish(OrderRequested) error }
type acceptedProducer interface{ Publish(OrderAccepted) error }

// Publisher publishes rider events to Kafka.
type Publisher struct {
	requested requestedProducer
	accepted  acceptedProducer
}

// NewPublisher creates a Kafka publisher.
func NewPublisher(requested requestedProducer, accepted acceptedProducer) *Publisher {
	return &Publisher{requested: requested, accepted: accepted}
}

// OrderRequested publishes an order requested event.
func (p *Publisher) OrderRequested(event OrderRequested) error {
	return p.requested.Publish(event)
}

// OrderAccepted publishes an order accepted event.
func (p *Publisher) OrderAccepted(event OrderAccepted) error {
	return p.accepted.Publish(event)
}
