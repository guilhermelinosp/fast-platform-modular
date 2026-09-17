package events

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/guilhermelinosp/fast-platform-modular/internal/rides"
)

func TestOutboxMemoStartClaimsOnce(t *testing.T) {
	memo := newOutboxMemo()

	if !memo.start("id-1") {
		t.Fatal("first start() = false, want true")
	}
	if memo.start("id-1") {
		t.Fatal("start() while in-flight = true, want false")
	}

	memo.release("id-1")
	if !memo.start("id-1") {
		t.Fatal("start() after release = false, want true")
	}
}

func TestOutboxMemoSettleBlocksRepublish(t *testing.T) {
	memo := newOutboxMemo()

	if !memo.start("id-1") {
		t.Fatal("start() = false, want true")
	}
	memo.settle("id-1")

	if memo.start("id-1") {
		t.Fatal("start() after settle = true, want false")
	}
}

func TestOutboxMemoSettleIsIdempotent(t *testing.T) {
	memo := newOutboxMemo()

	memo.settle("id-1")
	memo.settle("id-1")

	if got := len(memo.order); got != 1 {
		t.Fatalf("order length = %d, want 1 (settle must not enqueue twice)", got)
	}
	if got := len(memo.done); got != 1 {
		t.Fatalf("done size = %d, want 1", got)
	}
}

func TestOutboxMemoEvictsOldestWhenBounded(t *testing.T) {
	memo := newOutboxMemo()
	for i := 0; i < memoMaxEvents+1; i++ {
		memo.settle(fmt.Sprintf("id-%d", i))
	}

	if got := len(memo.order); got != memoMaxEvents {
		t.Fatalf("order length = %d, want %d", got, memoMaxEvents)
	}
	if got := len(memo.done); got != memoMaxEvents {
		t.Fatalf("done size = %d, want %d", got, memoMaxEvents)
	}
	if !memo.start("id-0") {
		t.Fatal("oldest settled id was not evicted; start() = false, want true")
	}
	if memo.start(fmt.Sprintf("id-%d", memoMaxEvents)) {
		t.Fatal("most recent settled id was evicted; start() = true, want false")
	}
}

func TestOutboxMemoStartIsConcurrencySafe(t *testing.T) {
	memo := newOutboxMemo()

	const goroutines = 64
	var wins atomic.Int32
	ready := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-ready
			if memo.start("shared") {
				wins.Add(1)
			}
		}()
	}
	close(ready)
	wg.Wait()

	if got := wins.Load(); got != 1 {
		t.Fatalf("concurrent start() wins = %d, want exactly 1", got)
	}
}

func TestPublisherPortsAreNoOps(t *testing.T) {
	var publisher Publisher

	if err := publisher.Requested(rides.Requested{EventID: "event-1"}); err != nil {
		t.Fatalf("Requested() = %v, want nil", err)
	}
	if err := publisher.Accepted(rides.Accepted{EventID: "event-1"}); err != nil {
		t.Fatalf("Accepted() = %v, want nil", err)
	}
}

func TestPublisherPublishRejectsUnsupportedEventType(t *testing.T) {
	var publisher Publisher

	err := publisher.publish(OutboxEvent{ID: "event-1", EventType: "rides.deleted.v9", Payload: []byte(`{}`)})
	if err == nil || !strings.Contains(err.Error(), "unsupported outbox event type") {
		t.Fatalf("publish() = %v, want unsupported outbox event type error", err)
	}
}

func TestPublisherPublishRejectsMalformedPayload(t *testing.T) {
	var publisher Publisher

	tests := []struct {
		name      string
		eventType string
		want      string
	}{
		{name: "requested", eventType: (rides.Requested{}).MessageType(), want: "decode requested event"},
		{name: "accepted", eventType: (rides.Accepted{}).MessageType(), want: "decode accepted event"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := publisher.publish(OutboxEvent{ID: "event-1", EventType: tt.eventType, Payload: []byte(`{"broken"`)})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("publish() = %v, want error containing %q", err, tt.want)
			}
		})
	}
}

// --- fakes -----------------------------------------------------------------

type fakeStore struct {
	row          OutboxEvent
	rowFound     bool
	rowErr       error
	rows         []OutboxEvent
	pendingErr   error
	rowCalls     int
	pendingCalls int
}

