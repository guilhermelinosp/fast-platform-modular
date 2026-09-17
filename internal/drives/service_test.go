package drives

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/guilhermelinosp/fast-platform-modular/internal/rides"
	apierrors "github.com/guilhermelinosp/hellnet-lib-api/errors"
	"github.com/jackc/pgx/v5/pgconn"
)

type fakeRepository struct {
	calls int
	input AcceptedInput
	ride  Ride
	err   error
}

func (f *fakeRepository) Accepted(_ context.Context, input AcceptedInput) (Ride, error) {
	f.calls++
	f.input = input
	return f.ride, f.err
}

func TestServiceAcceptedValidatesUUIDs(t *testing.T) {
	const rideID = "00000000-0000-0000-0000-000000000001"
	const driverID = "00000000-0000-0000-0000-000000000002"

	tests := []struct {
		name     string
		rideID   string
		driverID string
	}{
		{name: "invalid ride id", rideID: "not-a-uuid", driverID: driverID},
		{name: "empty ride id", rideID: "", driverID: driverID},
		{name: "invalid driver id", rideID: rideID, driverID: "not-a-uuid"},
		{name: "empty driver id", rideID: rideID, driverID: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeRepository{}
			service := NewService(nil, repo)

			_, err := service.Accepted(context.Background(), AcceptedInput{RideID: tt.rideID, DriverID: tt.driverID})
			assertServiceValidation(t, err)
			if repo.calls != 0 {
				t.Fatalf("repository calls = %d, want 0", repo.calls)
			}
		})
	}
}

func TestServiceAcceptedBuildsOutboxPayload(t *testing.T) {
	input := AcceptedInput{
		RideID:   "00000000-0000-0000-0000-000000000001",
		DriverID: "00000000-0000-0000-0000-000000000002",
		OutboxID: "00000000-0000-0000-0000-000000000003",
	}
	repo := &fakeRepository{ride: Ride{ID: input.RideID}}
	service := NewService(nil, repo)

	out, err := service.Accepted(context.Background(), input)
	if err != nil {
		t.Fatalf("Accepted() error = %v", err)
	}
	if repo.calls != 1 {
		t.Fatalf("repository calls = %d, want 1", repo.calls)
	}

	got := repo.input
	if got.EventType != (rides.Accepted{}).MessageType() {
		t.Errorf("EventType = %q, want %q", got.EventType, (rides.Accepted{}).MessageType())
	}

	var payload rides.Accepted
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
	if payload.RideID != input.RideID || payload.DriverID != input.DriverID {
		t.Errorf("payload ride/driver = (%q, %q), want (%q, %q)", payload.RideID, payload.DriverID, input.RideID, input.DriverID)
	}

	if out.ID != input.RideID {
		t.Errorf("output = %+v, want id %q", out, input.RideID)
	}
}

func TestServiceAcceptedMapsConflictToAlreadyAccepted(t *testing.T) {
	repo := &fakeRepository{err: &pgconn.PgError{Code: "23505", ConstraintName: "uq_ride_acceptances_ride"}}
	service := NewService(nil, repo)

	_, err := service.Accepted(context.Background(), AcceptedInput{
		RideID:   "00000000-0000-0000-0000-000000000001",
		DriverID: "00000000-0000-0000-0000-000000000002",
	})
	if err == nil {
		t.Fatal("Accepted() error = nil, want RIDE_ALREADY_ACCEPTED")
	}
	var apiErr *apierrors.Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("error type = %T, want *errors.Error", err)
	}
	if apiErr.Status != http.StatusConflict || apiErr.Code != "RIDE_ALREADY_ACCEPTED" {
		t.Fatalf("error = %+v, want status 409 RIDE_ALREADY_ACCEPTED", apiErr)
	}
}

func TestServiceAcceptedMapsDriverNotFound(t *testing.T) {
	repo := &fakeRepository{err: ErrDriverNotFound}
	service := NewService(nil, repo)

	_, err := service.Accepted(context.Background(), AcceptedInput{
		RideID:   "00000000-0000-0000-0000-000000000001",
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

func TestServiceAcceptedMapsRideNotAcceptable(t *testing.T) {
	repo := &fakeRepository{err: ErrRideNotAcceptable}
	service := NewService(nil, repo)

	_, err := service.Accepted(context.Background(), AcceptedInput{
		RideID:   "00000000-0000-0000-0000-000000000001",
		DriverID: "00000000-0000-0000-0000-000000000002",
	})
	if err == nil {
		t.Fatal("Accepted() error = nil, want RIDE_NOT_ACCEPTABLE")
	}
	var apiErr *apierrors.Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("error type = %T, want *errors.Error", err)
	}
	if apiErr.Status != http.StatusConflict || apiErr.Code != "RIDE_NOT_ACCEPTABLE" {
		t.Fatalf("error = %+v, want status 409 RIDE_NOT_ACCEPTABLE", apiErr)
	}
}

func TestServiceAcceptedPropagatesOtherErrors(t *testing.T) {
	t.Run("different unique constraint", func(t *testing.T) {
		pgErr := &pgconn.PgError{Code: "23505", ConstraintName: "rides_pkey"}
		repo := &fakeRepository{err: pgErr}
		service := NewService(nil, repo)

		_, err := service.Accepted(context.Background(), AcceptedInput{
			RideID:   "00000000-0000-0000-0000-000000000001",
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
			RideID:   "00000000-0000-0000-0000-000000000001",
			DriverID: "00000000-0000-0000-0000-000000000002",
		})
		if !errors.Is(err, want) {
			t.Fatalf("error = %v, want %v", err, want)
		}
	})
}

func TestServiceNewServiceDefaultsLogger(t *testing.T) {
	service := NewService(nil, &fakeRepository{})
	if service.logger == nil {
		t.Fatal("logger = nil, want slog.Default()")
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
