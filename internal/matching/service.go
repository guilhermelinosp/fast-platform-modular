package matching

import (
	"context"
	"log/slog"

	"uuid"

	apierrors "github.com/guilhermelinosp/hellnet-lib-api/errors"
)

// Repository is the matching persistence port.
type Repository interface {
	Match(context.Context, string) (Offer, error)
}

// Service matches rides to available drivers.
type Service struct {
	logger     *slog.Logger
	repository Repository
}

// NewService creates a matching service.
func NewService(logger *slog.Logger, repository Repository) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{logger: logger, repository: repository}
}

// Match assigns the next available driver to a ride. It rejects non-UUID ride
// ids and propagates repository outcomes unchanged so consumers can decide
// how to handle no-driver or already-matched cases.
func (s *Service) Match(ctx context.Context, rideID string) (OfferOutput, error) {
	if _, err := uuid.Parse(rideID); err != nil {
		return OfferOutput{}, apierrors.Validation("ride_id", "must be a UUID")
	}
	offer, err := s.repository.Match(ctx, rideID)
	if err != nil {
		return OfferOutput{}, err
	}
	return OfferOutput(offer), nil
}
