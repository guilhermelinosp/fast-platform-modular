package drivers

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
