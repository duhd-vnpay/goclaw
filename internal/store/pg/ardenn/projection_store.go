package ardenn

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

type PGProjectionStore struct {
	db *sqlx.DB
}

func NewPGProjectionStore(db *sqlx.DB) *PGProjectionStore {
	return &PGProjectionStore{db: db}
}

type RunRow struct {
	ID           uuid.UUID       `db:"id"`
	TenantID     uuid.UUID       `db:"tenant_id"`
	WorkflowID   uuid.UUID       `db:"workflow_id"`
	ProjectID    *uuid.UUID      `db:"project_id"`
	TriggeredBy  *uuid.UUID      `db:"triggered_by"`
	Variables    json.RawMessage `db:"variables"`
	Tier         string          `db:"tier"`
	Status       string          `db:"status"`
	LastSequence int64           `db:"last_sequence"`
}

type StepRunRow struct {
	ID            uuid.UUID       `db:"id"`
	RunID         uuid.UUID       `db:"run_id"`
	StepID        uuid.UUID       `db:"step_id"`
	Status        string          `db:"status"`
	AssignedUser  *uuid.UUID      `db:"assigned_user"`
	AssignedAgent *uuid.UUID      `db:"assigned_agent"`
	HandType      *string         `db:"hand_type"`
	Result        *string         `db:"result"`
	DispatchCount int             `db:"dispatch_count"`
	EvalRound     int             `db:"eval_round"`
	EvalScore     *float64        `db:"eval_score"`
	EvalPassed    *bool           `db:"eval_passed"`
	GateStatus    *string         `db:"gate_status"`
	GateDecidedBy *uuid.UUID      `db:"gate_decided_by"`
	Metadata      json.RawMessage `db:"metadata"`
	LastSequence  int64           `db:"last_sequence"`
}

func (s *PGProjectionStore) UpsertRun(ctx context.Context, r RunRow) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO ardenn_runs (id, tenant_id, workflow_id, project_id, triggered_by, variables, tier, status, last_sequence)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		 ON CONFLICT (id) DO UPDATE SET
		   status = EXCLUDED.status,
		   last_sequence = EXCLUDED.last_sequence,
		   updated_at = NOW()`,
		r.ID, r.TenantID, r.WorkflowID, r.ProjectID, r.TriggeredBy,
		r.Variables, r.Tier, r.Status, r.LastSequence)
	if err != nil {
		return fmt.Errorf("upsert run: %w", err)
	}
	return nil
}

func (s *PGProjectionStore) UpsertStepRun(ctx context.Context, sr StepRunRow) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO ardenn_step_runs (id, run_id, step_id, status, assigned_user, assigned_agent,
		   hand_type, result, dispatch_count, eval_round, eval_score, eval_passed,
		   gate_status, gate_decided_by, metadata, last_sequence)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
		 ON CONFLICT (run_id, step_id) DO UPDATE SET
		   status = EXCLUDED.status, assigned_user = EXCLUDED.assigned_user,
		   assigned_agent = EXCLUDED.assigned_agent, hand_type = EXCLUDED.hand_type,
		   result = EXCLUDED.result, dispatch_count = EXCLUDED.dispatch_count,
		   eval_round = EXCLUDED.eval_round, eval_score = EXCLUDED.eval_score,
		   eval_passed = EXCLUDED.eval_passed, gate_status = EXCLUDED.gate_status,
		   gate_decided_by = EXCLUDED.gate_decided_by, metadata = EXCLUDED.metadata,
		   last_sequence = EXCLUDED.last_sequence, updated_at = NOW()`,
		sr.ID, sr.RunID, sr.StepID, sr.Status, sr.AssignedUser, sr.AssignedAgent,
		sr.HandType, sr.Result, sr.DispatchCount, sr.EvalRound, sr.EvalScore,
		sr.EvalPassed, sr.GateStatus, sr.GateDecidedBy, sr.Metadata, sr.LastSequence)
	if err != nil {
		return fmt.Errorf("upsert step run: %w", err)
	}
	return nil
}

func (s *PGProjectionStore) GetActiveRuns(ctx context.Context, tenantID uuid.UUID) ([]RunRow, error) {
	var runs []RunRow
	err := s.db.SelectContext(ctx, &runs,
		`SELECT * FROM ardenn_runs WHERE tenant_id = $1 AND status IN ('running', 'paused')
		 ORDER BY created_at`, tenantID)
	return runs, err
}

func (s *PGProjectionStore) GetStepRuns(ctx context.Context, runID uuid.UUID) ([]StepRunRow, error) {
	var stepRuns []StepRunRow
	err := s.db.SelectContext(ctx, &stepRuns,
		`SELECT * FROM ardenn_step_runs WHERE run_id = $1 ORDER BY created_at`, runID)
	return stepRuns, err
}

// --- Query methods for gateway RPC ---

type ListRunsFilter struct {
	WorkflowID *uuid.UUID
	Status     *string
	Limit      int
	Offset     int
}

func (s *PGProjectionStore) ListRuns(ctx context.Context, tenantID uuid.UUID, f ListRunsFilter) ([]RunRow, error) {
	query := `SELECT * FROM ardenn_runs WHERE tenant_id = $1`
	args := []any{tenantID}
	idx := 2

	if f.WorkflowID != nil {
		query += fmt.Sprintf(" AND workflow_id = $%d", idx)
		args = append(args, *f.WorkflowID)
		idx++
	}
	if f.Status != nil {
		query += fmt.Sprintf(" AND status = $%d", idx)
		args = append(args, *f.Status)
		idx++
	}
	query += " ORDER BY created_at DESC"
	if f.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", f.Limit)
	}
	if f.Offset > 0 {
		query += fmt.Sprintf(" OFFSET %d", f.Offset)
	}

	var runs []RunRow
	err := s.db.SelectContext(ctx, &runs, query, args...)
	return runs, err
}

type MyTask struct {
	StepRunID    uuid.UUID `db:"id"              json:"id"`
	RunID        uuid.UUID `db:"run_id"          json:"runId"`
	StepID       uuid.UUID `db:"step_id"         json:"stepId"`
	StepName     string    `db:"step_name"       json:"stepName"`
	WorkflowName string   `db:"workflow_name"    json:"workflowName"`
	WorkflowID   uuid.UUID `db:"workflow_id"     json:"workflowId"`
	Status       string    `db:"status"          json:"status"`
	GateStatus   *string   `db:"gate_status"     json:"gateStatus"`
	HandType     *string   `db:"hand_type"       json:"handType"`
	CreatedAt    string    `db:"created_at"      json:"createdAt"`
}

func (s *PGProjectionStore) GetMyTasks(ctx context.Context, tenantID uuid.UUID, userID string) ([]MyTask, error) {
	var tasks []MyTask
	err := s.db.SelectContext(ctx, &tasks,
		`SELECT sr.id, sr.run_id, sr.step_id, st.name AS step_name,
		        w.name AS workflow_name, w.id AS workflow_id,
		        sr.status, sr.gate_status, sr.hand_type, sr.created_at
		 FROM ardenn_step_runs sr
		 JOIN ardenn_steps st ON sr.step_id = st.id
		 JOIN ardenn_runs r ON sr.run_id = r.id
		 JOIN ardenn_workflows w ON r.workflow_id = w.id
		 WHERE r.tenant_id = $1
		   AND (sr.assigned_user = $2::uuid OR (sr.gate_status = 'pending'))
		   AND sr.status IN ('running', 'waiting_gate')
		 ORDER BY sr.created_at DESC`,
		tenantID, userID)
	return tasks, err
}
