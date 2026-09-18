package matching

import (
	"context"
	"errors"

	"github.com/guilhermelinosp/hellnet-lib-database/database"
)

// ErrNoDriverAvailable is returned when no driver is currently available.
var ErrNoDriverAvailable = errors.New("matching: no driver available")

// ErrRideAlreadyMatched is returned when the ride already has an offer.
var ErrRideAlreadyMatched = errors.New("matching: ride already matched")

// availableDriverRow maps the driver selected by the matching query.
type availableDriverRow struct {
	DriverID string `db:"driver_id"`
}

// selectAvailableDriverSQL picks the driver whose latest availability event is
// available (append-only resolution: latest event per driver wins). It is a
// plain SELECT — the database is never mutated by matching.
const selectAvailableDriverSQL = `
SELECT e.driver_id
FROM driver_availability_events e
LEFT JOIN driver_availability_events newer
  ON newer.driver_id = e.driver_id AND newer.occurred_at > e.occurred_at
WHERE e.available AND newer.driver_id IS NULL
ORDER BY e.occurred_at
LIMIT 1`

// insertOfferSQL records one offer for an unmatched ride. The INSERT ... SELECT
// with WHERE NOT EXISTS makes matching idempotent without updating anything:
// a second attempt for the same ride inserts zero rows and is reported through
// the affected-rows count.
const insertOfferSQL = `
INSERT INTO ride_offers (id, ride_id, driver_id, status, created_at)
SELECT gen_random_uuid(), $1, $2, 'pending', now()
WHERE NOT EXISTS (SELECT 1 FROM ride_offers WHERE ride_id = $1)`

// execFn is a single SQL execution inside a transaction. It is the unit the
// repository depends on, so tests can capture statements without a database.
type execFn func(sql string, args ...any) (int64, error)

// driverFn selects the available driver inside the same transaction. Tests
// replace it with a canned result.
type driverFn func() (string, bool, error)

// transactional runs fn inside a database transaction, handing it the execute
// seam and a driver-selection closure built on the same transaction. Package
// variable so tests can replace it with a fake.
var transactional = func(db *database.DB, fn func(execute execFn, driver driverFn) error) error {
	return db.Transactional(func(tx *database.Tx) error {
		return fn(tx.Execute, func() (string, bool, error) {
			row, found, err := database.TxQueryRow[availableDriverRow](tx, selectAvailableDriverSQL)
			if err != nil || !found {
				return "", found, err
			}
			return row.DriverID, true, nil
		})
	})
}

// Database persists ride offers in PostgreSQL.
type Database struct{ db *database.DB }

// NewRepository creates a matching repository.
func NewRepository(db *database.DB) *Database { return &Database{db: db} }

// Match assigns an available driver to an order, recording the offer. Matching
// is idempotent: an order with an existing offer is reported via
// ErrRideAlreadyMatched without inserting a duplicate.
func (r *Database) Match(ctx context.Context, orderID string) (Offer, error) {
	_ = ctx
	var offer Offer
	err := transactional(r.db, func(execute execFn, driver driverFn) error {
		driverID, found, err := driver()
		if err != nil {
			return err
		}
		if !found {
			return ErrNoDriverAvailable
		}
		n, err := execute(insertOfferSQL, orderID, driverID)
		if err != nil {
			return err
		}
		if n == 0 {
			return ErrRideAlreadyMatched
		}
		offer = Offer{OrderID: orderID, DriverID: driverID, Status: "pending"}
		return nil
	})
	return offer, err
}
