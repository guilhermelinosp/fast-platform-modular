package matching

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/guilhermelinosp/fast-platform-modular/internal/rides"
	"github.com/guilhermelinosp/hellnet-lib-kafka/kafka"
)

const (
	consumerGroupEnv     = "HELLNET_KAFKA_MATCHING_CONSUMER_GROUP"
	defaultConsumerGroup = "fast-matching"
)

// ConsumerGroup returns the dedicated group used by matching. It must not
// reuse the driver BFF group, otherwise the two consumers would compete for
// the same requested-ride records.
func ConsumerGroup() string {
	if group := os.Getenv(consumerGroupEnv); group != "" {
		return group
	}
	return defaultConsumerGroup
}

// MatchService is the matching consumer port.
type MatchService interface {
	Match(context.Context, string) (OfferOutput, error)
}

// NewConsumer consumes the ride-requested topic and matches each ride. The
// no-driver and already-matched outcomes are normal skips: the consumer logs
// and continues instead of failing the record.
func NewConsumer(service MatchService) (*kafka.Consumer[rides.Requested], error) {
	if service == nil {
		return nil, fmt.Errorf("matching: service is nil")
	}
	handler := kafka.HandlerFunc[rides.Requested](func(ctx context.Context, event rides.Requested, _ kafka.Ctx) error {
		return matchEvent(ctx, service, event)
	})
	return kafka.NewConsumer(handler, kafka.HandlerSpec{Group: ConsumerGroup()})
}

// matchEvent is the consumer's single-event decision, extracted so tests can
// drive it without a Kafka broker.
func matchEvent(ctx context.Context, service MatchService, event rides.Requested) error {
	_, err := service.Match(ctx, event.RideID)
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrNoDriverAvailable):
		slog.Default().Info("matching: no driver available, skipping ride",
			"ride_id", event.RideID, "event_id", event.EventID)
		return nil
	case errors.Is(err, ErrRideAlreadyMatched):
		slog.Default().Info("matching: ride already matched, skipping",
			"ride_id", event.RideID, "event_id", event.EventID)
		return nil
	default:
		return err
	}
}
