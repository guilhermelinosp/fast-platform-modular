package drives

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/guilhermelinosp/hellnet-lib-database/database"
)

// fakeTx records every SQL statement the repository executes, in order, and
// returns per-statement results (defaulting to success) so tests can force a
// failure at any step.
type fakeTx struct {
	stmts   []string
	args    [][]any
	failAt  int
	failErr error
}

func (f *fakeTx) run(fn func(execute execFn) error) error {
	i := 0
	return fn(func(sql string, args ...any) (int64, error) {
		f.stmts = append(f.stmts, sql)
		f.args = append(f.args, args)
		if f.failErr != nil && i == f.failAt {
			return 0, f.failErr
		}
		i++
		return 1, nil
	})
}

// installFakeTx swaps the package transactional hook for the test window.
func installFakeTx(t *testing.T, tx *fakeTx) {
	t.Helper()
	orig := transactional
	transactional = func(_ *database.DB, fn func(execute execFn) error) error {
		return tx.run(fn)
	}
	t.Cleanup(func() { transactional = orig })
}

func TestRepositoryAcceptedInsertsAcceptanceHistoryOutboxInOrder(t *testing.T) {
	tx := &fakeTx{}
	installFakeTx(t, tx)

	input := AcceptedInput{
		RideID:           "ride-1",
		DriverID:         "driver-1",
		AcceptanceID:     "acceptance-1",
		StatusHistoryID:  "status-1",
		OutboxID:         "outbox-1",
		Payload:          []byte(`{"eventId":"outbox-1"}`),
		EventType:        "fast.ride.accepted.v1",
	}

	got, err := NewRepository(nil).Accepted(context.Background(), input)
	if err != nil {
		t.Fatalf("Accepted() error = %v", err)
	}

	if len(tx.stmts) != 3 {
		t.Fatalf("statements = %d, want 3: %q", len(tx.stmts), tx.stmts)
	}
	wantSQL := []string{
		"INSERT INTO ride_acceptances",
		"INSERT INTO ride_status_history",
		"INSERT INTO outbox_events",
	}
	for i, want := range wantSQL {
		if !strings.HasPrefix(tx.stmts[i], want) {
			t.Errorf("statement[%d] = %q, want prefix %q", i, tx.stmts[i], want)
		}
	}

	// First statement: acceptance id, ride id, driver id.
	if len(tx.args[0]) != 3 ||
		tx.args[0][0] != input.AcceptanceID || tx.args[0][1] != input.RideID || tx.args[0][2] != input.DriverID {
		t.Errorf("acceptance args = %v, want [%q %q %q]", tx.args[0], input.AcceptanceID, input.RideID, input.DriverID)
	}

	// Second statement: status history id + ride id.
	if len(tx.args[1]) != 2 || tx.args[1][0] != input.StatusHistoryID || tx.args[1][1] != input.RideID {
		t.Errorf("status history args = %v, want [%q %q]", tx.args[1], input.StatusHistoryID, input.RideID)
	}

	// Third statement: outbox id, ride id, payload, event type.
	outArgs := tx.args[2]
	if len(outArgs) != 4 || outArgs[0] != input.OutboxID || outArgs[1] != input.RideID {
		t.Errorf("outbox args = %v, want [%q %q ...]", outArgs, input.OutboxID, input.RideID)
	}
	if !bytes.Equal(outArgs[2].([]byte), input.Payload) || outArgs[3] != input.EventType {
		t.Errorf("outbox payload/type = (%v, %v), want (%q, %q)", outArgs[2], outArgs[3], input.Payload, input.EventType)
	}

	if got.ID != input.RideID {
		t.Errorf("got.ID = %q, want %q", got.ID, input.RideID)
	}
}

func TestRepositoryAcceptedFailsOnAcceptanceInsert(t *testing.T) {
	wantErr := errors.New("insert acceptance failed")
	tx := &fakeTx{failAt: 0, failErr: wantErr}
	installFakeTx(t, tx)

	_, err := NewRepository(nil).Accepted(context.Background(), AcceptedInput{RideID: "ride-1", DriverID: "driver-1"})
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
	if len(tx.stmts) != 1 {
		t.Errorf("statements = %d, want 1 (short-circuit on first error)", len(tx.stmts))
	}
}

func TestRepositoryAcceptedFailsOnStatusHistoryInsert(t *testing.T) {
	wantErr := errors.New("insert status failed")
	tx := &fakeTx{failAt: 1, failErr: wantErr}
	installFakeTx(t, tx)

	_, err := NewRepository(nil).Accepted(context.Background(), AcceptedInput{RideID: "ride-1", DriverID: "driver-1"})
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
	if len(tx.stmts) != 2 {
		t.Errorf("statements = %d, want 2 (acceptance succeeded, status failed)", len(tx.stmts))
	}
}

func TestRepositoryAcceptedFailsOnOutboxInsert(t *testing.T) {
	wantErr := errors.New("insert outbox failed")
	tx := &fakeTx{failAt: 2, failErr: wantErr}
	installFakeTx(t, tx)

	_, err := NewRepository(nil).Accepted(context.Background(), AcceptedInput{RideID: "ride-1", DriverID: "driver-1"})
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
	if len(tx.stmts) != 3 {
		t.Errorf("statements = %d, want 3 (two inserts then outbox failed)", len(tx.stmts))
	}
}