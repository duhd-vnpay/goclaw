package constraints

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/google/uuid"
)

type Violation struct {
	ID        uuid.UUID       `json:"id"`
	TenantID  uuid.UUID       `json:"tenant_id"`
	AgentID   uuid.UUID       `json:"agent_id"`
	SessionID *uuid.UUID      `json:"session_id,omitempty"`
	GuardName string          `json:"guard_name"`
	Phase     string          `json:"phase"`
	Kind      string          `json:"kind"`
	Action    string          `json:"action"`
	Feedback  string          `json:"feedback"`
	Context   json.RawMessage `json:"context"`
}

type ViolationStore struct {
	db *sql.DB
}

func NewViolationStore(db *sql.DB) *ViolationStore {
	return &ViolationStore{db: db}
}

func (s *ViolationStore) Record(ctx context.Context, v Violation) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO harness_constraint_violations
			(tenant_id, agent_id, session_id, guard_name, phase, kind, action, feedback, context)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		v.TenantID, v.AgentID, v.SessionID,
		v.GuardName, v.Phase, v.Kind, v.Action, v.Feedback, v.Context,
	)
	return err
}
