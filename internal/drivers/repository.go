package drivers

import (
	"context"

	"github.com/guilhermelinosp/hellnet-lib-database/database"
)

// Database persists driver availability in PostgreSQL.
//
// The database is append-only by decision: availability changes are recorded
// as INSERTs into driver_availability_events and the drivers table is a
// read-only identity registry. Nothing is ever updated or deleted; the current
// availability of a driver is always the latest event for that driver.
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

// NewRepository creates a driver availability repository.
func NewRepository(db *database.DB) *Database { return &Database{db: db} }

// SetAvailability records a driver availability change as an append-only
// event. The driver identity row is created when first seen (insert-if-missing
// via WHERE NOT EXISTS — still an INSERT, never an UPDATE). Matching later
// resolves the current availability from the latest event per driver.
func (r *Database) SetAvailability(ctx context.Context, input AvailabilityInput) (Driver, error) {
	_ = ctx
	var driver Driver
	err := transactional(r.db, func(execute execFn) error {
		if _, err := execute(
			"INSERT INTO drivers (id) SELECT $1 WHERE NOT EXISTS (SELECT 1 FROM drivers WHERE id = $1)",
			input.DriverID); err != nil {
			return err
		}
		if _, err := execute(
			"INSERT INTO driver_availability_events (id, driver_id, available, occurred_at) VALUES (gen_random_uuid(), $1, $2, now())",
			input.DriverID,
			input.Available); err != nil {
			return err
		}
		driver = Driver{ID: input.DriverID, Available: input.Available}
		return nil
	})
	if err != nil {
		return Driver{}, err
	}
	return driver, nil
}
