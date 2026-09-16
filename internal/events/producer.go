package events

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/guilhermelinosp/fast-platform-modular/internal/rides"
	"github.com/guilhermelinosp/hellnet-lib-database/database"
	"github.com/guilhermelinosp/hellnet-lib-kafka/kafka"
)

const (
	outboxChannel  = "outbox_events"
	memoMaxEvents  = 4096
	outboxReloadAt = 5 * time.Second
)

// Publisher listens for committed outbox rows and publishes them to Kafka.
//
// The outbox table is append-only from the application's point of view:
// producers INSERT rows, and this publisher only SELECTs them. The publisher
// never mutates a row (no UPDATE/DELETE), so delivery state — already
// published, in-flight, or retryable — lives entirely in memory and resets on
// restart. This yields at-least-once semantics: a duplicate publish is
// possible after a crash or once an id ages out of the in-memory memo.
type Publisher struct {
	db           *database.DB
	requested    *kafka.Producer[rides.Requested]
	accepted     *kafka.Producer[rides.Accepted]
	listenerConn *database.Conn
	stopListen   func() error
	stopScan     chan struct{}
	workers      sync.WaitGroup
	closeOnce    sync.Once

	memo *outboxMemo
}

// OutboxEvent is the durable event envelope stored in PostgreSQL.
type OutboxEvent struct {
	ID           string `db:"id"`
	EventType    string `db:"event_type"`
	EventVersion int    `db:"event_version"`
	Payload      []byte `db:"payload"`
}

// NewPublisher starts a PostgreSQL listener. Database schema changes are
// managed externally by sql/outbox_listener.sql.
func NewPublisher(db *database.DB, requested *kafka.Producer[rides.Requested], accepted *kafka.Producer[rides.Accepted]) (*Publisher, error) {
	conn, err := db.Acquire()
	if err != nil {
		return nil, fmt.Errorf("acquire outbox listener connection: %w", err)
	}
	p := &Publisher{
		db:           db,
		requested:    requested,
		accepted:     accepted,
		listenerConn: conn,
		stopScan:     make(chan struct{}),
		memo:         newOutboxMemo(),
	}
	stop, err := conn.ListenWithReconnect(outboxChannel, p.onNotification, database.ListenOptions{})
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("listen for outbox events: %w", err)
	}
	p.stopListen = stop
	p.workers.Add(1)
	go p.reconcileLoop()
	return p, nil
}

// Requested is retained as the service port; the committed outbox row wakes
// the listener through PostgreSQL NOTIFY.
func (p *Publisher) Requested(_ rides.Requested) error { return nil }

// Accepted is retained as the service port; the committed outbox row wakes
// the listener through PostgreSQL NOTIFY.
func (p *Publisher) Accepted(_ rides.Accepted) error { return nil }

func (p *Publisher) onNotification(id string) {
	p.workers.Go(func() { p.publishByID(id) })
}

// publishByID reads one pending outbox row and publishes it. No row is ever
// written back: claims, retries and publication tracking are in-memory only.
func (p *Publisher) publishByID(id string) {
	if !p.memo.start(id) {
		return
	}
	event, found, err := database.QueryRow[OutboxEvent](p.db, `SELECT id, event_type, event_version, payload FROM outbox_events WHERE id = $1`, id)
	if err != nil {
		p.memo.release(id)
		slog.Default().Error("outbox select failed", "error", err, "event_id", id)
		return
	}
	if !found {
		p.memo.release(id)
		return
	}
	if err := p.publish(event); err != nil {
		p.memo.release(id)
		slog.Default().Error("outbox publish failed; will retry on next reconcile", "error", err, "event_id", id)
		return
	}
	p.memo.settle(id)
}

func (p *Publisher) publish(event OutboxEvent) error {
	switch event.EventType {
	case (rides.Requested{}).MessageType():
		var message rides.Requested
		if err := json.Unmarshal(event.Payload, &message); err != nil {
			return fmt.Errorf("decode requested event: %w", err)
		}
		return p.requested.Publish(message)
	case (rides.Accepted{}).MessageType():
		var message rides.Accepted
		if err := json.Unmarshal(event.Payload, &message); err != nil {
			return fmt.Errorf("decode accepted event: %w", err)
		}
		return p.accepted.Publish(message)
	default:
		return fmt.Errorf("unsupported outbox event type %q", event.EventType)
	}
}

// reconcileLoop is the safety net for rows inserted while this process was
// down or whose publish failed: it reselects pending rows on a fixed cadence
// and relies on the memo to skip already-published ids.
func (p *Publisher) reconcileLoop() {
	defer p.workers.Done()
	ticker := time.NewTicker(outboxReloadAt)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			events, err := database.Query[OutboxEvent](p.db, `SELECT id, event_type, event_version, payload FROM outbox_events WHERE published_at IS NULL ORDER BY occurred_at LIMIT 100`)
			if err != nil {
				slog.Default().Error("outbox reconciliation failed", "error", err)
				continue
			}
			for _, event := range events {
				p.publishByID(event.ID)
			}
		case <-p.stopScan:
			return
		}
	}
}

// Close stops the listener and waits for in-flight publications.
func (p *Publisher) Close() {
	p.closeOnce.Do(func() {
		if p.stopListen != nil {
			if err := p.stopListen(); err != nil {
				slog.Default().Warn("stop outbox listener failed", "error", err)
			}
		}
		close(p.stopScan)
		p.workers.Wait()
		if p.listenerConn != nil {
			_ = p.listenerConn.Close()
		}
	})
}

// outboxMemo is the in-memory delivery state for the insert/select-only
// outbox. It prevents duplicate publication of the same id when the NOTIFY
// handler and the reconcile loop race, and it stops the reconcile loop from
// republishing settled events on every tick. It is bounded and loses state on
// restart, so the outbox remains at-least-once.
type outboxMemo struct {
	mu       sync.Mutex
	inflight map[string]struct{}
	done     map[string]struct{}
	order    []string // FIFO of done ids, for bounded eviction
}

func newOutboxMemo() *outboxMemo {
	return &outboxMemo{
		inflight: make(map[string]struct{}),
		done:     make(map[string]struct{}),
	}
}

// start claims id for publication. It is false when the event was already
// published, is currently being published, or is inside its retry window.
func (m *outboxMemo) start(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.done[id]; ok {
		return false
	}
	if _, ok := m.inflight[id]; ok {
		return false
	}
	m.inflight[id] = struct{}{}
	return true
}

// release abandons a failed attempt, allowing the next reconcile tick to
// retry the event.
func (m *outboxMemo) release(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.inflight, id)
}

// settle marks a successfully published event as settled so later selects
// skip it. The memo is bounded: the oldest settled ids are forgotten, which
// can cause a duplicate publish for very old events (accepted at-least-once
// behaviour).
func (m *outboxMemo) settle(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.inflight, id)
	if _, ok := m.done[id]; ok {
		return
	}
	m.done[id] = struct{}{}
	m.order = append(m.order, id)
	for len(m.order) > memoMaxEvents {
		oldest := m.order[0]
		m.order = m.order[1:]
		delete(m.done, oldest)
	}
}
