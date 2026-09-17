package drives

import (
	"context"
	"errors"
	"fmt"

	"github.com/guilhermelinosp/hellnet-lib-database/database"
)

// Domain errors surfaced by the repository. The service maps them to API
// responses; callers should test with errors.Is.
var (
	// ErrDriverNotFound means no matching row exists in drivers.
	ErrDriverNotFound = errors.New("driver not found")
	// ErrRideNotAcceptable means the ride is not in the requested state
	// (missing, already accepted, or in any other terminal state).
	ErrRideNotAcceptable = errors.New("ride is not in an acceptable state")
)

// Database persists driver actions in PostgreSQL.
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

// NewRepository creates a driver repository.
func NewRepository(db *database.DB) *Database { return &Database{db: db} }

// Accepted persists a ride acceptance and its outbox event atomically, but
// only when the ride is still in the requested state and the driver exists.
//
// Both guards are enforced by the SQL itself: the acceptance INSERT is a
// SELECT guarded by EXISTS(drivers) and the status-history INSERT is a SELECT
// guarded by the current state. A zero-row result means the guard failed, and
// the transaction aborts with the matching domain error.
func (r *Database) Accepted(ctx context.Context, input AcceptedInput) (Ride, error) {
	_ = ctx
	var ride Ride
	err := transactional(r.db, func(execute execFn) error {
		rows, err := execute("INSERT INTO ride_acceptances (id, ride_id, driver_id) SELECT $1::uuid, $2::uuid, $3::uuid WHERE EXISTS (SELECT 1 FROM drivers WHERE id = $3::uuid)", input.AcceptanceID, input.RideID, input.DriverID)
		if err != nil {
			return err
		}
		if rows == 0 {
			return fmt.Errorf("%w: %s", ErrDriverNotFound, input.DriverID)
		}

		rows, err = execute("INSERT INTO ride_status_history (id, ride_id, sequence, status_id) SELECT $1::uuid, $2::uuid, COALESCE((SELECT MAX(sequence) FROM ride_status_history WHERE ride_id = $2::uuid), 0) + 1, 3 WHERE EXISTS (SELECT 1 FROM ride_status_history WHERE ride_id = $2::uuid AND status_id = 1) AND NOT EXISTS (SELECT 1 FROM ride_status_history WHERE ride_id = $2::uuid AND status_id = 3)", input.StatusHistoryID, input.RideID)
		if err != nil {
			return err
		}
		if rows == 0 {
			return fmt.Errorf("%w: %s", ErrRideNotAcceptable, input.RideID)
		}

		if _, err := execute("INSERT INTO outbox_events (id, aggregate_type, aggregate_id, event_type, event_version, payload) VALUES ($1::uuid, 'ride', $2::uuid, $4, 1, $3::jsonb)", input.OutboxID, input.RideID, input.Payload, input.EventType); err != nil {
			return err
		}
		ride.ID = input.RideID
		return nil
	})
	return ride, err
}