func (f *fakeStore) QueryRow(id string) (OutboxEvent, bool, error) {
	f.rowCalls++
	if f.rowErr != nil {
		return OutboxEvent{}, false, f.rowErr
	}
	for _, e := range f.rows {
		if e.ID == id {
			return e, true, nil
		}
	}
	return f.row, f.rowFound, nil
}

func (f *fakeStore) QueryPending() ([]OutboxEvent, error) {
	f.pendingCalls++
	return f.rows, f.pendingErr
}

type fakeRequestedPublisher struct {
	mu    sync.Mutex
	calls int
	last  rides.Requested
	err   error
}

func (f *fakeRequestedPublisher) Publish(m rides.Requested) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.last = m
	return f.err
}

func (f *fakeRequestedPublisher) snapshot() (int, rides.Requested) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls, f.last
}

type fakeAcceptedPublisher struct {
	mu    sync.Mutex
	calls int
	last  rides.Accepted
	err   error
}

func (f *fakeAcceptedPublisher) Publish(m rides.Accepted) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.last = m
	return f.err
}

func newTestPublisher(store outboxStore) (*Publisher, *fakeRequestedPublisher, *fakeAcceptedPublisher) {
	requested := &fakeRequestedPublisher{}
	accepted := &fakeAcceptedPublisher{}
	return &Publisher{
		store:     store,
		requested: requested,
		accepted:  accepted,
		stopScan:  make(chan struct{}),
		memo:      newOutboxMemo(),
	}, requested, accepted
}

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return b
}

// --- publishByID ------------------------------------------------------------

func TestPublisherPublishByIDPublishesRequested(t *testing.T) {
	p, requested, accepted := newTestPublisher(&fakeStore{
		row:      OutboxEvent{ID: "event-1", EventType: (rides.Requested{}).MessageType(), Payload: mustMarshal(t, rides.Requested{EventID: "event-1"})},
		rowFound: true,
	})

	p.publishByID("event-1")

	if got := requested.calls; got != 1 {
		t.Fatalf("requested publish calls = %d, want 1", got)
	}
	if got := requested.last.EventID; got != "event-1" {
		t.Fatalf("published EventID = %q, want event-1", got)
	}
	if got := accepted.calls; got != 0 {
		t.Fatalf("accepted publish calls = %d, want 0", got)
	}
}

func TestPublisherPublishByIDPublishesAccepted(t *testing.T) {
	p, requested, accepted := newTestPublisher(&fakeStore{
		row:      OutboxEvent{ID: "event-1", EventType: (rides.Accepted{}).MessageType(), Payload: mustMarshal(t, rides.Accepted{EventID: "event-1"})},
		rowFound: true,
	})

	p.publishByID("event-1")

	if got := accepted.calls; got != 1 {
		t.Fatalf("accepted publish calls = %d, want 1", got)
	}
	if got := accepted.last.EventID; got != "event-1" {
		t.Fatalf("published EventID = %q, want event-1", got)
	}
	if got := requested.calls; got != 0 {
		t.Fatalf("requested publish calls = %d, want 0", got)
	}
}

func TestPublisherPublishByIDSkipsSettledEvents(t *testing.T) {
	store := &fakeStore{
		row:      OutboxEvent{ID: "event-1", EventType: (rides.Requested{}).MessageType(), Payload: mustMarshal(t, rides.Requested{EventID: "event-1"})},
		rowFound: true,
	}
	p, requested, _ := newTestPublisher(store)

	p.publishByID("event-1")
	p.publishByID("event-1")

	if got := requested.calls; got != 1 {
		t.Fatalf("requested publish calls = %d, want 1 (settled event must not republish)", got)
	}
	if got := store.rowCalls; got != 1 {
		t.Fatalf("QueryRow calls = %d, want 1 (memo must block before re-reading)", got)
	}
}

func TestPublisherPublishByIDRetriesAfterPublishFailure(t *testing.T) {
	store := &fakeStore{
		row:      OutboxEvent{ID: "event-1", EventType: (rides.Requested{}).MessageType(), Payload: mustMarshal(t, rides.Requested{EventID: "event-1"})},
		rowFound: true,
	}
	p, requested, _ := newTestPublisher(store)

	requested.err = errors.New("kafka down")
	p.publishByID("event-1")
	if got := requested.calls; got != 1 {
		t.Fatalf("requested publish calls = %d, want 1", got)
	}

	requested.err = nil
	p.publishByID("event-1")
	if got := requested.calls; got != 2 {
		t.Fatalf("requested publish calls = %d, want 2 (failed event must be retried)", got)
	}
}

