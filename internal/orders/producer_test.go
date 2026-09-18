package orders

import (
	"errors"
	"testing"
)

type fakeOrderRequestedProducer struct {
	calls int
	last  OrderRequested
	err   error
}

func (f *fakeOrderRequestedProducer) Publish(m OrderRequested) error {
	f.calls++
	f.last = m
	return f.err
}

type fakeOrderAcceptedProducer struct {
	calls int
	last  OrderAccepted
	err   error
}

func (f *fakeOrderAcceptedProducer) Publish(m OrderAccepted) error {
	f.calls++
	f.last = m
	return f.err
}

func TestPublisherOrderRequestedPublishes(t *testing.T) {
	requested := &fakeOrderRequestedProducer{}
	p := NewPublisher(requested, &fakeOrderAcceptedProducer{})

	event := OrderRequested{EventID: "event-1", OrderID: "order-1"}
	if err := p.OrderRequested(event); err != nil {
		t.Fatalf("OrderRequested() = %v, want nil", err)
	}

	if got := requested.calls; got != 1 {
		t.Fatalf("publish calls = %d, want 1", got)
	}
	if got := requested.last.EventID; got != "event-1" {
		t.Fatalf("published EventID = %q, want event-1", got)
	}
	if got := requested.last.OrderID; got != "order-1" {
		t.Fatalf("published OrderID = %q, want order-1", got)
	}
}

func TestPublisherOrderRequestedPropagatesError(t *testing.T) {
	requested := &fakeOrderRequestedProducer{err: errors.New("kafka down")}
	p := NewPublisher(requested, &fakeOrderAcceptedProducer{})

	err := p.OrderRequested(OrderRequested{EventID: "event-1"})
	if err == nil || err.Error() != "kafka down" {
		t.Fatalf("OrderRequested() = %v, want kafka down error", err)
	}
}

func TestPublisherOrderAcceptedPublishes(t *testing.T) {
	accepted := &fakeOrderAcceptedProducer{}
	p := NewPublisher(&fakeOrderRequestedProducer{}, accepted)

	event := OrderAccepted{EventID: "event-1", OrderID: "order-1", DriverID: "driver-1"}
	if err := p.OrderAccepted(event); err != nil {
		t.Fatalf("OrderAccepted() = %v, want nil", err)
	}

	if got := accepted.calls; got != 1 {
		t.Fatalf("publish calls = %d, want 1", got)
	}
	if got := accepted.last.EventID; got != "event-1" {
		t.Fatalf("published EventID = %q, want event-1", got)
	}
	if got := accepted.last.DriverID; got != "driver-1" {
		t.Fatalf("published DriverID = %q, want driver-1", got)
	}
}

func TestPublisherOrderAcceptedPropagatesError(t *testing.T) {
	accepted := &fakeOrderAcceptedProducer{err: errors.New("kafka down")}
	p := NewPublisher(&fakeOrderRequestedProducer{}, accepted)

	err := p.OrderAccepted(OrderAccepted{EventID: "event-1"})
	if err == nil || err.Error() != "kafka down" {
		t.Fatalf("OrderAccepted() = %v, want kafka down error", err)
	}
}
