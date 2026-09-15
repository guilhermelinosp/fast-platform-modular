package drives

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"uuid"

	"github.com/guilhermelinosp/fast-platform-modular/internal/rides"
	"github.com/guilhermelinosp/hellnet-lib-api/errors"
	"github.com/jackc/pgx/v5/pgconn"
)

// Service implements driver use cases.
type Service struct {
	logger     *slog.Logger
	repository interface {
		Accepted(context.Context, AcceptedInput) (Ride, error)
	}
}

// NewService creates a driver service.
func NewService(logger *slog.Logger, repository interface {
	Accepted(context.Context, AcceptedInput) (Ride, error)
}) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{logger: logger, repository: repository}
}

// Accepted handles the ride acceptance use case.
func (s *Service) Accepted(ctx context.Context, input AcceptedInput) (RideOutput, error) {
	if _, err := uuid.Parse(input.RideID); err != nil {
		return RideOutput{}, errors.Validation("ride_id", "must be a UUID")
	}
	if _, err := uuid.Parse(input.DriverID); err != nil {
		return RideOutput{}, errors.Validation("driver_id", "must be a UUID")
	}
	input.Payload, _ = json.Marshal(rides.Accepted{EventID: input.OutboxID, EventVersion: 1, OccurredAt: time.Now().UnixMilli(), RideID: input.RideID, DriverID: input.DriverID})
	input.EventType = (rides.Accepted{}).MessageType()
	ride, err := s.repository.Accepted(ctx, input)
	if err != nil {
		pgErr, isPgError := err.(*pgconn.PgError)
		if isPgError && pgErr.Code == "23505" && pgErr.ConstraintName == "uq_ride_acceptances_ride" {
			return RideOutput{}, errors.New(http.StatusConflict, "RIDE_ALREADY_ACCEPTED", "ride has already been accepted")
		}
		return RideOutput{}, err
	}
	return RideOutput(ride), nil
}
