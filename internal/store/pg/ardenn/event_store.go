package ardenn

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	engine "github.com/nextlevelbuilder/goclaw/internal/ardenn"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

type PGEventStore struct {
	db *sqlx.DB
}

func NewPGEventStore(db *sqlx.DB) *PGEventStore {
	return &PGEventStore{db: db}
}

func (s *PGEventStore) Emit(ctx context.Context, e engine.Event) (int64, error) {
	if e.ID == uuid.Nil {
		e.ID = uuid.New()
	}
	payload, err := json.Marshal(e.Payload)
	if err != nil {
		return 0, fmt.Errorf("marshal payload: %w", err)
	}

	var seq int64
	err = s.db.QueryRowContext(ctx,
		`INSERT INTO ardenn_events (id, tenant_id, run_id, step_id, event_type, actor_type, actor_id, payload)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 RETURNING sequence`,
		e.ID, e.TenantID, e.RunID, e.StepID, e.Type, e.ActorType, e.ActorID, payload,
	).Scan(&seq)
	if err != nil {
		return 0, fmt.Errorf("insert event: %w", err)
	}
	return seq, nil
}

func (s *PGEventStore) GetEvents(ctx context.Context, q engine.EventQuery) ([]engine.Event, error) {
	var conditions []string
	var args []any
	argN := 1

	conditions = append(conditions, fmt.Sprintf("run_id = $%d", argN))
	args = append(args, q.RunID)
	argN++

	if q.FromSequence > 0 {
		conditions = append(conditions, fmt.Sprintf("sequence > $%d", argN))
		args = append(args, q.FromSequence)
		argN++
	}

	if q.StepID != nil {
		conditions = append(conditions, fmt.Sprintf("step_id = $%d", argN))
		args = append(args, *q.StepID)
		argN++
	}

	if q.EventType != "" {
		conditions = append(conditions, fmt.Sprintf("event_type LIKE $%d", argN))
		args = append(args, q.EventType+"%")
		argN++
	}

	query := fmt.Sprintf(
		`SELECT id, tenant_id, run_id, step_id, sequence, event_type, actor_type, actor_id, payload, created_at
		 FROM ardenn_events WHERE %s ORDER BY sequence ASC`,
		strings.Join(conditions, " AND "),
	)

	if q.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", q.Limit)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query events: %w", err)
	}
	defer rows.Close()

	var events []engine.Event
	for rows.Next() {
		var e engine.Event
		var payloadBytes []byte
		if err := rows.Scan(&e.ID, &e.TenantID, &e.RunID, &e.StepID, &e.Sequence,
			&e.Type, &e.ActorType, &e.ActorID, &payloadBytes, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		if err := json.Unmarshal(payloadBytes, &e.Payload); err != nil {
			e.Payload = map[string]any{}
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

func (s *PGEventStore) GetLastSequence(ctx context.Context, runID uuid.UUID) (int64, error) {
	var seq int64
	err := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(sequence), 0) FROM ardenn_events WHERE run_id = $1`,
		runID,
	).Scan(&seq)
	return seq, err
}
