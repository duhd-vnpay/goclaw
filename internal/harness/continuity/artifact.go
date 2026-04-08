package continuity

import (
	"time"

	"github.com/google/uuid"
)

// HandoffArtifact is the structured state saved at context boundaries.
type HandoffArtifact struct {
	ID              uuid.UUID  `json:"id"`
	TenantID        uuid.UUID  `json:"tenant_id"`
	AgentID         uuid.UUID  `json:"agent_id"`
	UserID          string     `json:"user_id"`
	SessionID       uuid.UUID  `json:"session_id"`
	PipelineID      *uuid.UUID `json:"pipeline_id,omitempty"`
	Sequence        int        `json:"sequence"`
	Objective       string     `json:"objective"`
	Progress        Progress   `json:"progress"`
	Decisions       []Decision `json:"decisions"`
	Artifacts       []FileRef  `json:"artifacts"`
	OpenQuestions   []string   `json:"open_questions"`
	GitBranch       string     `json:"git_branch,omitempty"`
	GitCommit       string     `json:"git_commit,omitempty"`
	Strategy        string     `json:"strategy"` // "manual","auto_reset","checkpoint_boundary"
	ContextUsagePct int        `json:"context_usage_pct"`
	CreatedAt       time.Time  `json:"created_at"`
}

// Progress tracks task completion state.
type Progress struct {
	CompletedTasks []string `json:"completed"`
	CurrentTask    string   `json:"current"`
	RemainingTasks []string `json:"remaining"`
	BlockedTasks   []string `json:"blocked,omitempty"`
	PercentDone    int      `json:"percent_done"`
}

// Decision records a choice made during a session.
type Decision struct {
	What string `json:"what"`
	Why  string `json:"why"`
	When string `json:"when"`
}

// FileRef references a file created or modified during a session.
type FileRef struct {
	Path        string `json:"path"`
	Action      string `json:"action"` // "created","modified","deleted"
	Description string `json:"description"`
}
