package outbox

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

func TestProducerPortsAreNoOps(t *testing.T) {
	producer := &Producer{}

	if err := producer.Publish(Event{}); err == nil || !strings.Contains(err.Error(), "unsupported outbox event type") {
		t.Fatalf("Publish() = %v, want unsupported outbox event type error", err)
	}
}

func TestProducerPublishRejectsUnsupportedEventType(t *testing.T) {
	producer := &Producer{}

	err := producer.Publish(Event{ID: "event-1", EventType: "rides.deleted.v9", Payload: []byte(`{}`)})
	if err == nil || !strings.Contains(err.Error(), "unsupported outbox event type") {
		t.Fatalf("Publish() = %v, want unsupported outbox event type error", err)
	}
}

func TestProducerPublishRejectsMalformedPayload(t *testing.T) {
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
			producer := &Producer{}
			err := producer.Publish(Event{ID: "event-1", EventType: tt.eventType, Payload: []byte(`{"broken"`)})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Publish() = %v, want error containing %q", err, tt.want)
			}
		})
	}
}

// --- fakes -----------------------------------------------------------------

type fakeStore struct {
	row          Event
	rowFound     bool
	rowErr       error
	rows         []Event
	pendingErr   error
	rowCalls     int
	pendingCalls int
	published    []string
	failures     []string
	auditErr     error
}

func (f *fakeStore) QueryRow(id string) (Event, bool, error) {
	f.rowCalls++
	if f.rowErr != nil {
		return Event{}, false, f.rowErr
	}
	for _, e := range f.rows {
		if e.ID == id {
			return e, true, nil
		}
	}
	return f.row, f.rowFound, nil
}

func (f *fakeStore) QueryPending() ([]Event, error) {
	f.pendingCalls++
	return f.rows, f.pendingErr
}

func (f *fakeStore) RecordPublished(id string) error {
	f.published = append(f.published, id)
	return f.auditErr
}

func (f *fakeStore) RecordFailure(id string, reason string) error {
	f.failures = append(f.failures, id)
	return f.auditErr
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

func newTestListener(store outboxStore, publisher eventPublisher) *Listener {
	return &Listener{
		store:     store,
		publisher: publisher,
		stopScan:  make(chan struct{}),
		memo:      newOutboxMemo(),
	}
}

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return b
}

// --- Listener reconcileOnce --------------------------------------------------

func TestListenerReconcileOncePublishesPending(t *testing.T) {
	store := &fakeStore{
		rows: []Event{
			{ID: "event-1", EventType: (rides.Requested{}).MessageType(), Payload: mustMarshal(t, rides.Requested{EventID: "event-1"})},
			{ID: "event-2", EventType: (rides.Accepted{}).MessageType(), Payload: mustMarshal(t, rides.Accepted{EventID: "event-2"})},
		},
	}
	requested := &fakeRequestedPublisher{}
	accepted := &fakeAcceptedPublisher{}
	producer := &Producer{requested: requested, accepted: accepted}
	listener := newTestListener(store, producer)

	listener.reconcileOnce()

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

func TestListenerReconcileOnceQueryErrorSkipsPublish(t *testing.T) {
	store := &fakeStore{pendingErr: errors.New("db down")}
	requested := &fakeRequestedPublisher{}
	accepted := &fakeAcceptedPublisher{}
	producer := &Producer{requested: requested, accepted: accepted}
	listener := newTestListener(store, producer)

	listener.reconcileOnce()

	if got := store.pendingCalls; got != 1 {
		t.Fatalf("QueryPending calls = %d, want 1", got)
	}
	if got := requested.calls + accepted.calls; got != 0 {
		t.Fatalf("publish calls = %d, want 0 on query error", got)
	}
}

func TestListenerReconcileOnceSkipsSettledEvents(t *testing.T) {
	store := &fakeStore{
		rows: []Event{
			{ID: "event-1", EventType: (rides.Requested{}).MessageType(), Payload: mustMarshal(t, rides.Requested{EventID: "event-1"})},
		},
	}
	requested := &fakeRequestedPublisher{}
	accepted := &fakeAcceptedPublisher{}
	producer := &Producer{requested: requested, accepted: accepted}
	listener := newTestListener(store, producer)

	listener.reconcileOnce()
	listener.reconcileOnce()

	if got := requested.calls; got != 1 {
		t.Fatalf("requested publish calls = %d, want 1 (settled event must not republish)", got)
	}
	if got := store.pendingCalls; got != 2 {
		t.Fatalf("QueryPending calls = %d, want 2", got)
	}
}

func TestListenerReconcileOnceRetriesAfterPublishFailure(t *testing.T) {
	store := &fakeStore{
		rows: []Event{
			{ID: "event-1", EventType: (rides.Requested{}).MessageType(), Payload: mustMarshal(t, rides.Requested{EventID: "event-1"})},
		},
	}
	requested := &fakeRequestedPublisher{err: errors.New("kafka down")}
	accepted := &fakeAcceptedPublisher{}
	producer := &Producer{requested: requested, accepted: accepted}
	listener := newTestListener(store, producer)

	listener.reconcileOnce()
	if got := requested.calls; got != 1 {
		t.Fatalf("requested publish calls = %d, want 1", got)
	}

	requested.err = nil
	listener.reconcileOnce()
	if got := requested.calls; got != 2 {
		t.Fatalf("requested publish calls = %d, want 2 (failed event must be retried)", got)
	}
}

func TestListenerOnNotificationDispatchesPublish(t *testing.T) {
	store := &fakeStore{
		row:      Event{ID: "event-1", EventType: (rides.Requested{}).MessageType(), Payload: mustMarshal(t, rides.Requested{EventID: "event-1"})},
		rowFound: true,
	}
	requested := &fakeRequestedPublisher{}
	accepted := &fakeAcceptedPublisher{}
	producer := &Producer{requested: requested, accepted: accepted}
	listener := newTestListener(store, producer)

	listener.onNotification("event-1")

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

func TestListenerCloseIsIdempotent(t *testing.T) {
	store := &fakeStore{}
	requested := &fakeRequestedPublisher{}
	accepted := &fakeAcceptedPublisher{}
	producer := &Producer{requested: requested, accepted: accepted}
	listener := newTestListener(store, producer)

	listener.Close()
	listener.Close() // must not panic or deadlock: closeOnce guards the teardown
}
