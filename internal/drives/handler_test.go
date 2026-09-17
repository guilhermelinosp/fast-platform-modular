package drives

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"testing"

	"uuid"

	"github.com/guilhermelinosp/hellnet-lib-api/api"
	apierrors "github.com/guilhermelinosp/hellnet-lib-api/errors"
)

type fakeService struct {
	calls int
	input AcceptedInput
	out   RideOutput
	err   error
}

func (f *fakeService) Accepted(_ context.Context, input AcceptedInput) (RideOutput, error) {
	f.calls++
	f.input = input
	return f.out, f.err
}

type fakeRequest struct {
	body   []byte
	header map[string]string
	params map[string]string
}

func (r *fakeRequest) Param(key string) string  { return r.params[key] }
func (r *fakeRequest) Query(key string) string  { return "" }
func (r *fakeRequest) Header(key string) string { return r.header[key] }
func (r *fakeRequest) Bind(dst any) error       { return api.BindJSON(bytes.NewReader(r.body), dst) }
func (r *fakeRequest) Raw() *http.Request       { return nil }

func TestHandlerAcceptSuccess(t *testing.T) {
	const rideID = "00000000-0000-0000-0000-000000000001"
	const driverID = "00000000-0000-0000-0000-000000000002"
	service := &fakeService{out: RideOutput{ID: rideID}}
	handler := NewHandler(service)
	req := &fakeRequest{
		header: map[string]string{"driver_id": driverID},
		params: map[string]string{"rideId": rideID},
	}

	response, err := handler.accept(context.Background(), req)
	if err != nil {
		t.Fatalf("accept() error = %v", err)
	}
	if response.Status != http.StatusCreated {
		t.Fatalf("status = %d, want 201", response.Status)
	}
	if got := response.Body.(RideOutput); got.ID != rideID {
		t.Fatalf("body = %+v, want ride with id %q", got, rideID)
	}

	input := service.input
	if service.calls != 1 || input.RideID != rideID || input.DriverID != driverID {
		t.Fatalf("service input = %+v (calls %d), want ride %q and driver %q", input, service.calls, rideID, driverID)
	}
	if _, err := uuid.Parse(input.AcceptanceID); err != nil {
		t.Errorf("AcceptanceID %q is not a UUID: %v", input.AcceptanceID, err)
	}
	if _, err := uuid.Parse(input.StatusHistoryID); err != nil {
		t.Errorf("StatusHistoryID %q is not a UUID: %v", input.StatusHistoryID, err)
	}
	if _, err := uuid.Parse(input.OutboxID); err != nil {
		t.Errorf("OutboxID %q is not a UUID: %v", input.OutboxID, err)
	}
}

func TestHandlerAcceptValidatesDriverID(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{name: "missing", value: ""},
		{name: "not a uuid", value: "driver-1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewHandler(&fakeService{})
			req := &fakeRequest{
				header: map[string]string{"driver_id": tt.value},
				params: map[string]string{"rideId": "00000000-0000-0000-0000-000000000001"},
			}

			_, err := handler.accept(context.Background(), req)
			if err == nil {
				t.Fatal("accept() error = nil, want validation")
			}
			var apiErr *apierrors.Error
			if !errors.As(err, &apiErr) {
				t.Fatalf("error type = %T, want *errors.Error", err)
			}
			if apiErr.Status != http.StatusBadRequest || apiErr.Code != "VALIDATION_ERROR" {
				t.Fatalf("error = %+v, want VALIDATION_ERROR", apiErr)
			}
		})
	}
}

func TestHandlerAcceptPropagatesServiceError(t *testing.T) {
	want := errors.New("service boom")
	handler := NewHandler(&fakeService{err: want})
	req := &fakeRequest{
		header: map[string]string{"driver_id": "00000000-0000-0000-0000-000000000002"},
		params: map[string]string{"rideId": "00000000-0000-0000-0000-000000000001"},
	}

	_, err := handler.accept(context.Background(), req)
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
}
