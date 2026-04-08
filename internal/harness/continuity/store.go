package continuity

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/google/uuid"
)

// ArtifactStore persists handoff artifacts to PostgreSQL.
type ArtifactStore struct {
	db *sql.DB
}

// NewArtifactStore creates a new store backed by the given database.
func NewArtifactStore(db *sql.DB) *ArtifactStore {
	return &ArtifactStore{db: db}
}

// Save persists a handoff artifact.
func (s *ArtifactStore) Save(ctx context.Context, a *HandoffArtifact) error {
	progress, _ := json.Marshal(a.Progress)
	decisions, _ := json.Marshal(a.Decisions)
	artifacts, _ := json.Marshal(a.Artifacts)
	questions, _ := json.Marshal(a.OpenQuestions)

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO harness_handoff_artifacts
			(tenant_id, agent_id, user_id, session_id, pipeline_id, sequence,
			 objective, progress, decisions, artifacts, open_questions,
			 git_branch, git_commit, strategy, context_usage_pct)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`,
		a.TenantID, a.AgentID, a.UserID, a.SessionID, a.PipelineID, a.Sequence,
		a.Objective, progress, decisions, artifacts, questions,
		a.GitBranch, a.GitCommit, a.Strategy, a.ContextUsagePct,
	)
	return err
}

// GetLatest returns the most recent handoff artifact for the given agent+user.
// Returns nil, nil if no artifact exists.
func (s *ArtifactStore) GetLatest(ctx context.Context, tenantID, agentID uuid.UUID, userID string) (*HandoffArtifact, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, tenant_id, agent_id, user_id, session_id, pipeline_id, sequence,
				objective, progress, decisions, artifacts, open_questions,
				git_branch, git_commit, strategy, context_usage_pct, created_at
		 FROM harness_handoff_artifacts
		 WHERE tenant_id = $1 AND agent_id = $2 AND user_id = $3
		 ORDER BY created_at DESC LIMIT 1`,
		tenantID, agentID, userID,
	)

	var a HandoffArtifact
	var progressJSON, decisionsJSON, artifactsJSON, questionsJSON []byte
	err := row.Scan(
		&a.ID, &a.TenantID, &a.AgentID, &a.UserID, &a.SessionID, &a.PipelineID, &a.Sequence,
		&a.Objective, &progressJSON, &decisionsJSON, &artifactsJSON, &questionsJSON,
		&a.GitBranch, &a.GitCommit, &a.Strategy, &a.ContextUsagePct, &a.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	json.Unmarshal(progressJSON, &a.Progress)
	json.Unmarshal(decisionsJSON, &a.Decisions)
	json.Unmarshal(artifactsJSON, &a.Artifacts)
	json.Unmarshal(questionsJSON, &a.OpenQuestions)

	return &a, nil
}
