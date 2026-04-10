package tools

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/nextlevelbuilder/goclaw/internal/harness/continuity"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// HarnessCheckpointTool saves a checkpoint for the current session.
type HarnessCheckpointTool struct {
	handler *continuity.ToolHandler
}

func NewHarnessCheckpointTool(handler *continuity.ToolHandler) *HarnessCheckpointTool {
	return &HarnessCheckpointTool{handler: handler}
}

func (t *HarnessCheckpointTool) Name() string { return "harness_checkpoint" }

func (t *HarnessCheckpointTool) Description() string {
	return `Save a checkpoint of current progress for long-running tasks. ` +
		`Stores objective, completed tasks, current task, remaining tasks, decisions, and open questions.`
}

func (t *HarnessCheckpointTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"objective":     map[string]any{"type": "string", "description": "Overall objective of the task"},
			"completed":     map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "List of completed tasks"},
			"current":       map[string]any{"type": "string", "description": "Current task being worked on"},
			"remaining":     map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "List of remaining tasks"},
			"decisions":     map[string]any{"type": "array", "items": map[string]any{"type": "object", "properties": map[string]any{"what": map[string]any{"type": "string"}, "why": map[string]any{"type": "string"}}}, "description": "Key decisions made"},
			"open_questions": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Open questions that need answers"},
		},
		"required": []string{"objective", "completed", "current", "remaining"},
	}
}

func (t *HarnessCheckpointTool) Execute(ctx context.Context, args map[string]any) *Result {
	// Extract context values
	tenantID := store.TenantIDFromContext(ctx)
	agentID := store.AgentIDFromContext(ctx)
	sessionKey := ToolSessionKeyFromCtx(ctx)
	userID := store.UserIDFromContext(ctx)

	// Generate a session UUID from the session key
	sessionUUID := uuid.NewMD5(uuid.NameSpaceURL, []byte(sessionKey))

	// Convert args to JSON for the handler
	input, err := json.Marshal(args)
	if err != nil {
		return ErrorResult("failed to marshal checkpoint input: " + err.Error())
	}

	result, err := t.handler.HandleCheckpoint(ctx, tenantID, agentID, sessionUUID, userID, input)
	if err != nil {
		return ErrorResult("checkpoint failed: " + err.Error())
	}
	return NewResult(result)
}

// HarnessResumeTool loads the most recent checkpoint for session continuity.
type HarnessResumeTool struct {
	handler *continuity.ToolHandler
}

func NewHarnessResumeTool(handler *continuity.ToolHandler) *HarnessResumeTool {
	return &HarnessResumeTool{handler: handler}
}

func (t *HarnessResumeTool) Name() string { return "harness_resume" }

func (t *HarnessResumeTool) Description() string {
	return `Resume from a previously saved checkpoint. Loads the most recent handoff artifact for this user.`
}

func (t *HarnessResumeTool) Parameters() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
}

func (t *HarnessResumeTool) Execute(ctx context.Context, args map[string]any) *Result {
	tenantID := store.TenantIDFromContext(ctx)
	agentID := store.AgentIDFromContext(ctx)
	userID := store.UserIDFromContext(ctx)

	result, err := t.handler.HandleResume(ctx, tenantID, agentID, userID)
	if err != nil {
		return ErrorResult("resume failed: " + err.Error())
	}
	return NewResult(result)
}

// HarnessResetTool clears all checkpoints for the current session.
type HarnessResetTool struct {
	handler *continuity.ToolHandler
}

func NewHarnessResetTool(handler *continuity.ToolHandler) *HarnessResetTool {
	return &HarnessResetTool{handler: handler}
}

func (t *HarnessResetTool) Name() string { return "harness_reset" }

func (t *HarnessResetTool) Description() string {
	return `Clear all checkpoints for the current session. Use when starting a completely new task.`
}

func (t *HarnessResetTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"reason": map[string]any{"type": "string", "description": "Reason for resetting the context"},
		},
		"required": []string{"reason"},
	}
}

func (t *HarnessResetTool) Execute(ctx context.Context, args map[string]any) *Result {
	tenantID := store.TenantIDFromContext(ctx)
	agentID := store.AgentIDFromContext(ctx)
	sessionKey := ToolSessionKeyFromCtx(ctx)
	userID := store.UserIDFromContext(ctx)

	reason, _ := args["reason"].(string)
	input, _ := json.Marshal(map[string]string{"reason": reason})

	sessionUUID := uuid.NewMD5(uuid.NameSpaceURL, []byte(sessionKey))
	result, err := t.handler.HandleReset(ctx, tenantID, agentID, sessionUUID, userID, input)
	if err != nil {
		return ErrorResult("reset failed: " + err.Error())
	}
	return NewResult(result)
}