func TestPublisherPublishByIDRetriesAfterDecodeError(t *testing.T) {
	store := &fakeStore{
		row:      OutboxEvent{ID: "event-1", EventType: (rides.Requested{}).MessageType(), Payload: []byte(`{"broken"`)},
		rowFound: true,
	}
	p, requested, _ := newTestPublisher(store)

	p.publishByID("event-1")
	p.publishByID("event-1")

	if got := store.rowCalls; got != 2 {
		t.Fatalf("QueryRow calls = %d, want 2 (decode failure must release the claim)", got)
	}
	if got := requested.calls; got != 0 {
		t.Fatalf("requested publish calls = %d, want 0", got)
	}
}

func TestPublisherPublishByIDReleasesWhenNotFound(t *testing.T) {
	store := &fakeStore{rowFound: false}
	p, requested, _ := newTestPublisher(store)

	p.publishByID("event-1")
	p.publishByID("event-1")

	if got := store.rowCalls; got != 2 {
		t.Fatalf("QueryRow calls = %d, want 2 (missing row must release the claim)", got)
	}
	if got := requested.calls; got != 0 {
		t.Fatalf("requested publish calls = %d, want 0", got)
	}
}

func TestPublisherPublishByIDReleasesAfterQueryError(t *testing.T) {
	store := &fakeStore{rowErr: errors.New("db down")}
	p, requested, _ := newTestPublisher(store)

	p.publishByID("event-1")
	p.publishByID("event-1")

	if got := store.rowCalls; got != 2 {
		t.Fatalf("QueryRow calls = %d, want 2 (query failure must release the claim)", got)
	}
	if got := requested.calls; got != 0 {
		t.Fatalf("requested publish calls = %d, want 0", got)
	}
}

// --- reconcileOnce ----------------------------------------------------------

func TestPublisherReconcileOncePublishesPending(t *testing.T) {
	store := &fakeStore{
		rows: []OutboxEvent{
			{ID: "event-1", EventType: (rides.Requested{}).MessageType(), Payload: mustMarshal(t, rides.Requested{EventID: "event-1"})},
			{ID: "event-2", EventType: (rides.Accepted{}).MessageType(), Payload: mustMarshal(t, rides.Accepted{EventID: "event-2"})},
		},
	}
	p, requested, accepted := newTestPublisher(store)

	p.reconcileOnce()

	if got := store.pendingCalls; got != 1 {
		t.Fatalf("QueryPending calls = %d, want 1", got)
	}
	if got := requested.calls; got != 1 || requested.last.EventID != "event-1" {
		t.Fatalf("requested publishes = %d (last %q), want 1 of event-1", requested.calls, requested.last.EventID)
	}
	if got := accepted.calls; got != 1 || accepted.last.EventID != "event-2" {
		t.Fatalf("accepted publishes = %d (last %q), want 1 of event-2", accepted.calls, accepted.last.EventID)
	}
}

func TestPublisherReconcileOnceQueryErrorSkipsPublish(t *testing.T) {
	store := &fakeStore{pendingErr: errors.New("db down")}
	p, requested, accepted := newTestPublisher(store)

	p.reconcileOnce()

	if got := store.pendingCalls; got != 1 {
		t.Fatalf("QueryPending calls = %d, want 1", got)
	}
	if got := requested.calls + accepted.calls; got != 0 {
		t.Fatalf("publish calls = %d, want 0 on query error", got)
	}
}

// --- onNotification e Close -------------------------------------------------

func TestPublisherOnNotificationDispatchesPublish(t *testing.T) {
	store := &fakeStore{
		row:      OutboxEvent{ID: "event-1", EventType: (rides.Requested{}).MessageType(), Payload: mustMarshal(t, rides.Requested{EventID: "event-1"})},
		rowFound: true,
	}
	p, requested, _ := newTestPublisher(store)

	p.onNotification("event-1")

	deadline := time.Now().Add(2 * time.Second)
	for {
		calls, last := requested.snapshot()
		if calls == 1 {
			if got := last.EventID; got != "event-1" {
				t.Fatalf("published EventID = %q, want event-1", got)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("requested publish calls = %d after timeout, want 1 (notification must dispatch publication)", calls)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestPublisherCloseIsIdempotent(t *testing.T) {
	p, _, _ := newTestPublisher(&fakeStore{})

	p.Close()
	p.Close() // must not panic or deadlock: closeOnce guards the teardown
}
