package drives

import (
	"context"
	"net/http"

	"uuid"

	"github.com/guilhermelinosp/hellnet-lib-api/api"
	"github.com/guilhermelinosp/hellnet-lib-api/errors"
)

// Handler exposes driver actions over rides.
type Handler struct {
	service interface {
		Accepted(context.Context, AcceptedInput) (RideOutput, error)
	}
}

// NewHandler creates a driver HTTP handler.
func NewHandler(service interface {
	Accepted(context.Context, AcceptedInput) (RideOutput, error)
}) *Handler {
	return &Handler{service: service}
}

// Routes returns driver routes.
func (h *Handler) Routes() []api.Route {
	return []api.Route{
		{Method: http.MethodPost, Path: "/drives/{rideId}/accept", Handler: api.HandlerFunc(h.accept)},
	}
}

func (h *Handler) accept(ctx context.Context, req api.Request) (api.Response, error) {
	driverID := req.Header("driver_id")
	if _, err := uuid.Parse(driverID); err != nil {
		return api.Response{}, errors.Validation("driver_id", "must be a UUID")
	}

	ride, err := h.service.Accepted(ctx, AcceptedInput{
		RideID:          req.Param("rideId"),
		DriverID:        driverID,
		AcceptanceID:    uuid.New().String(),
		StatusHistoryID: uuid.New().String(),
		OutboxID:        uuid.New().String(),
	})
	if err != nil {
		return api.Response{}, err
	}
	return api.JSON(http.StatusCreated, ride), nil
}
