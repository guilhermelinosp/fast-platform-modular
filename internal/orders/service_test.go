package orders

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"uuid"

	apierrors "github.com/guilhermelinosp/hellnet-lib-api/errors"
)

type fakeRepository struct {
	calls int
	input OrderRequestedInput
	order Order
	err   error
}

func (f *fakeRepository) Requested(_ context.Context, input OrderRequestedInput) (Order, error) {
	f.calls++
	f.input = input
	return f.order, f.err
}

func TestServiceRequestedValidatesRequiredFields(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		riderID string
	}{
		{name: "empty id", id: "", riderID: "00000000-0000-0000-0000-000000000001"},
		{name: "whitespace id", id: "   ", riderID: "00000000-0000-0000-0000-000000000001"},
		{name: "empty rider id", id: "00000000-0000-0000-0000-000000000001", riderID: ""},
		{name: "whitespace rider id", id: "00000000-0000-0000-0000-000000000001", riderID: "\t"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeRepository{}
			service := NewService(nil, repo)

			_, err := service.Requested(context.Background(), OrderRequestedInput{ID: tt.id, RiderID: tt.riderID})
			assertValidation(t, err)
			if repo.calls != 0 {
				t.Fatalf("repository calls = %d, want 0", repo.calls)
			}
		})
	}
}

func TestServiceRequestedBuildsOutboxPayload(t *testing.T) {
	input := OrderRequestedInput{
		ID:                   "00000000-0000-0000-0000-000000000001",
		RiderID:              "00000000-0000-0000-0000-000000000002",
		PickupLatitude:       -23.56,
		PickupLongitude:      -46.65,
		DestinationLatitude:  -23.55,
		DestinationLongitude: -46.64,
	}
	repo := &fakeRepository{order: Order(inputAsOrder(input))}
	service := NewService(nil, repo)

	out, err := service.Requested(context.Background(), input)
	if err != nil {
		t.Fatalf("Requested() error = %v", err)
	}
	if repo.calls != 1 {
		t.Fatalf("repository calls = %d, want 1", repo.calls)
	}

	got := repo.input
	if got.ID != input.ID || got.RiderID != input.RiderID {
		t.Fatalf("input id/rider = (%q, %q), want (%q, %q)", got.ID, got.RiderID, input.ID, input.RiderID)
	}
	if got.StatusHistoryID == "" || got.OutboxID == "" {
		t.Fatalf("generated ids empty: status=%q outbox=%q", got.StatusHistoryID, got.OutboxID)
	}
	if _, err := uuid.Parse(got.StatusHistoryID); err != nil {
		t.Errorf("StatusHistoryID %q is not a UUID: %v", got.StatusHistoryID, err)
	}
	if _, err := uuid.Parse(got.OutboxID); err != nil {
		t.Errorf("OutboxID %q is not a UUID: %v", got.OutboxID, err)
	}
	if got.EventType != (OrderRequested{}).MessageType() {
		t.Errorf("EventType = %q, want %q", got.EventType, (OrderRequested{}).MessageType())
	}

	var payload OrderRequested
	if err := json.Unmarshal(got.Payload, &payload); err != nil {
		t.Fatalf("payload is not valid requested JSON: %v", err)
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
	if payload.OrderID != input.ID || payload.RiderID != input.RiderID {
		t.Errorf("payload ride = (%q, %q), want (%q, %q)", payload.OrderID, payload.RiderID, input.ID, input.RiderID)
	}
	if payload.PickupLatitude != input.PickupLatitude || payload.PickupLongitude != input.PickupLongitude ||
		payload.DestinationLatitude != input.DestinationLatitude || payload.DestinationLongitude != input.DestinationLongitude {
		t.Errorf("payload coordinates mismatch: %+v", payload)
	}

	if out.ID != input.ID || out.RiderID != input.RiderID {
		t.Errorf("output = %+v, want ride fields mapped", out)
	}
}

func TestServiceRequestedPreservesProvidedIDs(t *testing.T) {
	input := OrderRequestedInput{
		ID:              "00000000-0000-0000-0000-000000000001",
		RiderID:         "00000000-0000-0000-0000-000000000002",
		StatusHistoryID: "00000000-0000-0000-0000-000000000003",
		OutboxID:        "00000000-0000-0000-0000-000000000004",
	}
	repo := &fakeRepository{}
	service := NewService(nil, repo)

	if _, err := service.Requested(context.Background(), input); err != nil {
		t.Fatalf("Requested() error = %v", err)
	}
	if repo.input.StatusHistoryID != input.StatusHistoryID || repo.input.OutboxID != input.OutboxID {
		t.Fatalf("provided ids overwritten: status=%q outbox=%q", repo.input.StatusHistoryID, repo.input.OutboxID)
	}
}

func TestServiceRequestedPropagatesRepositoryError(t *testing.T) {
	want := errors.New("repository boom")
	repo := &fakeRepository{err: want}
	service := NewService(nil, repo)

	_, err := service.Requested(context.Background(), OrderRequestedInput{ID: "id-1", RiderID: "rider-1"})
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
}

func TestServiceNewServiceDefaultsTelemetry(t *testing.T) {
	service := NewService(nil, &fakeRepository{})
	if service.tel == nil {
		t.Fatal("tel = nil, want telemetry client")
	}
}

func assertValidation(t *testing.T, err error) {
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

func inputAsOrder(input OrderRequestedInput) Order {
	return Order{
		ID: input.ID, RiderID: input.RiderID,
		PickupLatitude: input.PickupLatitude, PickupLongitude: input.PickupLongitude,
		DestinationLatitude: input.DestinationLatitude, DestinationLongitude: input.DestinationLongitude,
	}
}
