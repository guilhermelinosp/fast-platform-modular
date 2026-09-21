package drivers

import (
	"context"
	"errors"
	"testing"

	"github.com/guilhermelinosp/hellnet-lib-database/database"
)

// fakeTx records every SQL statement the repository executes, in order, and
// returns one affected row per statement by default.
type fakeTx struct {
	stmts []string
	args  [][]any
	err   error
}

func (f *fakeTx) run(fn func(execute execFn) error) error {
	return fn(func(sql string, args ...any) (int64, error) {
		f.stmts = append(f.stmts, sql)
		f.args = append(f.args, args)
		if f.err != nil {
			return 0, f.err
		}
		return 1, nil
	})
}

// installFakeTx swaps the package-level transactional seam for the test.
func installFakeTx(t *testing.T, tx *fakeTx) {
	t.Helper()
	original := transactional
	transactional = func(_ *database.DB, fn func(execute execFn) error) error {
		return tx.run(fn)
	}
	t.Cleanup(func() { transactional = original })
}

// TestRepositorySetAvailabilityAppendsIdentityAndEvent asserts the append-only
// contract: the driver identity insert (insert-if-missing) comes first and the
// availability event insert second. Nothing is updated or deleted.
func TestRepositorySetAvailabilityAppendsIdentityAndEvent(t *testing.T) {
	tx := &fakeTx{}
	installFakeTx(t, tx)

	driver, err := NewRepository(nil).SetAvailability(context.Background(), AvailabilityInput{DriverID: "driver-1", Available: true})
	if err != nil {
		t.Fatalf("SetAvailability() error = %v", err)
	}
	if driver.ID != "driver-1" || driver.Available != true {
		t.Fatalf("driver = %+v, want driver-1 available", driver)
	}
	if len(tx.stmts) != 2 {
		t.Fatalf("statements = %d, want 2 (identity + availability event)", len(tx.stmts))
	}
	if tx.args[0][0] != "driver-1" {
		t.Fatalf("identity args = %v, want (driver-1)", tx.args[0])
	}
	if tx.args[1][0] != "driver-1" || tx.args[1][1] != true {
		t.Fatalf("availability args = %v, want (driver-1, true)", tx.args[1])
	}
}

func TestRepositorySetAvailabilityFalse(t *testing.T) {
	tx := &fakeTx{}
	installFakeTx(t, tx)

	driver, err := NewRepository(nil).SetAvailability(context.Background(), AvailabilityInput{DriverID: "driver-1", Available: false})
	if err != nil {
		t.Fatalf("SetAvailability() error = %v", err)
	}
	if driver.ID != "driver-1" || driver.Available != false {
		t.Fatalf("driver = %+v, want driver-1 unavailable", driver)
	}
	if tx.args[1][1] != false {
		t.Fatalf("availability args = %v, want (driver-1, false)", tx.args[1])
	}
}

func TestRepositorySetAvailabilityFailure(t *testing.T) {
	wantErr := errors.New("insert failed")
	tx := &fakeTx{err: wantErr}
	installFakeTx(t, tx)

	_, err := NewRepository(nil).SetAvailability(context.Background(), AvailabilityInput{DriverID: "driver-1", Available: true})
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
}
