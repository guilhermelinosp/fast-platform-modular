package rider

// Ride is the persisted ride representation returned by the service.
type Ride struct {
	ID, RiderID                                                                string
	PickupLatitude, PickupLongitude, DestinationLatitude, DestinationLongitude float64
}

// Requested is the Kafka event emitted when a ride is requested.
type Requested struct {
	EventID              string  `json:"eventId" avro:"eventId"`
	EventVersion         int     `json:"eventVersion" avro:"eventVersion"`
	OccurredAt           int64   `json:"occurredAt" avro:"occurredAt"`
	RideID               string  `json:"rideId" avro:"rideId"`
	RiderID              string  `json:"riderId" avro:"riderId"`
	PickupLatitude       float64 `json:"pickupLatitude" avro:"pickupLatitude"`
	PickupLongitude      float64 `json:"pickupLongitude" avro:"pickupLongitude"`
	DestinationLatitude  float64 `json:"destinationLatitude" avro:"destinationLatitude"`
	DestinationLongitude float64 `json:"destinationLongitude" avro:"destinationLongitude"`
}

// MessageType returns the requested event type.
func (Requested) MessageType() string { return "fast-ride-requested.v1" }

// Accepted is the Kafka event emitted when a ride is accepted.
type Accepted struct {
	EventID      string `json:"eventId"`
	EventVersion int    `json:"eventVersion"`
	OccurredAt   int64  `json:"occurredAt"`
	RideID       string `json:"rideId"`
	DriverID     string `json:"driverId"`
}

// MessageType returns the accepted event type.
func (Accepted) MessageType() string { return "fast-ride-accepted.v1" }

// RequestedInput contains data needed to request a ride.
type RequestedInput struct {
	ID, RiderID                                                                string
	PickupLatitude, PickupLongitude, DestinationLatitude, DestinationLongitude float64
	StatusHistoryID, OutboxID                                                  string
	Payload                                                                    []byte
}

// AcceptedInput contains data needed to accept a ride.
type AcceptedInput struct {
	RideID, DriverID, AcceptanceID, StatusHistoryID, OutboxID string
	Payload                                                   []byte
}

// RideOutput is the public ride response.
type RideOutput struct {
	ID, RiderID                                                                string
	PickupLatitude, PickupLongitude, DestinationLatitude, DestinationLongitude float64
}
