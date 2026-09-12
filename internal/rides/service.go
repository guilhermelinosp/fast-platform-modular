package rides

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"uuid"

	apierrors "github.com/guilhermelinosp/hellnet-lib-api/errors"
)

// Service implements rider use cases.
type Service struct {
	logger     *slog.Logger
	repository interface {
		Requested(context.Context, RequestedInput) (Ride, error)
		Accepted(context.Context, AcceptedInput) (Ride, error)
	}
	publisher interface {
		Requested(Requested) error
		Accepted(Accepted) error
	}
}

// NewService creates the rider service.
func NewService(logger *slog.Logger, repository interface {
	Requested(context.Context, RequestedInput) (Ride, error)
	Accepted(context.Context, AcceptedInput) (Ride, error)
}, publisher interface {
	Requested(Requested) error
	Accepted(Accepted) error
}) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{logger: logger, repository: repository, publisher: publisher}
}

// Requested handles the ride request use case.
func (s *Service) Requested(ctx context.Context, input RequestedInput) (RideOutput, error) {
	if strings.TrimSpace(input.ID) == "" || strings.TrimSpace(input.RiderID) == "" {
		return RideOutput{}, apierrors.Validation("ride", "id and rider_id are required")
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

	ride, err := s.repository.Requested(ctx, input)
	if err != nil {
		return RideOutput{}, err
	}

	if s.publisher != nil {
		if err := s.publisher.Requested(Requested{
			EventID:              input.OutboxID,
			EventVersion:         1,
			OccurredAt:           time.Now().UnixMilli(),
			RideID:               ride.ID,
			RiderID:              ride.RiderID,
			PickupLatitude:       ride.PickupLatitude,
			PickupLongitude:      ride.PickupLongitude,
			DestinationLatitude:  ride.DestinationLatitude,
			DestinationLongitude: ride.DestinationLongitude,
		}); err != nil {
			return RideOutput{}, fmt.Errorf("publish requested: %w", err)
		}
	}
	return RideOutput(ride), nil
}

// Accepted handles the ride acceptance use case.
func (s *Service) Accepted(ctx context.Context, input AcceptedInput) (RideOutput, error) {
	if _, err := uuid.Parse(input.RideID); err != nil {
		return RideOutput{}, apierrors.Validation("ride_id", "must be a UUID")
	}
	if _, err := uuid.Parse(input.DriverID); err != nil {
		return RideOutput{}, apierrors.Validation("driver_id", "must be a UUID")
	}

	input.Payload, _ = json.Marshal(Accepted{
		EventID:      input.OutboxID,
		EventVersion: 1,
		OccurredAt:   time.Now().UnixMilli(),
		RideID:       input.RideID,
		DriverID:     input.DriverID,
	})

	ride, err := s.repository.Accepted(ctx, input)

	if err != nil {
		return RideOutput{}, err
	}

	if s.publisher != nil {
		if err := s.publisher.Accepted(Accepted{
			EventID:      input.OutboxID,
			EventVersion: 1,
			OccurredAt:   time.Now().UnixMilli(),
			RideID:       ride.ID,
			DriverID:     input.DriverID,
		}); err != nil {
			return RideOutput{}, fmt.Errorf("publish accepted: %w", err)
		}
	}

	return RideOutput{ID: ride.ID}, nil
}
