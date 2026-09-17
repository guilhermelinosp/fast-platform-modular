package rides

import (
	"context"

	"github.com/guilhermelinosp/hellnet-lib-database/database"
)

// Database persists rider data in PostgreSQL.
type Database struct{ db *database.DB }

// execFn is a single SQL execution inside a transaction. It is the unit the
// repository depends on, so tests can capture statements without a database.
type execFn func(sql string, args ...any) (int64, error)

// transactional runs fn inside a database transaction. It is a package
// variable so tests can replace it with a fake that records statements.
var transactional = func(db *database.DB, fn func(execute execFn) error) error {
	return db.Transactional(func(tx *database.Tx) error {
		return fn(tx.Execute)
	})
}

// NewRepository creates a rider database repository.
func NewRepository(db *database.DB) *Database {
	return &Database{db: db}
}

// Requested persists a requested ride and its outbox event.
func (r *Database) Requested(ctx context.Context, input RequestedInput) (Ride, error) {
	_ = ctx
	var ride Ride
	err := transactional(r.db, func(execute execFn) error {
		if _, err := execute(
			"INSERT INTO rides (id, rider_id, pickup_latitude, pickup_longitude, destination_latitude, destination_longitude) VALUES ($1, $2, $3, $4, $5, $6)",
			input.ID,
			input.RiderID,
			input.PickupLatitude,
			input.PickupLongitude,
			input.DestinationLatitude,
			input.DestinationLongitude); err != nil {
			return err
		}

		if _, err := execute(
			"INSERT INTO ride_status_history (id,ride_id,sequence,status_id) VALUES ($1, $2, 1, 1)",
			input.StatusHistoryID,
			input.ID); err != nil {
			return err
		}

		if _, err := execute(
			"INSERT INTO outbox_events (id,aggregate_type,aggregate_id,event_type,event_version, payload) VALUES ($1, 'ride', $2, $4, 1, $3::jsonb)",
			input.OutboxID,
			input.ID,
			input.Payload, input.EventType); err != nil {
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