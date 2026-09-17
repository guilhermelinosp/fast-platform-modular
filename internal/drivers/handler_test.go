package drivers

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/guilhermelinosp/hellnet-lib-api/api"
	apierrors "github.com/guilhermelinosp/hellnet-lib-api/errors"
)

type fakeService struct {
	calls int
	input AvailabilityInput
	out   DriverOutput
	err   error
}

func (f *fakeService) SetAvailability(_ context.Context, input AvailabilityInput) (DriverOutput, error) {
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

func TestHandlerSetAvailabilitySuccess(t *testing.T) {
	const driverID = "00000000-0000-0000-0000-000000000002"
	service := &fakeService{out: DriverOutput{ID: driverID, Available: true}}
	handler := NewHandler(service)
	req := &fakeRequest{
		body:   []byte(`{"available":true}`),
		params: map[string]string{"driverId": driverID},
	}

	response, err := handler.setAvailability(context.Background(), req)
	if err != nil {
		t.Fatalf("setAvailability() error = %v", err)
	}
	if response.Status != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Status)
	}
	if got := response.Body.(DriverOutput); got.ID != driverID || got.Available != true {
		t.Fatalf("body = %+v, want driver %q available", got, driverID)
	}
	if service.calls != 1 || service.input.DriverID != driverID || service.input.Available != true {
		t.Fatalf("service input = %+v (calls %d), want driver %q available", service.input, service.calls, driverID)
	}
}

func TestHandlerSetAvailabilityUnavailable(t *testing.T) {
	const driverID = "00000000-0000-0000-0000-000000000002"
	service := &fakeService{out: DriverOutput{ID: driverID, Available: false}}
	handler := NewHandler(service)
	req := &fakeRequest{
		body:   []byte(`{"available":false}`),
		params: map[string]string{"driverId": driverID},
	}

	response, err := handler.setAvailability(context.Background(), req)
	if err != nil {
		t.Fatalf("setAvailability() error = %v", err)
	}
	if response.Status != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Status)
	}
	if service.input.Available != false {
		t.Fatalf("service input.Available = %v, want false", service.input.Available)
	}
}

func TestHandlerSetAvailabilityValidatesDriverID(t *testing.T) {
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
				body:   []byte(`{"available":true}`),
				params: map[string]string{"driverId": tt.value},
			}

			_, err := handler.setAvailability(context.Background(), req)
			if err == nil {
				t.Fatal("setAvailability() error = nil, want validation")
			}
			var apiErr *apierrors.Error
			if !errors.As(err, &apiErr) {
				t.Fatalf("error type = %T, want *errors.Error", err)
			}
			if apiErr.Status != http.StatusBadRequest {
				t.Fatalf("error = %+v, want 400", apiErr)
			}
		})
	}
}

func TestHandlerSetAvailabilityRequiresField(t *testing.T) {
	handler := NewHandler(&fakeService{})
	req := &fakeRequest{
		body:   []byte(`{}`),
		params: map[string]string{"driverId": "00000000-0000-0000-0000-000000000002"},
	}

	_, err := handler.setAvailability(context.Background(), req)
	if err == nil {
		t.Fatal("setAvailability() error = nil, want validation for missing available")
	}
	var apiErr *apierrors.Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("error type = %T, want *errors.Error", err)
	}
	if apiErr.Status != http.StatusBadRequest {
		t.Fatalf("error = %+v, want 400", apiErr)
	}
}

func TestHandlerSetAvailabilityPropagatesServiceError(t *testing.T) {
	want := errors.New("service boom")
	handler := NewHandler(&fakeService{err: want})
	req := &fakeRequest{
		body:   []byte(`{"available":true}`),
		params: map[string]string{"driverId": "00000000-0000-0000-0000-000000000002"},
	}

	_, err := handler.setAvailability(context.Background(), req)
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
}
