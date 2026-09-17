package matching

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"uuid"

	apierrors "github.com/guilhermelinosp/hellnet-lib-api/errors"
)

type fakeRepository struct {
	offer Offer
	err   error
}

func (f *fakeRepository) Match(context.Context, string) (Offer, error) {
	if f.err != nil {
		return Offer{}, f.err
	}
	return f.offer, nil
}

func TestServiceMatch(t *testing.T) {
	repo := &fakeRepository{offer: Offer{RideID: "00000000-0000-0000-0000-000000000001", DriverID: "00000000-0000-0000-0000-000000000002", Status: "pending"}}
	service := NewService(nil, repo)

	out, err := service.Match(context.Background(), "00000000-0000-0000-0000-000000000001")
	if err != nil {
		t.Fatalf("Match() error = %v", err)
	}
	if out.RideID != repo.offer.RideID || out.DriverID != repo.offer.DriverID || out.Status != "pending" {
		t.Fatalf("output = %+v, want %+v", out, repo.offer)
	}
}

func TestServiceMatchRejectsNonUUID(t *testing.T) {
	service := NewService(nil, &fakeRepository{})

	_, err := service.Match(context.Background(), "not-a-uuid")
	if err == nil {
		t.Fatal("Match() error = nil, want validation error")
	}
	var apiErr *apierrors.Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("error type = %T, want *errors.Error", err)
	}
	if apiErr.Status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", apiErr.Status)
	}
}

func TestServiceMatchPropagatesRepositoryError(t *testing.T) {
	wantErr := errors.New("db down")
	service := NewService(nil, &fakeRepository{err: wantErr})

	_, err := service.Match(context.Background(), "00000000-0000-0000-0000-000000000001")
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
}

func TestServiceMatchPropagatesNoDriver(t *testing.T) {
	service := NewService(nil, &fakeRepository{err: ErrNoDriverAvailable})

	_, err := service.Match(context.Background(), "00000000-0000-0000-0000-000000000001")
	if !errors.Is(err, ErrNoDriverAvailable) {
		t.Fatalf("error = %v, want ErrNoDriverAvailable", err)
	}
}

func TestNewServiceDefaultsLogger(t *testing.T) {
	service := NewService(nil, &fakeRepository{})
	if service.logger == nil {
		t.Fatal("logger = nil, want default")
	}
}

func TestParseUUIDBuildsValidMatchingUnitID(t *testing.T) {
	id := uuid.New().String()
	if _, err := uuid.Parse(id); err != nil {
		t.Fatalf("generated UUID %q is not valid: %v", id, err)
	}
}
