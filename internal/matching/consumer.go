package matching

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/guilhermelinosp/fast-platform-modular/internal/orders"
	"github.com/guilhermelinosp/hellnet-lib-kafka/kafka"
)

const (
	consumerGroupEnv     = "HELLNET_KAFKA_MATCHING_CONSUMER_GROUP"
	defaultConsumerGroup = "fast-matching"
)

func ConsumerGroup() string {
	if group := os.Getenv(consumerGroupEnv); group != "" {
		return group
	}
	return defaultConsumerGroup
}

type MatchService interface {
	Match(context.Context, string) (OfferOutput, error)
}

func NewConsumer(service MatchService) (*kafka.Consumer[orders.OrderRequested], error) {
	if service == nil {
		return nil, fmt.Errorf("matching: service is nil")
	}
	handler := kafka.HandlerFunc[orders.OrderRequested](func(ctx context.Context, event orders.OrderRequested, _ kafka.Ctx) error {
		return matchEvent(ctx, service, event)
	})
	return kafka.NewConsumer(handler, kafka.HandlerSpec{Group: ConsumerGroup()})
}

func matchEvent(ctx context.Context, service MatchService, event orders.OrderRequested) error {
	_, err := service.Match(ctx, event.OrderID)
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrNoDriverAvailable):
		slog.Default().Info("matching: no driver available, skipping order",
			"order_id", event.OrderID, "event_id", event.EventID)
		return nil
	case errors.Is(err, ErrRideAlreadyMatched):
		slog.Default().Info("matching: order already matched, skipping",
			"order_id", event.OrderID, "event_id", event.EventID)
		return nil
	default:
		return err
	}
}
