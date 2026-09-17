package drivers

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"uuid"

	apierrors "github.com/guilhermelinosp/hellnet-lib-api/errors"
)

// fakeRepository implements the service repository port.
type fakeRepository struct {
	driver Driver
	err    error
}

func (f *fakeRepository) SetAvailability(context.Context, AvailabilityInput) (Driver, error) {
	if f.err != nil {
		return Driver{}, f.err
	}
	return f.driver, nil
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

func TestNewServiceDefaultsLogger(t *testing.T) {
	service := NewService(nil, &fakeRepository{})
	if service.logger == nil {
		t.Fatal("logger = nil, want default")
	}
}

func TestParseUUIDBuildsValidInstanceID(t *testing.T) {
	id := uuid.New().String()
	if _, err := uuid.Parse(id); err != nil {
		t.Fatalf("generated UUID %q is not valid: %v", id, err)
	}
}
