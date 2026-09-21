package matching

import (
	"context"
	"net/http"

	"github.com/guilhermelinosp/fast-platform-modular/internal/orders"
	"github.com/guilhermelinosp/hellnet-lib-api/errors"
	"github.com/guilhermelinosp/hellnet-lib-environments/environments"
	"github.com/guilhermelinosp/hellnet-lib-kafka/kafka"
	"github.com/guilhermelinosp/hellnet-lib-telemetry/telemetry"
)

// MatchService matches an order to an available driver.
type MatchService interface {
	Match(context.Context, string) (OfferOutput, error)
}

// NewConsumer builds a Kafka consumer that matches incoming ride requests.
func NewConsumer(ops telemetry.Client, service MatchService) (*kafka.Consumer[orders.OrderRequested], error) {
	if service == nil {
		return nil, errors.New(http.StatusInternalServerError, "INTERNAL", "matching: service is nil")
	}
	handler := kafka.HandlerFunc[orders.OrderRequested](func(ctx context.Context, event orders.OrderRequested, _ kafka.Ctx) error {
		return matchEvent(ctx, event, service)
	})
	return kafka.NewConsumer(handler, kafka.HandlerSpec{Group: environments.Get("HELLNET_KAFKA_MATCHING_CONSUMER_GROUP", "fast-matching")})
}

func matchEvent(ctx context.Context, event orders.OrderRequested, service MatchService) error {
	_, err := service.Match(ctx, event.OrderID)
	switch {
	case err == nil:
		return nil
	case isErrNoDriverAvailable(err):
		return nil // Info log handled by service
	case isErrRideAlreadyMatched(err):
		return nil // Info log handled by service
	default:
		return err
	}
}

func isErrNoDriverAvailable(err error) bool {
	return err != nil && err.Error() == "matching: no driver available"
}

func isErrRideAlreadyMatched(err error) bool {
	return err != nil && err.Error() == "matching: ride already matched"
}
