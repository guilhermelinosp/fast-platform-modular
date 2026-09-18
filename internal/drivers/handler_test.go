package drivers

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"uuid"

	"github.com/guilhermelinosp/hellnet-lib-api/api"
	apierrors "github.com/guilhermelinosp/hellnet-lib-api/errors"
)

type fakeService struct {
	calls       int
	setAvailIn  AvailabilityInput
	setAvailOut DriverOutput
	setAvailErr error
	acceptIn    AcceptedInput
	acceptOut   OrderOutput
	acceptErr   error
}

func (f *fakeService) SetAvailability(_ context.Context, input AvailabilityInput) (DriverOutput, error) {
	f.calls++
	f.setAvailIn = input
	return f.setAvailOut, f.setAvailErr
}

func (f *fakeService) Accepted(_ context.Context, input AcceptedInput) (OrderOutput, error) {
	f.calls++
	f.acceptIn = input
	return f.acceptOut, f.acceptErr
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
	const driverID = "00000000-0000-0000-0000-000000000001"
	service := &fakeService{setAvailOut: DriverOutput{ID: driverID, Available: true}}
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
		t.Fatalf("body = %+v, want driver with id %q and available true", got, driverID)
	}
	if service.setAvailIn.DriverID != driverID || service.setAvailIn.Available != true {
		t.Fatalf("service input = %+v, want driver %q available true", service.setAvailIn, driverID)
	}
}

func TestHandlerSetAvailabilityValidates(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		driver  string
		wantErr string
	}{
		{name: "missing available", body: `{}`, driver: "00000000-0000-0000-0000-000000000001", wantErr: "available"},
		{name: "invalid driver id", body: `{"available":true}`, driver: "not-a-uuid", wantErr: "driver_id"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewHandler(&fakeService{})
			req := &fakeRequest{
				body:   []byte(tt.body),
				params: map[string]string{"driverId": tt.driver},
			}
			_, err := handler.setAvailability(context.Background(), req)
			if err == nil {
				t.Fatal("setAvailability() error = nil, want validation")
			}
			var apiErr *apierrors.Error
			if !errors.As(err, &apiErr) {
				t.Fatalf("error type = %T, want *errors.Error", err)
			}
			if apiErr.Status != http.StatusBadRequest || apiErr.Code != "VALIDATION_ERROR" {
				t.Fatalf("error = %+v, want VALIDATION_ERROR", apiErr)
			}
			if !strings.Contains(apiErr.Message, tt.wantErr) {
				t.Fatalf("message = %q, want it to mention %q", apiErr.Message, tt.wantErr)
			}
		})
	}
}

func TestHandlerAcceptSuccess(t *testing.T) {
	const orderID = "00000000-0000-0000-0000-000000000001"
	const driverID = "00000000-0000-0000-0000-000000000002"
	service := &fakeService{acceptOut: OrderOutput{ID: orderID}}
	handler := NewHandler(service)
	req := &fakeRequest{
		header: map[string]string{"driver_id": driverID},
		params: map[string]string{"orderId": orderID},
	}

	response, err := handler.accept(context.Background(), req)
	if err != nil {
		t.Fatalf("accept() error = %v", err)
	}
	if response.Status != http.StatusCreated {
		t.Fatalf("status = %d, want 201", response.Status)
	}
	if got := response.Body.(OrderOutput); got.ID != orderID {
		t.Fatalf("body = %+v, want order with id %q", got, orderID)
	}

	input := service.acceptIn
	if service.calls != 1 || input.OrderID != orderID || input.DriverID != driverID {
		t.Fatalf("service input = %+v (calls %d), want order %q and driver %q", input, service.calls, orderID, driverID)
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
				params: map[string]string{"orderId": "00000000-0000-0000-0000-000000000001"},
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
	handler := NewHandler(&fakeService{acceptErr: want})
	req := &fakeRequest{
		header: map[string]string{"driver_id": "00000000-0000-0000-0000-000000000002"},
		params: map[string]string{"orderId": "00000000-0000-0000-0000-000000000001"},
	}

	_, err := handler.accept(context.Background(), req)
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
}
