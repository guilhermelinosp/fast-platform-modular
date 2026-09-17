package drivers

import (
	"context"
	"net/http"

	"uuid"

	"github.com/guilhermelinosp/hellnet-lib-api/api"
	"github.com/guilhermelinosp/hellnet-lib-api/errors"
)

// availabilityRequest is the accepted payload for availability updates.
type availabilityRequest struct {
	Available *bool `json:"available"`
}

// Handler exposes driver HTTP routes.
type Handler struct {
	service interface {
		SetAvailability(context.Context, AvailabilityInput) (DriverOutput, error)
	}
}

// NewHandler creates a driver HTTP handler.
func NewHandler(service interface {
	SetAvailability(context.Context, AvailabilityInput) (DriverOutput, error)
}) *Handler {
	return &Handler{service: service}
}

// Routes returns driver routes.
func (h *Handler) Routes() []api.Route {
	return []api.Route{
		{Method: http.MethodPut, Path: "/drivers/{driverId}/availability", Handler: api.HandlerFunc(h.setAvailability)},
	}
}

func (h *Handler) setAvailability(ctx context.Context, req api.Request) (api.Response, error) {
	driverID := req.Param("driverId")
	if _, err := uuid.Parse(driverID); err != nil {
		return api.Response{}, errors.Validation("driver_id", "must be a UUID")
	}
	var body availabilityRequest
	if err := req.Bind(&body); err != nil {
		return api.Response{}, err
	}
	if body.Available == nil {
		return api.Response{}, errors.Validation("available", "is required")
	}
	driver, err := h.service.SetAvailability(ctx, AvailabilityInput{DriverID: driverID, Available: *body.Available})
	if err != nil {
		return api.Response{}, err
	}
	return api.JSON(http.StatusOK, driver), nil
}
