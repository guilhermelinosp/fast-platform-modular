package orders

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

func TestRepositoryRequestedInsertsOrderHistoryOutboxInOrder(t *testing.T) {
	tx := &fakeTx{}
	installFakeTx(t, tx)

	input := OrderRequestedInput{
		ID:                   "order-1",
		RiderID:              "rider-1",
		PickupLatitude:       1.1,
		PickupLongitude:      2.2,
		DestinationLatitude:  3.3,
		DestinationLongitude: 4.4,
		StatusHistoryID:      "status-1",
		OutboxID:             "outbox-1",
		Payload:              []byte(`{"eventId":"outbox-1"}`),
		EventType:            "fast.order.requested.v1",
	}

	got, err := NewRepository(nil).Requested(context.Background(), input)
	if err != nil {
		t.Fatalf("Requested() error = %v", err)
	}

	if len(tx.stmts) != 3 {
		t.Fatalf("statements = %d, want 3: %q", len(tx.stmts), tx.stmts)
	}
	wantSQL := []string{
		"INSERT INTO orders",
		"INSERT INTO order_status_history",
		"INSERT INTO outbox_events",
	}
	for i, want := range wantSQL {
		if !strings.HasPrefix(tx.stmts[i], want) {
			t.Errorf("statement[%d] = %q, want prefix %q", i, tx.stmts[i], want)
		}
	}

	// First statement arguments: id, rider_id, pickup lat/lon, dest lat/lon.
	orderArgs := tx.args[0]
	wantOrderArgs := []any{input.ID, input.RiderID, input.PickupLatitude, input.PickupLongitude, input.DestinationLatitude, input.DestinationLongitude}
	if len(orderArgs) != len(wantOrderArgs) {
		t.Fatalf("order args = %v, want %v", orderArgs, wantOrderArgs)
	}
	for i := range wantOrderArgs {
		if orderArgs[i] != wantOrderArgs[i] {
			t.Errorf("order arg[%d] = %v, want %v", i, orderArgs[i], wantOrderArgs[i])
		}
	}

	// Second statement: status history id + order id.
	if len(tx.args[1]) != 2 || tx.args[1][0] != input.StatusHistoryID || tx.args[1][1] != input.ID {
		t.Errorf("status history args = %v, want [%q %q]", tx.args[1], input.StatusHistoryID, input.ID)
	}

	// Third statement: outbox id, order id, payload, event type.
	outArgs := tx.args[2]
	if len(outArgs) != 4 || outArgs[0] != input.OutboxID || outArgs[1] != input.ID {
		t.Errorf("outbox args = %v, want [%q %q ...]", outArgs, input.OutboxID, input.ID)
	}
	if !bytes.Equal(outArgs[2].([]byte), input.Payload) || outArgs[3] != input.EventType {
		t.Errorf("outbox payload/type = (%v, %v), want (%q, %q)", outArgs[2], outArgs[3], input.Payload, input.EventType)
	}

	if got.ID != input.ID || got.RiderID != input.RiderID {
		t.Errorf("got = %+v, want id %q rider %q", got, input.ID, input.RiderID)
	}
	if got.PickupLatitude != input.PickupLatitude || got.PickupLongitude != input.PickupLongitude ||
		got.DestinationLatitude != input.DestinationLatitude || got.DestinationLongitude != input.DestinationLongitude {
		t.Errorf("got coords = %+v, want input coords", got)
	}
}

func TestRepositoryRequestedFailsOnOrderInsert(t *testing.T) {
	wantErr := errors.New("insert orders failed")
	tx := &fakeTx{failAt: 0, failErr: wantErr}
	installFakeTx(t, tx)

	_, err := NewRepository(nil).Requested(context.Background(), OrderRequestedInput{ID: "order-1", RiderID: "rider-1"})
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
	if len(tx.stmts) != 1 {
		t.Errorf("statements = %d, want 1 (short-circuit on first error)", len(tx.stmts))
	}
}

func TestRepositoryRequestedFailsOnStatusHistoryInsert(t *testing.T) {
	wantErr := errors.New("insert status failed")
	tx := &fakeTx{failAt: 1, failErr: wantErr}
	installFakeTx(t, tx)

	_, err := NewRepository(nil).Requested(context.Background(), OrderRequestedInput{ID: "order-1", RiderID: "rider-1"})
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
	if len(tx.stmts) != 2 {
		t.Errorf("statements = %d, want 2 (order succeeded, status failed)", len(tx.stmts))
	}
}

func TestRepositoryRequestedFailsOnOutboxInsert(t *testing.T) {
	wantErr := errors.New("insert outbox failed")
	tx := &fakeTx{failAt: 2, failErr: wantErr}
	installFakeTx(t, tx)

	_, err := NewRepository(nil).Requested(context.Background(), OrderRequestedInput{ID: "order-1", RiderID: "rider-1"})
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
	if len(tx.stmts) != 3 {
		t.Errorf("statements = %d, want 3 (two inserts then outbox failed)", len(tx.stmts))
	}
}
