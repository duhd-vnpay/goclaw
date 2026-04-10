package ardenn

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// ArdennArtifact is a checkpoint snapshot saved after step completion.
type ArdennArtifact struct {
	ID        uuid.UUID      `json:"id"`
	TenantID  uuid.UUID      `json:"tenant_id"`
	RunID     uuid.UUID      `json:"run_id"`
	StepID    uuid.UUID      `json:"step_id"`
	Output    string         `json:"output"`
	Variables map[string]any `json:"variables"`
	Sequence  int64          `json:"sequence"`
	CreatedAt time.Time      `json:"created_at"`
}

// ArdennArtifactStore persists checkpoint artifacts.
type ArdennArtifactStore interface {
	Save(ctx context.Context, a *ArdennArtifact) error
	GetLatest(ctx context.Context, runID, stepID uuid.UUID) (*ArdennArtifact, error)
}

// Checkpointer manages L2 checkpoint save/resume for workflow steps.
type Checkpointer struct {
	events    EventStore
	artifacts ArdennArtifactStore
}

func NewCheckpointer(events EventStore, artifacts ArdennArtifactStore) *Checkpointer {
	return &Checkpointer{events: events, artifacts: artifacts}
}

// Checkpoint saves a snapshot after step completion and emits continuity.checkpoint.
func (c *Checkpointer) Checkpoint(ctx context.Context, run *RunState, step *StepDef, output string) error {
	artifact := &ArdennArtifact{
		ID:        uuid.New(),
		TenantID:  run.TenantID,
		RunID:     run.ID,
		StepID:    step.ID,
		Output:    output,
		Variables: run.Variables,
		Sequence:  run.LastSequence,
		CreatedAt: time.Now(),
	}

	if err := c.artifacts.Save(ctx, artifact); err != nil {
		return err
	}

	stepID := step.ID
	c.events.Emit(ctx, Event{
		TenantID:  run.TenantID,
		RunID:     run.ID,
		StepID:    &stepID,
		Type:      EventContinuityCheckpoint,
		ActorType: ActorEngine,
		Payload: map[string]any{
			"artifact_id": artifact.ID.String(),
			"sequence":    artifact.Sequence,
		},
	})

	return nil
}

// Resume loads the latest checkpoint and returns events since that point.
func (c *Checkpointer) Resume(ctx context.Context, runID, stepID uuid.UUID) (*ArdennArtifact, []Event, error) {
	artifact, err := c.artifacts.GetLatest(ctx, runID, stepID)
	if err != nil {
		return nil, nil, err
	}
	if artifact == nil {
		return nil, nil, nil
	}

	events, err := c.events.GetEvents(ctx, EventQuery{
		RunID:        runID,
		FromSequence: artifact.Sequence + 1,
	})
	if err != nil {
		return artifact, nil, err
	}

	c.events.Emit(ctx, Event{
		TenantID:  artifact.TenantID,
		RunID:     runID,
		StepID:    &stepID,
		Type:      EventContinuityResume,
		ActorType: ActorEngine,
		Payload: map[string]any{
			"artifact_id":   artifact.ID.String(),
			"from_sequence": artifact.Sequence,
			"events_since":  len(events),
		},
	})

	return artifact, events, nil
}
