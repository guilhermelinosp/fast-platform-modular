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

const outboxChannel = "outbox_events"

// Publisher listens for committed outbox rows and publishes them to Kafka.
type Publisher struct {
	db           *database.DB
	requested    *kafka.Producer[rides.Requested]
	accepted     *kafka.Producer[rides.Accepted]
	listenerConn *database.Conn
	stopListen   func() error
	stopScan     chan struct{}
	workers      sync.WaitGroup
	closeOnce    sync.Once
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
	p := &Publisher{db: db, requested: requested, accepted: accepted, listenerConn: conn, stopScan: make(chan struct{})}
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

func (p *Publisher) publishByID(id string) {
	event, found, err := database.QueryRow[OutboxEvent](p.db, `UPDATE outbox_events SET attempts = attempts + 1 WHERE id = $1 AND published_at IS NULL AND next_attempt_at <= now() RETURNING id, event_type, event_version, payload`, id)
	if err != nil {
		slog.Default().Error("outbox claim failed", "error", err, "event_id", id)
		return
	}
	if !found {
		return
	}
	if err := p.publish(event); err != nil {
		if _, updateErr := p.db.Execute(`UPDATE outbox_events SET last_error = $2, next_attempt_at = now() + interval '5 seconds' WHERE id = $1 AND published_at IS NULL`, event.ID, err.Error()); updateErr != nil {
			slog.Default().Error("outbox failure update failed", "error", updateErr, "event_id", event.ID)
		}
		return
	}
	if _, err := p.db.Execute(`UPDATE outbox_events SET published_at = now(), last_error = NULL WHERE id = $1`, event.ID); err != nil {
		slog.Default().Error("outbox mark published failed", "error", err, "event_id", event.ID)
	}
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

func (p *Publisher) reconcileLoop() {
	defer p.workers.Done()
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			events, err := database.Query[OutboxEvent](p.db, `SELECT id, event_type, event_version, payload FROM outbox_events WHERE published_at IS NULL AND next_attempt_at <= now() ORDER BY occurred_at LIMIT 100`)
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
