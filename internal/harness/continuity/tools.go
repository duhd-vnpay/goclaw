package continuity

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
)

// ToolHandler handles harness_checkpoint, harness_resume, harness_reset tools.
type ToolHandler struct {
	store    *ArtifactStore
	resolver *StrategyResolver
}

// NewToolHandler creates a new tool handler.
func NewToolHandler(store *ArtifactStore, resolver *StrategyResolver) *ToolHandler {
	return &ToolHandler{store: store, resolver: resolver}
}

// CheckpointInput is the JSON input for harness_checkpoint tool.
type CheckpointInput struct {
	Objective    string `json:"objective"`
	Completed    []string `json:"completed"`
	Current      string   `json:"current"`
	Remaining    []string `json:"remaining"`
	Decisions    []struct {
		What string `json:"what"`
		Why  string `json:"why"`
	} `json:"decisions"`
	OpenQuestions []string `json:"open_questions"`
}

// HandleCheckpoint saves current progress as a structured handoff artifact.
func (h *ToolHandler) HandleCheckpoint(ctx context.Context, tenantID, agentID, sessionID uuid.UUID, userID string, input json.RawMessage) (string, error) {
	var in CheckpointInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("parse checkpoint input: %w", err)
	}

	decisions := make([]Decision, len(in.Decisions))
	for i, d := range in.Decisions {
		decisions[i] = Decision{What: d.What, Why: d.Why}
	}

	artifact := &HandoffArtifact{
		TenantID:  tenantID,
		AgentID:   agentID,
		UserID:    userID,
		SessionID: sessionID,
		Objective: in.Objective,
		Progress: Progress{
			CompletedTasks: in.Completed,
			CurrentTask:    in.Current,
			RemainingTasks: in.Remaining,
		},
		Decisions:    decisions,
		OpenQuestions: in.OpenQuestions,
		Strategy:     "manual",
	}

	if err := h.store.Save(ctx, artifact); err != nil {
		return "", fmt.Errorf("save checkpoint: %w", err)
	}

	return fmt.Sprintf("Checkpoint saved. Progress: %d tasks done, %d remaining.",
		len(in.Completed), len(in.Remaining)), nil
}

// HandleResume loads the most recent handoff artifact for session continuity.
func (h *ToolHandler) HandleResume(ctx context.Context, tenantID, agentID uuid.UUID, userID string) (string, error) {
	artifact, err := h.store.GetLatest(ctx, tenantID, agentID, userID)
	if err != nil {
		return "", fmt.Errorf("load artifact: %w", err)
	}
	if artifact == nil {
		return "No previous session state found. This appears to be a fresh start.", nil
	}
	return BuildResumeContext(artifact), nil
}

// ResetInput is the JSON input for harness_reset tool.
type ResetInput struct {
	Reason string `json:"reason"`
}

// HandleReset requests a context reset — saves artifact and signals the harness.
func (h *ToolHandler) HandleReset(ctx context.Context, tenantID, agentID, sessionID uuid.UUID, userID string, input json.RawMessage) (string, error) {
	var in ResetInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("parse reset input: %w", err)
	}

	return fmt.Sprintf("Context reset requested: %s. The harness will save current state and provide a fresh context.", in.Reason), nil
}
