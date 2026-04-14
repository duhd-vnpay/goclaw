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

// InFlightRun is a lightweight summary of a run that needs startup recovery.
type InFlightRun struct {
	RunID      uuid.UUID
	WorkflowID uuid.UUID
	TenantID   uuid.UUID
}

// GetInFlightRuns returns runs that have been started but not yet reached a
// terminal state (completed/failed/cancelled). Used for startup recovery to
// re-register step defs and wake parked runs after a pod restart.
//
// Uses the event log as source of truth: a run is in-flight if the latest
// run.* event for it is not terminal. The workflow_id is extracted from the
// run.created event payload.
func (s *PGEventStore) GetInFlightRuns(ctx context.Context) ([]InFlightRun, error) {
	rows, err := s.db.QueryContext(ctx, `
		WITH run_states AS (
			SELECT
				run_id,
				tenant_id,
				MAX(CASE WHEN event_type = 'run.created' THEN payload->>'workflow_id' END) as workflow_id,
				MAX(CASE WHEN event_type LIKE 'run.%' THEN event_type END) as last_run_event
			FROM ardenn_events
			GROUP BY run_id, tenant_id
		)
		SELECT run_id, tenant_id, workflow_id
		FROM run_states
		WHERE last_run_event NOT IN ('run.completed', 'run.failed', 'run.cancelled')
		  AND workflow_id IS NOT NULL
		ORDER BY run_id
	`)
	if err != nil {
		return nil, fmt.Errorf("query in-flight runs: %w", err)
	}
	defer rows.Close()

	var result []InFlightRun
	for rows.Next() {
		var r InFlightRun
		var workflowIDStr string
		if err := rows.Scan(&r.RunID, &r.TenantID, &workflowIDStr); err != nil {
			return nil, fmt.Errorf("scan in-flight run: %w", err)
		}
		wid, err := uuid.Parse(workflowIDStr)
		if err != nil {
			continue // skip malformed
		}
		r.WorkflowID = wid
		result = append(result, r)
	}
	return result, rows.Err()
}
