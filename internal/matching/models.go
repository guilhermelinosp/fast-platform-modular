// Package matching implements the ride-to-driver matching consumer. It owns
// the dedicated "fast-matching" consumer group: it selects an available driver
// for each requested ride and records an offer so the driver BFF can accept
// it. Persisting offers in PostgreSQL keeps matching consistent with the
// acceptance guard (the same ride_acceptances flow) and survives consumer
// restarts.
package matching

// Offer is a driver assigned to a ride by the matching consumer.
type Offer struct {
	ID       string
	RideID   string
	DriverID string
	Status   string
}

// OfferOutput is the public offer representation.
type OfferOutput struct {
	ID       string `json:"id"`
	RideID   string `json:"ride_id"`
	DriverID string `json:"driver_id"`
	Status   string `json:"status"`
}
