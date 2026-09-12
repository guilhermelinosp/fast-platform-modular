package rides

import (
	"context"

	"github.com/guilhermelinosp/hellnet-lib-database/database"
)

// Database persists rider data in PostgreSQL.
type Database struct{ db *database.DB }

// NewDatabase creates a rider database repository.
func NewDatabase(db *database.DB) *Database {
	return &Database{db: db}
}

// Requested persists a requested ride and its outbox event.
func (r *Database) Requested(ctx context.Context, input RequestedInput) (Ride, error) {
	_ = ctx
	var ride Ride
	err := r.db.Transactional(func(tx *database.Tx) error {
		if _, err := tx.Execute(
			"INSERT INTO rides (id, rider_id, pickup_latitude, pickup_longitude, destination_latitude, destination_longitude) VALUES ($1, $2, $3, $4, $5, $6)",
			input.ID,
			input.RiderID,
			input.PickupLatitude,
			input.PickupLongitude,
			input.DestinationLatitude,
			input.DestinationLongitude); err != nil {
			return err
		}

		if _, err := tx.Execute(
			"INSERT INTO ride_status_history (id,ride_id,sequence,status_id) VALUES ($1, $2, 1, 1)",
			input.StatusHistoryID,
			input.ID); err != nil {
			return err
		}

		if _, err := tx.Execute(
			"INSERT INTO outbox_events (id,aggregate_type,aggregate_id,event_type,event_version, payload) VALUES ($1, 'ride', $2, 'fast-ride-requested.v1', 1, $3::jsonb)",
			input.OutboxID,
			input.ID,
			input.Payload); err != nil {
			return err
		}

		ride = Ride{
			ID:                   input.ID,
			RiderID:              input.RiderID,
			PickupLatitude:       input.PickupLatitude,
			PickupLongitude:      input.PickupLongitude,
			DestinationLatitude:  input.DestinationLatitude,
			DestinationLongitude: input.DestinationLongitude,
		}

		return nil
	})
	return ride, err
}

// Accepted persists a ride acceptance and its outbox event.
func (r *Database) Accepted(ctx context.Context, input AcceptedInput) (Ride, error) {
	_ = ctx
	var ride Ride
	err := r.db.Transactional(func(tx *database.Tx) error {
		if _, err := tx.Execute(
			"INSERT INTO ride_acceptances (id, ride_id, driver_id) VALUES ($1, $2, $3)",
			input.AcceptanceID,
			input.RideID,
			input.DriverID); err != nil {
			return err
		}

		if _, err := tx.Execute(
			"INSERT INTO ride_status_history (id, ride_id, sequence, status_id) SELECT $1::uuid, $2::uuid, COALESCE(MAX(sequence), 0) + 1, 3 FROM ride_status_history WHERE ride_id = $2::uuid",
			input.StatusHistoryID,
			input.RideID); err != nil {
			return err
		}

		if _, err := tx.Execute(
			"INSERT INTO outbox_events (id, aggregate_type, aggregate_id, event_type, event_version, payload) VALUES ($1::uuid, 'ride', $2::uuid, 'fast-ride-accepted.v1', 1, $3::jsonb)",
			input.OutboxID,
			input.RideID,
			input.Payload); err != nil {
			return err
		}

		ride = Ride{
			ID: input.RideID,
		}
		return nil
	})
	return ride, err
}
