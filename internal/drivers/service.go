package drivers

import (
	"context"
	"log/slog"

	"uuid"

	apierrors "github.com/guilhermelinosp/hellnet-lib-api/errors"
)

// Service implements driver availability use cases.
type Service struct {
	logger     *slog.Logger
	repository interface {
		SetAvailability(context.Context, AvailabilityInput) (Driver, error)
	}
}

// NewService creates a driver availability service.
func NewService(logger *slog.Logger, repository interface {
	SetAvailability(context.Context, AvailabilityInput) (Driver, error)
}) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{logger: logger, repository: repository}
}

// SetAvailability updates a driver's availability flag.
func (s *Service) SetAvailability(ctx context.Context, input AvailabilityInput) (DriverOutput, error) {
	if _, err := uuid.Parse(input.DriverID); err != nil {
		return DriverOutput{}, apierrors.Validation("driver_id", "must be a UUID")
	}
	driver, err := s.repository.SetAvailability(ctx, input)
	if err != nil {
		return DriverOutput{}, err
	}
	return DriverOutput(driver), nil
}
