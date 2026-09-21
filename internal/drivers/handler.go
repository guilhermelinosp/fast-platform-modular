package drivers

import (
	"context"
	"net/http"

	"uuid"

	"github.com/guilhermelinosp/hellnet-lib-api/api"
	"github.com/guilhermelinosp/hellnet-lib-api/errors"
)

// Handler exposes driver HTTP routes.
type Handler struct {
	service interface {
		SetAvailability(context.Context, AvailabilityInput) (DriverOutput, error)
		Accepted(context.Context, AcceptedInput) (OrderOutput, error)
	}
}

// NewHandler creates a driver HTTP handler.
func NewHandler(service interface {
	SetAvailability(context.Context, AvailabilityInput) (DriverOutput, error)
	Accepted(context.Context, AcceptedInput) (OrderOutput, error)
}) *Handler {
	return &Handler{service: service}
}

// Routes returns driver routes.
func (h *Handler) Routes() []api.Route {
	return []api.Route{
		// {Method: http.MethodPut, Path: "/drivers/{driverId}/availability", Handler: api.HandlerFunc(h.setAvailability)},
		{Method: http.MethodPost, Path: "/orders/{orderId}/accept", Handler: api.HandlerFunc(h.accept)},
	}
}

// setAvailability handles PUT /drivers/{driverId}/availability
func (h *Handler) setAvailability(ctx context.Context, req api.Request) (api.Response, error) {
	driverID := req.Param("driverId")
	if _, err := uuid.Parse(driverID); err != nil {
		return api.Response{}, errors.Validation("driver_id", "must be a UUID")
	}
	var body struct {
		Available *bool `json:"available"`
	}
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

// accept handles POST /orders/{orderId}/accept
func (h *Handler) accept(ctx context.Context, req api.Request) (api.Response, error) {
	driverID := req.Header("driver_id")
	if _, err := uuid.Parse(driverID); err != nil {
		return api.Response{}, errors.Validation("driver_id", "must be a UUID")
	}

	order, err := h.service.Accepted(ctx, AcceptedInput{
		OrderID:         req.Param("orderId"),
		DriverID:        driverID,
		AcceptanceID:    uuid.New().String(),
		StatusHistoryID: uuid.New().String(),
		OutboxID:        uuid.New().String(),
	})
	if err != nil {
		return api.Response{}, err
	}
	return api.JSON(http.StatusCreated, order), nil
}
