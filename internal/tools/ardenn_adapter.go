package tools

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync/atomic"

	"github.com/google/uuid"
)

// ardennInterceptedActions are the actions routed through Ardenn when enabled.
var ardennInterceptedActions = map[string]bool{
	"create":   true,
	"complete": true,
	"cancel":   true,
}

// ArdennWorkflowEngine is the minimal interface the adapter needs from Ardenn.
// Defined here to avoid circular imports with internal/ardenn.
type ArdennWorkflowEngine interface {
	StartRun(ctx context.Context, req ArdennAdapterStartReq) (uuid.UUID, error)
	Wake(ctx context.Context, runID uuid.UUID) error
}

// ArdennAdapterStartReq is the adapter's view of a run start request.
type ArdennAdapterStartReq struct {
	TenantID    uuid.UUID
	TeamID      uuid.UUID
	Subject     string
	Description string
	Assignee    string
	Priority    int
	BlockedBy   []uuid.UUID
	Variables   map[string]any
}

// shadowMetrics tracks shadow mode comparison results for observability.
type shadowMetrics struct {
	matches       atomic.Int64
	discrepancies atomic.Int64
}

// ArdennTeamTasksAdapter wraps a team_tasks Tool and routes lifecycle actions
// through the Ardenn engine when enabled for the team.
type ArdennTeamTasksAdapter struct {
	wrapped    Tool // interface for testability; production callers pass *TeamTasksTool
	engine     ArdennWorkflowEngine
	shadowMode bool
	metrics    shadowMetrics
}

// NewArdennTeamTasksAdapter creates an adapter. If engine is nil, all calls pass through.
func NewArdennTeamTasksAdapter(wrapped Tool, engine ArdennWorkflowEngine) *ArdennTeamTasksAdapter {
	return &ArdennTeamTasksAdapter{
		wrapped: wrapped,
		engine:  engine,
	}
}

// ---------------------------------------------------------------------------
// Tool interface delegation
// ---------------------------------------------------------------------------

func (a *ArdennTeamTasksAdapter) Name() string               { return a.wrapped.Name() }
func (a *ArdennTeamTasksAdapter) Description() string        { return a.wrapped.Description() }
func (a *ArdennTeamTasksAdapter) Parameters() map[string]any { return a.wrapped.Parameters() }

// Execute routes lifecycle actions through Ardenn when enabled, otherwise
// delegates to the wrapped tool.
func (a *ArdennTeamTasksAdapter) Execute(ctx context.Context, args map[string]any) *Result {
	action, _ := args["action"].(string)

	// Check if Ardenn is enabled for this team
	if !a.isArdennEnabled(ctx) || a.engine == nil {
		return a.wrapped.Execute(ctx, args)
	}

	// Only intercept lifecycle actions
	if !ardennInterceptedActions[action] {
		return a.wrapped.Execute(ctx, args)
	}

	// Shadow mode: run both, compare, return original
	if a.shadowMode || a.isShadowEnabled(ctx) || isShadowGlobal() {
		return a.executeShadow(ctx, action, args)
	}

	// Ardenn-routed execution
	switch action {
	case "create":
		return a.ardennCreate(ctx, args)
	case "complete":
		return a.ardennComplete(ctx, args)
	case "cancel":
		return a.ardennCancel(ctx, args)
	default:
		return a.wrapped.Execute(ctx, args)
	}
}

// SetShadowMode enables or disables shadow mode programmatically.
func (a *ArdennTeamTasksAdapter) SetShadowMode(enabled bool) {
	a.shadowMode = enabled
}

// ShadowMetrics returns match and discrepancy counts for observability.
func (a *ArdennTeamTasksAdapter) ShadowMetrics() (matches, discrepancies int64) {
	return a.metrics.matches.Load(), a.metrics.discrepancies.Load()
}

// ---------------------------------------------------------------------------
// Feature flag helpers
// ---------------------------------------------------------------------------

func (a *ArdennTeamTasksAdapter) isArdennEnabled(ctx context.Context) bool {
	settings := teamSettingsFromCtx(ctx)
	if settings == nil {
		return false
	}
	enabled, _ := settings["ardenn_enabled"].(bool)
	return enabled
}

