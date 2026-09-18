package drivers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"uuid"

	"github.com/guilhermelinosp/fast-platform-modular/internal/orders"
	apierrors "github.com/guilhermelinosp/hellnet-lib-api/errors"
	"github.com/jackc/pgx/v5/pgconn"
)

// fakeRepository implements the service repository port.
type fakeRepository struct {
	driver     Driver
	order      Order
	err        error
	acceptIn   AcceptedInput
	setAvailIn AvailabilityInput
	calls      int
}

func (f *fakeRepository) SetAvailability(context.Context, AvailabilityInput) (Driver, error) {
	f.calls++
	f.setAvailIn = AvailabilityInput{}
	if f.err != nil {
		return Driver{}, f.err
	}
	return f.driver, nil
}

func (f *fakeRepository) Accepted(_ context.Context, input AcceptedInput) (Order, error) {
	f.calls++
	f.acceptIn = input
	if f.err != nil {
		return Order{}, f.err
	}
	return f.order, nil
}

func TestServiceSetAvailability(t *testing.T) {
	repo := &fakeRepository{driver: Driver{ID: "00000000-0000-0000-0000-000000000001", Available: true}}
	service := NewService(nil, repo)

	out, err := service.SetAvailability(context.Background(), AvailabilityInput{DriverID: "00000000-0000-0000-0000-000000000001", Available: true})
	if err != nil {
		t.Fatalf("SetAvailability() error = %v", err)
	}
	if out.ID != repo.driver.ID || out.Available != true {
		t.Fatalf("output = %+v, want %+v", out, repo.driver)
	}
}

func TestServiceSetAvailabilityRejectsNonUUID(t *testing.T) {
	service := NewService(nil, &fakeRepository{})

	_, err := service.SetAvailability(context.Background(), AvailabilityInput{DriverID: "not-a-uuid", Available: true})
	if err == nil {
		t.Fatal("SetAvailability() error = nil, want validation error")
	}
	var apiErr *apierrors.Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("error type = %T, want *errors.Error", err)
	}
	if apiErr.Status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", apiErr.Status)
	}
}

func TestServiceSetAvailabilityPropagatesError(t *testing.T) {
	wantErr := errors.New("db down")
	service := NewService(nil, &fakeRepository{err: wantErr})

	_, err := service.SetAvailability(context.Background(), AvailabilityInput{DriverID: "00000000-0000-0000-0000-000000000001", Available: true})
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
}

func TestServiceAcceptedValidatesUUIDs(t *testing.T) {
	const orderID = "00000000-0000-0000-0000-000000000001"
	const driverID = "00000000-0000-0000-0000-000000000002"

	tests := []struct {
		name          string
		orderID       string
		driverID      string
		wantRepoCalls int
	}{
		{name: "invalid order id", orderID: "not-a-uuid", driverID: driverID, wantRepoCalls: 0},
		{name: "empty order id", orderID: "", driverID: driverID, wantRepoCalls: 0},
		{name: "invalid driver id", orderID: orderID, driverID: "not-a-uuid", wantRepoCalls: 0},
		{name: "empty driver id", orderID: orderID, driverID: "", wantRepoCalls: 0},
		{name: "valid UUIDs", orderID: "00000000-0000-0000-0000-000000000001", driverID: "00000000-0000-0000-0000-000000000002", wantRepoCalls: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeRepository{}
			service := NewService(nil, repo)

			_, err := service.Accepted(context.Background(), AcceptedInput{OrderID: tt.orderID, DriverID: tt.driverID})
			if tt.wantRepoCalls == 0 {
				assertServiceValidation(t, err)
			} else if err != nil {
				t.Fatalf("Accepted() error = %v, want nil", err)
			}
			if repo.calls != tt.wantRepoCalls {
				t.Fatalf("repository calls = %d, want %d", repo.calls, tt.wantRepoCalls)
			}
			if tt.wantRepoCalls > 0 {
				if repo.acceptIn.OrderID != tt.orderID || repo.acceptIn.DriverID != tt.driverID {
					t.Fatalf("service input = %+v, want order %q driver %q", repo.acceptIn, tt.orderID, tt.driverID)
				}
			}
		})
	}
}

func TestServiceAcceptedBuildsOutboxPayload(t *testing.T) {
	input := AcceptedInput{
		OrderID:  "00000000-0000-0000-0000-000000000001",
		DriverID: "00000000-0000-0000-0000-000000000002",
		OutboxID: "00000000-0000-0000-0000-000000000003",
	}
	repo := &fakeRepository{order: Order{ID: input.OrderID}}
	service := NewService(nil, repo)

	out, err := service.Accepted(context.Background(), input)
	if err != nil {
		t.Fatalf("Accepted() error = %v", err)
	}
	if repo.calls != 1 {
		t.Fatalf("repository calls = %d, want 1", repo.calls)
	}

	got := repo.acceptIn
	if got.EventType != (orders.OrderAccepted{}).MessageType() {
		t.Errorf("EventType = %q, want %q", got.EventType, (orders.OrderAccepted{}).MessageType())
	}

	var payload orders.OrderAccepted
	if err := json.Unmarshal(got.Payload, &payload); err != nil {
		t.Fatalf("payload is not valid accepted JSON: %v", err)
	}
	if payload.EventID != got.OutboxID {
		t.Errorf("payload.EventID = %q, want outbox id %q", payload.EventID, got.OutboxID)
	}
	if payload.EventVersion != 1 {
		t.Errorf("payload.EventVersion = %d, want 1", payload.EventVersion)
	}
	if payload.OccurredAt <= 0 {
		t.Errorf("payload.OccurredAt = %d, want > 0", payload.OccurredAt)
	}
	if payload.OrderID != input.OrderID || payload.DriverID != input.DriverID {
		t.Errorf("payload order/driver = (%q, %q), want (%q, %q)", payload.OrderID, payload.DriverID, input.OrderID, input.DriverID)
	}

	if out.ID != input.OrderID {
		t.Errorf("output = %+v, want id %q", out, input.OrderID)
	}
}

