package matching

import (
	"context"
	"errors"
	"testing"

	"github.com/guilhermelinosp/hellnet-lib-database/database"
)

type fakeTx struct {
	driverID  string
	found     bool
	driverErr error
	rows      int64
	rowsErr   error
	stmts     []string
	args      [][]any
}

func (f *fakeTx) run(fn func(execute execFn, driver driverFn) error) error {
	return fn(
		func(sql string, args ...any) (int64, error) {
			f.stmts = append(f.stmts, sql)
			f.args = append(f.args, args)
			return f.rows, f.rowsErr
		},
		func() (string, bool, error) {
			return f.driverID, f.found, f.driverErr
		},
	)
}

func installFakeTx(t *testing.T, tx *fakeTx) {
	t.Helper()
	original := transactional
	transactional = func(_ *database.DB, fn func(execute execFn, driver driverFn) error) error {
		return tx.run(fn)
	}
	t.Cleanup(func() { transactional = original })
}

func TestRepositoryMatchInsertsOffer(t *testing.T) {
	tx := &fakeTx{driverID: "driver-1", found: true, rows: 1}
	installFakeTx(t, tx)

	offer, err := NewRepository(nil).Match(context.Background(), "ride-1")
	if err != nil {
		t.Fatalf("Match() error = %v", err)
	}
	if offer.RideID != "ride-1" || offer.DriverID != "driver-1" || offer.Status != "pending" {
		t.Fatalf("offer = %+v, want ride-1/driver-1/pending", offer)
	}
	if len(tx.stmts) != 1 {
		t.Fatalf("statements = %d, want 1 (offer insert)", len(tx.stmts))
	}
	if tx.args[0][0] != "ride-1" || tx.args[0][1] != "driver-1" {
		t.Fatalf("insert args = %v, want (ride-1, driver-1)", tx.args[0])
	}
}

func TestRepositoryMatchNoDriver(t *testing.T) {
	tx := &fakeTx{found: false}
	installFakeTx(t, tx)

	_, err := NewRepository(nil).Match(context.Background(), "ride-1")
	if !errors.Is(err, ErrNoDriverAvailable) {
		t.Fatalf("error = %v, want ErrNoDriverAvailable", err)
	}
	if len(tx.stmts) != 0 {
		t.Fatalf("statements = %d, want 0 (no insert when no driver)", len(tx.stmts))
	}
}

func TestRepositoryMatchAlreadyMatched(t *testing.T) {
	tx := &fakeTx{driverID: "driver-1", found: true, rows: 0}
	installFakeTx(t, tx)

	_, err := NewRepository(nil).Match(context.Background(), "ride-1")
	if !errors.Is(err, ErrRideAlreadyMatched) {
		t.Fatalf("error = %v, want ErrRideAlreadyMatched", err)
	}
}

func TestRepositoryMatchDriverQueryError(t *testing.T) {
	wantErr := errors.New("query failed")
	tx := &fakeTx{driverErr: wantErr}
	installFakeTx(t, tx)

	_, err := NewRepository(nil).Match(context.Background(), "ride-1")
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
}

func TestRepositoryMatchInsertError(t *testing.T) {
	wantErr := errors.New("insert failed")
	tx := &fakeTx{driverID: "driver-1", found: true, rowsErr: wantErr}
	installFakeTx(t, tx)

	_, err := NewRepository(nil).Match(context.Background(), "ride-1")
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
}
