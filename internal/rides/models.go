package rides

import "os"

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
func (Requested) MessageType() string {
	if topic := os.Getenv("HELLNET_KAFKA_TOPIC_RIDE_REQUESTED"); topic != "" {
		return topic
	}
	return "fast-ride-requested.v1"
}

// Accepted is the Kafka event emitted when a ride is accepted.
type Accepted struct {
	EventID      string `json:"eventId"`
	EventVersion int    `json:"eventVersion"`
	OccurredAt   int64  `json:"occurredAt"`
	RideID       string `json:"rideId"`
	DriverID     string `json:"driverId"`
}

// MessageType returns the accepted event type.
func (Accepted) MessageType() string {
	if topic := os.Getenv("HELLNET_KAFKA_TOPIC_RIDE_ACCEPTED"); topic != "" {
		return topic
	}
	return "fast-ride-accepted.v1"
}

// RequestedInput contains data needed to request a ride.
type RequestedInput struct {
	ID, RiderID                                                                string
	PickupLatitude, PickupLongitude, DestinationLatitude, DestinationLongitude float64
	StatusHistoryID, OutboxID                                                  string
	Payload                                                                    []byte
	EventType                                                                  string
}

// RideOutput is the public ride response.
type RideOutput struct {
	ID                   string  `json:"id"`
	RiderID              string  `json:"rider_id"`
	PickupLatitude       float64 `json:"pickup_latitude"`
	PickupLongitude      float64 `json:"pickup_longitude"`
	DestinationLatitude  float64 `json:"destination_latitude"`
	DestinationLongitude float64 `json:"destination_longitude"`
}
