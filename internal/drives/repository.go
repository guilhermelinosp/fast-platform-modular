package drives

import (
	"context"

	"github.com/guilhermelinosp/hellnet-lib-database/database"
)

// Database persists driver actions in PostgreSQL.
type Database struct{ db *database.DB }

// NewRepository creates a driver repository.
func NewRepository(db *database.DB) *Database { return &Database{db: db} }

// Accepted persists a ride acceptance and its outbox event atomically.
func (r *Database) Accepted(ctx context.Context, input AcceptedInput) (Ride, error) {
	_ = ctx
	var ride Ride
	err := r.db.Transactional(func(tx *database.Tx) error {
		if _, err := tx.Execute("INSERT INTO ride_acceptances (id, ride_id, driver_id) VALUES ($1, $2, $3)", input.AcceptanceID, input.RideID, input.DriverID); err != nil {
			return err
		}
		if _, err := tx.Execute("INSERT INTO ride_status_history (id, ride_id, sequence, status_id) SELECT $1::uuid, $2::uuid, COALESCE(MAX(sequence), 0) + 1, 3 FROM ride_status_history WHERE ride_id = $2::uuid", input.StatusHistoryID, input.RideID); err != nil {
			return err
		}
		if _, err := tx.Execute("INSERT INTO outbox_events (id, aggregate_type, aggregate_id, event_type, event_version, payload) VALUES ($1::uuid, 'ride', $2::uuid, $4, 1, $3::jsonb)", input.OutboxID, input.RideID, input.Payload, input.EventType); err != nil {
			return err
		}
		ride.ID = input.RideID
		return nil
	})
	return ride, err
}
