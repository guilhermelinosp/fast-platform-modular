package orders

import (
	"context"
	"net/http"

	"uuid"

	"github.com/guilhermelinosp/hellnet-lib-api/api"
	"github.com/guilhermelinosp/hellnet-lib-api/errors"
)

// Handler exposes the rider HTTP routes.
type Handler struct {
	service interface {
		Requested(context.Context, OrderRequestedInput) (OrderOutput, error)
	}
}

type requestInput struct {
	ID                   string  `json:"id"`
	RiderID              string  `json:"rider_id"`
	PickupLatitude       float64 `json:"pickup_latitude"`
	PickupLongitude      float64 `json:"pickup_longitude"`
	DestinationLatitude  float64 `json:"destination_latitude"`
	DestinationLongitude float64 `json:"destination_longitude"`
}

// NewHandler creates a rider HTTP handler.
func NewHandler(service interface {
	Requested(context.Context, OrderRequestedInput) (OrderOutput, error)
}) *Handler {
	return &Handler{service: service}
}

// Routes returns the rider routes.
func (h *Handler) Routes() []api.Route {
	return []api.Route{
		{Method: http.MethodPost, Path: "/orders", Handler: api.HandlerFunc(h.request)},
	}
}

func (h *Handler) request(ctx context.Context, req api.Request) (api.Response, error) {
	var in requestInput
	if err := req.Bind(&in); err != nil {
		return api.Response{}, err
	}
	if in.ID == "" {
		in.ID = uuid.New().String()
	}
	riderID := req.Header("rider_id")
	if riderID == "" {
		riderID = in.RiderID
	}
	if riderID == "" {
		riderID = uuid.New().String()
	}
	if _, err := uuid.Parse(in.ID); err != nil {
		return api.Response{}, errors.Validation("id", "must be a UUID")
	}
	if _, err := uuid.Parse(riderID); err != nil {
		return api.Response{}, errors.Validation("rider_id", "must be a UUID")
	}
	input := OrderRequestedInput{
		ID:                   in.ID,
		RiderID:              riderID,
		PickupLatitude:       in.PickupLatitude,
		PickupLongitude:      in.PickupLongitude,
		DestinationLatitude:  in.DestinationLatitude,
		DestinationLongitude: in.DestinationLongitude,
	}
	order, err := h.service.Requested(ctx, input)
	if err != nil {
		return api.Response{}, err
	}
	return api.JSON(http.StatusCreated, order), nil
}
