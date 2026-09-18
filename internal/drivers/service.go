package drivers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/guilhermelinosp/fast-platform-modular/internal/orders"
	apierrors "github.com/guilhermelinosp/hellnet-lib-api/errors"
	"github.com/guilhermelinosp/hellnet-lib-telemetry/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
)

// Service implements driver use cases.
type Service struct {
	tel        telemetry.Client
	repository interface {
		SetAvailability(context.Context, AvailabilityInput) (Driver, error)
		Accepted(context.Context, AcceptedInput) (Order, error)
	}

	// Metrics
	setAvailabilityTotal    metric.Int64Counter
	setAvailabilityDuration metric.Int64Histogram
	acceptedTotal           metric.Int64Counter
	acceptedDuration        metric.Int64Histogram
}

// NewService creates a driver service.
func NewService(tel telemetry.Client, repository interface {
	SetAvailability(context.Context, AvailabilityInput) (Driver, error)
	Accepted(context.Context, AcceptedInput) (Order, error)
}) *Service {
	if tel == nil {
		tel, _ = telemetry.New()
	}
	s := &Service{tel: tel, repository: repository}

	// Initialize metrics
	if tel != nil && tel.Metric() != nil {
		s.setAvailabilityTotal, _ = tel.Metric().Counter("drivers.set_availability.total")
		s.setAvailabilityDuration, _ = tel.Metric().Histogram("drivers.set_availability.duration_seconds")
		s.acceptedTotal, _ = tel.Metric().Counter("drivers.accepted.total")
		s.acceptedDuration, _ = tel.Metric().Histogram("drivers.accepted.duration_seconds")
	}

	return s
}

// SetAvailability updates a driver's availability flag.
func (s *Service) SetAvailability(ctx context.Context, input AvailabilityInput) (DriverOutput, error) {
	var result DriverOutput
	var err error

	if s.tel != nil {
		err = s.tel.Worker("drivers.set_availability", func(ctx context.Context) error {
			result, err = s.doSetAvailability(ctx, input)
			return err
		}, attribute.String("driver_id", input.DriverID))
	} else {
		result, err = s.doSetAvailability(ctx, input)
	}

	return result, err
}

func (s *Service) doSetAvailability(ctx context.Context, input AvailabilityInput) (DriverOutput, error) {
	start := time.Now()
	status := "success"
	defer func() {
		if s.setAvailabilityTotal != nil {
			s.setAvailabilityTotal.Add(ctx, 1, metric.WithAttributes(attribute.String("status", status)))
		}
		if s.setAvailabilityDuration != nil {
			s.setAvailabilityDuration.Record(ctx, time.Since(start).Milliseconds(), metric.WithAttributes(attribute.String("status", status)))
		}
	}()

	if _, err := uuid.Parse(input.DriverID); err != nil {
		status = "validation_error"
		return DriverOutput{}, apierrors.Validation("driver_id", "must be a UUID")
	}
	driver, err := s.repository.SetAvailability(ctx, input)
	if err != nil {
		status = "error"
		return DriverOutput{}, err
	}

	s.tel.Log().Info("driver availability updated", "driver_id", input.DriverID, "available", input.Available)
	return DriverOutput(driver), nil
}

// Accepted handles the order acceptance use case.
func (s *Service) Accepted(ctx context.Context, input AcceptedInput) (OrderOutput, error) {
	var result OrderOutput
	var err error

	if s.tel != nil {
		err = s.tel.Worker("drivers.accepted", func(ctx context.Context) error {
			result, err = s.doAccepted(ctx, input)
			return err
		}, attribute.String("driver_id", input.DriverID), attribute.String("order_id", input.OrderID))
	} else {
		result, err = s.doAccepted(ctx, input)
	}

	return result, err
}

func (s *Service) doAccepted(ctx context.Context, input AcceptedInput) (OrderOutput, error) {
	start := time.Now()
	status := "success"
	defer func() {
		if s.acceptedTotal != nil {
			s.acceptedTotal.Add(ctx, 1, metric.WithAttributes(attribute.String("status", status)))
		}
		if s.acceptedDuration != nil {
			s.acceptedDuration.Record(ctx, time.Since(start).Milliseconds(), metric.WithAttributes(attribute.String("status", status)))
		}
	}()

	if _, err := uuid.Parse(input.OrderID); err != nil {
		status = "validation_error"
		return OrderOutput{}, apierrors.Validation("order_id", "must be a UUID")
	}
	if _, err := uuid.Parse(input.DriverID); err != nil {
		status = "validation_error"
		return OrderOutput{}, apierrors.Validation("driver_id", "must be a UUID")
	}
	input.Payload, _ = json.Marshal(orders.OrderAccepted{EventID: input.OutboxID, EventVersion: 1, OccurredAt: time.Now().UnixMilli(), OrderID: input.OrderID, DriverID: input.DriverID})
	input.EventType = (orders.OrderAccepted{}).MessageType()
	order, err := s.repository.Accepted(ctx, input)
	if err != nil {
		status = "error"
		if errors.Is(err, ErrDriverNotFound) {
			return OrderOutput{}, apierrors.New(http.StatusNotFound, "DRIVER_NOT_FOUND", "driver does not exist")
		}
		if errors.Is(err, ErrOrderNotAcceptable) {
			return OrderOutput{}, apierrors.New(http.StatusConflict, "ORDER_NOT_ACCEPTABLE", "order is not in the requested state")
		}
		pgErr, isPgError := err.(*pgconn.PgError)
		if isPgError && pgErr.Code == "23505" && pgErr.ConstraintName == "uq_order_acceptances_order" {
			return OrderOutput{}, apierrors.New(http.StatusConflict, "ORDER_ALREADY_ACCEPTED", "order has already been accepted")
		}
		return OrderOutput{}, err
	}

	s.tel.Log().Info("order accepted", "order_id", input.OrderID, "driver_id", input.DriverID)
	return OrderOutput(order), nil
}
