package rides

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
	calls int
	input RequestedInput
	out   RideOutput
	err   error
}

func (f *fakeService) Requested(_ context.Context, input RequestedInput) (RideOutput, error) {
	f.calls++
	f.input = input
	return f.out, f.err
}

// fakeRequest implements api.Request with enough surface for handler tests.
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

func TestHandlerRequestSuccess(t *testing.T) {
	const id = "00000000-0000-0000-0000-000000000001"
	const riderID = "00000000-0000-0000-0000-000000000002"
	service := &fakeService{out: RideOutput{ID: id, RiderID: riderID}}
	handler := NewHandler(service)
	req := &fakeRequest{body: []byte(`{"id":"` + id + `","rider_id":"` + riderID + `","pickup_latitude":-23.56}`)}

	response, err := handler.request(context.Background(), req)
	if err != nil {
		t.Fatalf("request() error = %v", err)
	}
	if response.Status != http.StatusCreated {
		t.Fatalf("status = %d, want 201", response.Status)
	}
	if got := response.Body.(RideOutput); got.ID != id {
		t.Fatalf("body = %+v, want ride with id %q", got, id)
	}
	if service.calls != 1 || service.input.ID != id || service.input.RiderID != riderID {
		t.Fatalf("service input = %+v (calls %d), want id %q and rider %q", service.input, service.calls, id, riderID)
	}
}

func TestHandlerRequestHeaderTakesPrecedenceForRiderID(t *testing.T) {
	const bodyRider = "00000000-0000-0000-0000-000000000002"
	const headerRider = "00000000-0000-0000-0000-000000000003"
	service := &fakeService{}
	handler := NewHandler(service)
	req := &fakeRequest{
		body:   []byte(`{"id":"00000000-0000-0000-0000-000000000001","rider_id":"` + bodyRider + `"}`),
		header: map[string]string{"rider_id": headerRider},
	}

	if _, err := handler.request(context.Background(), req); err != nil {
		t.Fatalf("request() error = %v", err)
	}
	if service.input.RiderID != headerRider {
		t.Fatalf("service rider_id = %q, want header value %q", service.input.RiderID, headerRider)
	}
}

func TestHandlerRequestGeneratesMissingIDs(t *testing.T) {
	service := &fakeService{}
	handler := NewHandler(service)

	if _, err := handler.request(context.Background(), &fakeRequest{body: []byte(`{}`)}); err != nil {
		t.Fatalf("request() error = %v", err)
	}
	if _, err := uuid.Parse(service.input.ID); err != nil {
		t.Errorf("generated id %q is not a UUID: %v", service.input.ID, err)
	}
	if _, err := uuid.Parse(service.input.RiderID); err != nil {
		t.Errorf("generated rider_id %q is not a UUID: %v", service.input.RiderID, err)
	}
}

func TestHandlerRequestValidatesID(t *testing.T) {
	handler := NewHandler(&fakeService{})
	req := &fakeRequest{body: []byte(`{"id":"not-a-uuid","rider_id":"00000000-0000-0000-0000-000000000002"}`)}

	_, err := handler.request(context.Background(), req)
	assertHandlerValidation(t, err, "id")
}

func TestHandlerRequestValidatesRiderID(t *testing.T) {
	t.Run("from header", func(t *testing.T) {
		handler := NewHandler(&fakeService{})
		req := &fakeRequest{
			body:   []byte(`{"id":"00000000-0000-0000-0000-000000000001"}`),
			header: map[string]string{"rider_id": "not-a-uuid"},
		}
		_, err := handler.request(context.Background(), req)
		assertHandlerValidation(t, err, "rider_id")
	})

	t.Run("from body", func(t *testing.T) {
		handler := NewHandler(&fakeService{})
		req := &fakeRequest{body: []byte(`{"id":"00000000-0000-0000-0000-000000000001","rider_id":"not-a-uuid"}`)}
		_, err := handler.request(context.Background(), req)
		assertHandlerValidation(t, err, "rider_id")
	})
}

func TestHandlerRequestPropagatesBindAndServiceErrors(t *testing.T) {
	t.Run("bind error", func(t *testing.T) {
		handler := NewHandler(&fakeService{})
		if _, err := handler.request(context.Background(), &fakeRequest{body: []byte(`{`)}); err == nil {
			t.Fatal("request() error = nil, want bind error")
		}
	})

	t.Run("service error", func(t *testing.T) {
		want := errors.New("service boom")
		handler := NewHandler(&fakeService{err: want})
		req := &fakeRequest{body: []byte(`{"id":"00000000-0000-0000-0000-000000000001","rider_id":"00000000-0000-0000-0000-000000000002"}`)}
		_, err := handler.request(context.Background(), req)
		if !errors.Is(err, want) {
			t.Fatalf("error = %v, want %v", err, want)
		}
	})
}

func assertHandlerValidation(t *testing.T, err error, field string) {
	t.Helper()
	if err == nil {
		t.Fatalf("error = nil, want %q validation", field)
	}
	var apiErr *apierrors.Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("error type = %T, want *errors.Error", err)
	}
	if apiErr.Status != http.StatusBadRequest || apiErr.Code != "VALIDATION_ERROR" {
		t.Fatalf("error = %+v, want VALIDATION_ERROR", apiErr)
	}
	if !strings.Contains(apiErr.Message, field) {
		t.Fatalf("message = %q, want it to mention field %q", apiErr.Message, field)
	}
}
