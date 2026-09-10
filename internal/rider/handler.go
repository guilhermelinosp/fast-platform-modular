package rider

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"github.com/guilhermelinosp/hellnet-lib-api/api"
	apierrors "github.com/guilhermelinosp/hellnet-lib-api/errors"
)

// Handler exposes the rider HTTP routes.
type Handler struct {
	service interface {
		Requested(context.Context, RequestedInput) (RideOutput, error)
		Accepted(context.Context, AcceptedInput) (RideOutput, error)
	}
}

type acceptInput struct {
	DriverID string `json:"driver_id"`
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
	Requested(context.Context, RequestedInput) (RideOutput, error)
	Accepted(context.Context, AcceptedInput) (RideOutput, error)
}) *Handler {
	return &Handler{service: service}
}

// Routes returns the rider routes.
func (h *Handler) Routes() []api.Route {
	return []api.Route{
		{Method: http.MethodPost, Path: "/rides", Handler: api.HandlerFunc(h.request)},
		{Method: http.MethodPost, Path: "/rides/{rideId}/accept", Handler: api.HandlerFunc(h.accept)},
	}
}

func (h *Handler) accept(ctx context.Context, req api.Request) (api.Response, error) {
	var in acceptInput
	if err := req.Bind(&in); err != nil {
		return api.Response{}, err
	}
	if _, err := uuid.Parse(in.DriverID); err != nil {
		return api.Response{}, apierrors.Validation("driver_id", "must be a UUID")
	}
	input := AcceptedInput{
		RideID:          req.Param("rideId"),
		DriverID:        in.DriverID,
		AcceptanceID:    uuid.NewString(),
		StatusHistoryID: uuid.NewString(),
		OutboxID:        uuid.NewString(),
	}
	ride, err := h.service.Accepted(ctx, input)
	if err != nil {
		return api.Response{}, err
	}
	return api.JSON(http.StatusCreated, ride), nil
}

func (h *Handler) request(ctx context.Context, req api.Request) (api.Response, error) {
	var in requestInput
	if err := req.Bind(&in); err != nil {
		return api.Response{}, err
	}
	if in.ID == "" {
		in.ID = uuid.NewString()
	}
	if in.RiderID == "" {
		in.RiderID = uuid.NewString()
	}
	if _, err := uuid.Parse(in.ID); err != nil {
		return api.Response{}, apierrors.Validation("id", "must be a UUID")
	}
	if _, err := uuid.Parse(in.RiderID); err != nil {
		return api.Response{}, apierrors.Validation("rider_id", "must be a UUID")
	}
	payload, _ := json.Marshal(in)
	input := RequestedInput{
		ID:                   in.ID,
		RiderID:              in.RiderID,
		PickupLatitude:       in.PickupLatitude,
		PickupLongitude:      in.PickupLongitude,
		DestinationLatitude:  in.DestinationLatitude,
		DestinationLongitude: in.DestinationLongitude,
		Payload:              payload,
	}
	ride, err := h.service.Requested(ctx, input)
	if err != nil {
		return api.Response{}, err
	}
	return api.JSON(http.StatusCreated, ride), nil
}
