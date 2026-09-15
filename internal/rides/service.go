package rides

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"time"

	"uuid"

	"github.com/guilhermelinosp/hellnet-lib-api/errors"
)

// Service implements rider use cases.
type Service struct {
	logger     *slog.Logger
	repository interface {
		Requested(context.Context, RequestedInput) (Ride, error)
	}
}

// NewService creates the rider service.
func NewService(logger *slog.Logger, repository interface {
	Requested(context.Context, RequestedInput) (Ride, error)
}) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{logger: logger, repository: repository}
}

// Requested handles the ride request use case.
func (s *Service) Requested(ctx context.Context, input RequestedInput) (RideOutput, error) {
	if strings.TrimSpace(input.ID) == "" || strings.TrimSpace(input.RiderID) == "" {
		return RideOutput{}, errors.Validation("ride", "id and rider_id are required")
	}

	if input.StatusHistoryID == "" {
		input.StatusHistoryID = uuid.New().String()
	}

	if input.OutboxID == "" {
		input.OutboxID = uuid.New().String()
	}

	input.Payload, _ = json.Marshal(Requested{
		EventID:      input.OutboxID,
		EventVersion: 1, OccurredAt: time.Now().UnixMilli(),
		RideID:               input.ID,
		RiderID:              input.RiderID,
		PickupLatitude:       input.PickupLatitude,
		PickupLongitude:      input.PickupLongitude,
		DestinationLatitude:  input.DestinationLatitude,
		DestinationLongitude: input.DestinationLongitude,
	})
	input.EventType = (Requested{}).MessageType()

	ride, err := s.repository.Requested(ctx, input)
	if err != nil {
		return RideOutput{}, err
	}

	return RideOutput(ride), nil
}