func (a *ArdennTeamTasksAdapter) isShadowEnabled(ctx context.Context) bool {
	settings := teamSettingsFromCtx(ctx)
	if settings == nil {
		return false
	}
	shadow, _ := settings["ardenn_shadow"].(bool)
	return shadow
}

// teamSettingsFromCtx extracts team settings from context.
// This leverages GoClaw's existing context propagation pattern.
// Placeholder — wired during integration.
func teamSettingsFromCtx(ctx context.Context) map[string]any {
	if v, ok := ctx.Value(ctxKeyTeamSettings).(map[string]any); ok {
		return v
	}
	return nil
}

// ctxKeyTeamSettings is the context key for team settings injection.
type ctxKeyTeamSettingsType struct{}

var ctxKeyTeamSettings = ctxKeyTeamSettingsType{}

// WithTeamSettings returns a context carrying the given team settings map.
// Used by tests and by the integration wiring layer.
func WithTeamSettings(ctx context.Context, settings map[string]any) context.Context {
	return context.WithValue(ctx, ctxKeyTeamSettings, settings)
}

// isShadowGlobal checks the ARDENN_SHADOW_ENABLED env var.
func isShadowGlobal() bool {
	return os.Getenv("ARDENN_SHADOW_ENABLED") == "true"
}

// ---------------------------------------------------------------------------
// Task 2: Create action mapping
// ---------------------------------------------------------------------------

func (a *ArdennTeamTasksAdapter) ardennCreate(ctx context.Context, args map[string]any) *Result {
	subject, _ := args["subject"].(string)
	description, _ := args["description"].(string)
	assignee, _ := args["assignee"].(string)

	if subject == "" || assignee == "" {
		// Let the original tool handle validation errors
		return a.wrapped.Execute(ctx, args)
	}

	tenantID := tenantIDFromCtx(ctx)
	teamID := teamIDFromCtx(ctx)

	priority := 0
	if p, ok := args["priority"].(float64); ok {
		priority = int(p)
	}

	var blockedBy []uuid.UUID
	if bb, ok := args["blocked_by"].([]any); ok {
		for _, b := range bb {
			if s, ok := b.(string); ok {
				if id, err := uuid.Parse(s); err == nil {
					blockedBy = append(blockedBy, id)
				}
			}
		}
	}

	req := ArdennAdapterStartReq{
		TenantID:    tenantID,
		TeamID:      teamID,
		Subject:     subject,
		Description: description,
		Assignee:    assignee,
		Priority:    priority,
		BlockedBy:   blockedBy,
		Variables: map[string]any{
			"team_id":  teamID.String(),
			"subject":  subject,
			"priority": priority,
		},
	}

	runID, err := a.engine.StartRun(ctx, req)
	if err != nil {
		slog.Error("ardenn.adapter: create failed, falling back",
			"error", err, "subject", subject)
		return a.wrapped.Execute(ctx, args)
	}

	slog.Info("ardenn.adapter: created run",
		"run_id", runID, "subject", subject, "assignee", assignee)

	return NewResult(fmt.Sprintf(
		`{"task_id":"%s","status":"pending","subject":"%s","assignee":"%s","engine":"ardenn"}`,
		runID, subject, assignee,
	))
}

// tenantIDFromCtx extracts tenant ID from context.
// Placeholder — wired during integration via store.TenantIDFromCtx.
func tenantIDFromCtx(ctx context.Context) uuid.UUID {
	if v, ok := ctx.Value(ctxKeyTenantID).(uuid.UUID); ok {
		return v
	}
	return uuid.Nil
}

// teamIDFromCtx extracts team ID from context.
// Placeholder — wired during integration via store.TeamIDFromCtx.
func teamIDFromCtx(ctx context.Context) uuid.UUID {
	if v, ok := ctx.Value(ctxKeyTeamID).(uuid.UUID); ok {
		return v
	}
	return uuid.Nil
}

type ctxKeyTenantIDType struct{}
type ctxKeyTeamIDType struct{}

var ctxKeyTenantID = ctxKeyTenantIDType{}
var ctxKeyTeamID = ctxKeyTeamIDType{}

// WithTenantID returns a context carrying the given tenant ID (for tests).
func WithTenantID(ctx context.Context, id uuid.UUID) context.Context {
	return context.WithValue(ctx, ctxKeyTenantID, id)
}