func TestServiceAcceptedMapsConflictToAlreadyAccepted(t *testing.T) {
	repo := &fakeRepository{err: &pgconn.PgError{Code: "23505", ConstraintName: "uq_order_acceptances_order"}}
	service := NewService(nil, repo)

	_, err := service.Accepted(context.Background(), AcceptedInput{
		OrderID:  "00000000-0000-0000-0000-000000000001",
		DriverID: "00000000-0000-0000-0000-000000000002",
	})
	if err == nil {
		t.Fatal("Accepted() error = nil, want ORDER_ALREADY_ACCEPTED")
	}
	var apiErr *apierrors.Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("error type = %T, want *errors.Error", err)
	}
	if apiErr.Status != http.StatusConflict || apiErr.Code != "ORDER_ALREADY_ACCEPTED" {
		t.Fatalf("error = %+v, want status 409 ORDER_ALREADY_ACCEPTED", apiErr)
	}
}

func TestServiceAcceptedMapsDriverNotFound(t *testing.T) {
	repo := &fakeRepository{err: ErrDriverNotFound}
	service := NewService(nil, repo)

	_, err := service.Accepted(context.Background(), AcceptedInput{
		OrderID:  "00000000-0000-0000-0000-000000000001",
		DriverID: "00000000-0000-0000-0000-000000000002",
	})
	if err == nil {
		t.Fatal("Accepted() error = nil, want DRIVER_NOT_FOUND")
	}
	var apiErr *apierrors.Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("error type = %T, want *errors.Error", err)
	}
	if apiErr.Status != http.StatusNotFound || apiErr.Code != "DRIVER_NOT_FOUND" {
		t.Fatalf("error = %+v, want status 404 DRIVER_NOT_FOUND", apiErr)
	}
}

func TestServiceAcceptedMapsOrderNotAcceptable(t *testing.T) {
	repo := &fakeRepository{err: ErrOrderNotAcceptable}
	service := NewService(nil, repo)

	_, err := service.Accepted(context.Background(), AcceptedInput{
		OrderID:  "00000000-0000-0000-0000-000000000001",
		DriverID: "00000000-0000-0000-0000-000000000002",
	})
	if err == nil {
		t.Fatal("Accepted() error = nil, want ORDER_NOT_ACCEPTABLE")
	}
	var apiErr *apierrors.Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("error type = %T, want *errors.Error", err)
	}
	if apiErr.Status != http.StatusConflict || apiErr.Code != "ORDER_NOT_ACCEPTABLE" {
		t.Fatalf("error = %+v, want status 409 ORDER_NOT_ACCEPTABLE", apiErr)
	}
}

func TestServiceAcceptedPropagatesOtherErrors(t *testing.T) {
	t.Run("different unique constraint", func(t *testing.T) {
		pgErr := &pgconn.PgError{Code: "23505", ConstraintName: "orders_pkey"}
		repo := &fakeRepository{err: pgErr}
		service := NewService(nil, repo)

		_, err := service.Accepted(context.Background(), AcceptedInput{
			OrderID:  "00000000-0000-0000-0000-000000000001",
			DriverID: "00000000-0000-0000-0000-000000000002",
		})
		if !errors.Is(err, pgErr) {
			t.Fatalf("error = %v, want %v propagated unchanged", err, pgErr)
		}
	})

	t.Run("non-pg error", func(t *testing.T) {
		want := errors.New("repository boom")
		repo := &fakeRepository{err: want}
		service := NewService(nil, repo)

		_, err := service.Accepted(context.Background(), AcceptedInput{
			OrderID:  "00000000-0000-0000-0000-000000000001",
			DriverID: "00000000-0000-0000-0000-000000000002",
		})
		if !errors.Is(err, want) {
			t.Fatalf("error = %v, want %v", err, want)
		}
	})
}

func TestNewServiceDefaultsTelemetry(t *testing.T) {
	service := NewService(nil, &fakeRepository{})
	if service.tel == nil {
		t.Fatal("tel = nil, want telemetry client")
	}
}

func TestParseUUIDBuildsValidInstanceID(t *testing.T) {
	id := uuid.New().String()
	if _, err := uuid.Parse(id); err != nil {
		t.Fatalf("generated UUID %q is not valid: %v", id, err)
	}
}

func assertServiceValidation(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("error = nil, want validation error")
	}
	var apiErr *apierrors.Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("error type = %T, want *errors.Error", err)
	}
	if apiErr.Status != http.StatusBadRequest || apiErr.Code != "VALIDATION_ERROR" {
		t.Fatalf("error = %+v, want status 400 VALIDATION_ERROR", apiErr)
	}
}
