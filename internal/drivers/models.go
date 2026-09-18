package drivers

import "errors"

// AvailabilityInput contains the driver availability change request.
type AvailabilityInput struct {
	DriverID  string
	Available bool
}

// Driver is the persisted driver availability state.
type Driver struct {
	ID        string
	Available bool
}

// DriverOutput is the public availability response.
type DriverOutput struct {
	ID        string `json:"id"`
	Available bool   `json:"available"`
}

// AcceptedInput contains data needed to accept an order.
type AcceptedInput struct {
	OrderID, DriverID, AcceptanceID, StatusHistoryID, OutboxID string
	Payload                                                    []byte
	EventType                                                  string
}

// Order is the persistence result of a driver operation.
type Order struct{ ID string }

// OrderOutput is the public result of a driver operation.
type OrderOutput struct {
	ID string `json:"id"`
}

// Domain errors surfaced by the repository. The service maps them to API
// responses; callers should test with errors.Is.
var (
	// ErrDriverNotFound means no matching row exists in drivers.
	ErrDriverNotFound = errors.New("driver not found")
	// ErrOrderNotAcceptable means the order is not in the requested state
	// (missing, already accepted, or in any other terminal state).
	ErrOrderNotAcceptable = errors.New("order is not in an acceptable state")
)