// WithTeamID returns a context carrying the given team ID (for tests).
func WithTeamID(ctx context.Context, id uuid.UUID) context.Context {
	return context.WithValue(ctx, ctxKeyTeamID, id)
}

// ---------------------------------------------------------------------------
// Task 3: Complete/Cancel action mapping
// ---------------------------------------------------------------------------

func (a *ArdennTeamTasksAdapter) ardennComplete(ctx context.Context, args map[string]any) *Result {
	taskID, _ := args["task_id"].(string)
	if taskID == "" {
		return a.wrapped.Execute(ctx, args)
	}

	runID, err := uuid.Parse(taskID)
	if err != nil {
		// Not an Ardenn UUID — fall through to original
		return a.wrapped.Execute(ctx, args)
	}

	resultText, _ := args["result"].(string)

	if err := a.engine.Wake(ctx, runID); err != nil {
		slog.Error("ardenn.adapter: complete wake failed, falling back",
			"run_id", runID, "error", err)
		return a.wrapped.Execute(ctx, args)
	}

	slog.Info("ardenn.adapter: completed task",
		"run_id", runID, "result_len", len(resultText))

	return NewResult(fmt.Sprintf(
		`{"task_id":"%s","status":"completed","engine":"ardenn"}`, runID,
	))
}

func (a *ArdennTeamTasksAdapter) ardennCancel(ctx context.Context, args map[string]any) *Result {
	taskID, _ := args["task_id"].(string)
	if taskID == "" {
		return a.wrapped.Execute(ctx, args)
	}

	runID, err := uuid.Parse(taskID)
	if err != nil {
		return a.wrapped.Execute(ctx, args)
	}

	reason, _ := args["text"].(string)

	if err := a.engine.Wake(ctx, runID); err != nil {
		slog.Error("ardenn.adapter: cancel wake failed, falling back",
			"run_id", runID, "error", err)
		return a.wrapped.Execute(ctx, args)
	}

	slog.Info("ardenn.adapter: cancelled task",
		"run_id", runID, "reason", reason)

	return NewResult(fmt.Sprintf(
		`{"task_id":"%s","status":"cancelled","engine":"ardenn"}`, runID,
	))
}

// ---------------------------------------------------------------------------
// Task 4: Shadow mode
// ---------------------------------------------------------------------------

func (a *ArdennTeamTasksAdapter) executeShadow(ctx context.Context, action string, args map[string]any) *Result {
	// 1. Run original tool (blocking — this is the result we return)
	originalResult := a.wrapped.Execute(ctx, args)

	// 2. Run Ardenn in background (fire-and-forget)
	bgCtx := context.WithoutCancel(ctx)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("ardenn.shadow: panic in background",
					"action", action, "panic", r)
			}
		}()

		var ardennResult *Result
		switch action {
		case "create":
			ardennResult = a.ardennCreate(bgCtx, args)
		case "complete":
			ardennResult = a.ardennComplete(bgCtx, args)
		case "cancel":
			ardennResult = a.ardennCancel(bgCtx, args)
		}

		a.compareShadow(action, originalResult, ardennResult)
	}()

	return originalResult
}

func (a *ArdennTeamTasksAdapter) compareShadow(action string, original, ardenn *Result) {
	if original == nil || ardenn == nil {
		a.metrics.discrepancies.Add(1)
		slog.Warn("ardenn.shadow: nil result",
			"action", action,
			"original_nil", original == nil,
			"ardenn_nil", ardenn == nil)
		return
	}

	originalErr := original.IsError
	ardennErr := ardenn.IsError

	if originalErr == ardennErr {
		a.metrics.matches.Add(1)
		slog.Info("ardenn.shadow: match",
			"action", action,
			"is_error", originalErr)
	} else {
		a.metrics.discrepancies.Add(1)
		slog.Warn("ardenn.shadow: discrepancy",
			"action", action,
			"original_error", originalErr,
			"ardenn_error", ardennErr,
			"original_content", truncateShadow(original.ForLLM, 200),
			"ardenn_content", truncateShadow(ardenn.ForLLM, 200))
	}
}

func truncateShadow(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
