package matching

import (
	"context"

	"uuid"

	"github.com/guilhermelinosp/hellnet-lib-api/errors"
)

// Repository is the matching persistence port.
type Repository interface {
	Match(context.Context, string) (Offer, error)
}

// Service matches rides to available drivers.
type Service struct {
	repository Repository
}

// NewService creates a matching service.
func NewService(repository Repository) *Service {
	return &Service{repository: repository}
}

// Match assigns the next available driver to an order. It rejects non-UUID order
// ids and propagates repository outcomes unchanged so consumers can decide
// how to handle no-driver or already-matched cases.
func (s *Service) Match(ctx context.Context, orderID string) (OfferOutput, error) {
	if _, err := uuid.Parse(orderID); err != nil {
		return OfferOutput{}, errors.Validation("order_id", "must be a UUID")
	}
	offer, err := s.repository.Match(ctx, orderID)
	if err != nil {
		return OfferOutput{}, err
	}
	return OfferOutput(offer), nil
}
