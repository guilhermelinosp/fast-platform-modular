package drives

// AcceptedInput contains data needed to accept a ride.
type AcceptedInput struct {
	RideID, DriverID, AcceptanceID, StatusHistoryID, OutboxID string
	Payload                                                   []byte
	EventType                                                 string
}

// Ride is the persistence result of a driver operation.
type Ride struct{ ID string }

// RideOutput is the public result of a driver operation.
type RideOutput struct {
	ID string `json:"id"`
}
