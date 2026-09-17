package rides

import (
	"errors"
	"testing"
)

type fakeRequestedProducer struct {
	calls int
	last  Requested
	err   error
}

func (f *fakeRequestedProducer) Publish(m Requested) error {
	f.calls++
	f.last = m
	return f.err
}

type fakeAcceptedProducer struct {
	calls int
	last  Accepted
	err   error
}

func (f *fakeAcceptedProducer) Publish(m Accepted) error {
	f.calls++
	f.last = m
	return f.err
}

func TestPublisherRequestedPublishes(t *testing.T) {
	requested := &fakeRequestedProducer{}
	p := NewPublisher(requested, &fakeAcceptedProducer{})

	event := Requested{EventID: "event-1", RideID: "ride-1"}
	if err := p.Requested(event); err != nil {
		t.Fatalf("Requested() = %v, want nil", err)
	}

	if got := requested.calls; got != 1 {
		t.Fatalf("publish calls = %d, want 1", got)
	}
	if got := requested.last.EventID; got != "event-1" {
		t.Fatalf("published EventID = %q, want event-1", got)
	}
	if got := requested.last.RideID; got != "ride-1" {
		t.Fatalf("published RideID = %q, want ride-1", got)
	}
}

func TestPublisherRequestedPropagatesError(t *testing.T) {
	requested := &fakeRequestedProducer{err: errors.New("kafka down")}
	p := NewPublisher(requested, &fakeAcceptedProducer{})

	err := p.Requested(Requested{EventID: "event-1"})
	if err == nil || err.Error() != "kafka down" {
		t.Fatalf("Requested() = %v, want kafka down error", err)
	}
}

func TestPublisherAcceptedPublishes(t *testing.T) {
	accepted := &fakeAcceptedProducer{}
	p := NewPublisher(&fakeRequestedProducer{}, accepted)

	event := Accepted{EventID: "event-1", RideID: "ride-1", DriverID: "driver-1"}
	if err := p.Accepted(event); err != nil {
		t.Fatalf("Accepted() = %v, want nil", err)
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

func TestPublisherAcceptedPropagatesError(t *testing.T) {
	accepted := &fakeAcceptedProducer{err: errors.New("kafka down")}
	p := NewPublisher(&fakeRequestedProducer{}, accepted)

	err := p.Accepted(Accepted{EventID: "event-1"})
	if err == nil || err.Error() != "kafka down" {
		t.Fatalf("Accepted() = %v, want kafka down error", err)
	}
}
