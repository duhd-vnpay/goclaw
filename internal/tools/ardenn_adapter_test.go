package tools

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

type mockWrappedTool struct {
	name       string
	lastAction string
	result     *Result
	mu         sync.Mutex
	callCount  int
}

func (m *mockWrappedTool) Name() string               { return m.name }
func (m *mockWrappedTool) Description() string        { return "mock team_tasks tool" }
func (m *mockWrappedTool) Parameters() map[string]any { return map[string]any{"action": "string"} }
func (m *mockWrappedTool) Execute(_ context.Context, args map[string]any) *Result {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastAction, _ = args["action"].(string)
	m.callCount++
	return m.result
}

func (m *mockWrappedTool) getCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.callCount
}

type mockArdennEngine struct {
	mu          sync.Mutex
	startCalled bool
	wakeCalled  bool
	startErr    error
	wakeErr     error
	runID       uuid.UUID
	lastReq     ArdennAdapterStartReq
}

func (m *mockArdennEngine) StartRun(_ context.Context, req ArdennAdapterStartReq) (uuid.UUID, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.startCalled = true
	m.lastReq = req
	return m.runID, m.startErr
}

func (m *mockArdennEngine) Wake(_ context.Context, _ uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.wakeCalled = true
	return m.wakeErr
}

func (m *mockArdennEngine) wasStartCalled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.startCalled
}

func (m *mockArdennEngine) wasWakeCalled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.wakeCalled
}

// ---------------------------------------------------------------------------
// Helper: build context with ardenn_enabled
// ---------------------------------------------------------------------------

func ardennCtx(enabled, shadow bool) context.Context {
	ctx := context.Background()
	settings := map[string]any{
		"ardenn_enabled": enabled,
		"ardenn_shadow":  shadow,
	}
	return WithTeamSettings(ctx, settings)
}

// ---------------------------------------------------------------------------
// Task 1 tests
// ---------------------------------------------------------------------------

func TestAdapter_PassThrough_WhenDisabled(t *testing.T) {
	wrapped := &mockWrappedTool{name: "team_tasks", result: NewResult("original")}
	engine := &mockArdennEngine{runID: uuid.New()}
	adapter := NewArdennTeamTasksAdapter(wrapped, engine)

	// Context without ardenn_enabled
	ctx := context.Background()
	result := adapter.Execute(ctx, map[string]any{"action": "create", "subject": "test", "assignee": "bot"})

	if result.ForLLM != "original" {
		t.Errorf("expected original result, got %q", result.ForLLM)
	}
	if engine.wasStartCalled() {
		t.Error("engine.StartRun should not be called when disabled")
	}
}

func TestAdapter_PassThrough_ReadActions(t *testing.T) {
	wrapped := &mockWrappedTool{name: "team_tasks", result: NewResult("list-result")}
	engine := &mockArdennEngine{runID: uuid.New()}
	adapter := NewArdennTeamTasksAdapter(wrapped, engine)

	ctx := ardennCtx(true, false)

	readActions := []string{
		"list", "get", "search", "comment", "progress",
		"claim", "review", "approve", "reject", "update",
		"attach", "ask_user", "clear_ask_user", "retry",
	}

	for _, action := range readActions {
		t.Run(action, func(t *testing.T) {
			result := adapter.Execute(ctx, map[string]any{"action": action})
			if result.ForLLM != "list-result" {
				t.Errorf("expected pass-through for action %q, got %q", action, result.ForLLM)
			}
			if engine.wasStartCalled() {
				t.Errorf("engine should not be called for action %q", action)
			}
		})
	}
}

func TestAdapter_DelegatesMetadata(t *testing.T) {
	wrapped := &mockWrappedTool{name: "team_tasks", result: NewResult("ok")}
	adapter := NewArdennTeamTasksAdapter(wrapped, nil)

	if adapter.Name() != "team_tasks" {
		t.Errorf("Name() = %q, want %q", adapter.Name(), "team_tasks")
	}
	if adapter.Description() != "mock team_tasks tool" {
		t.Errorf("Description() = %q", adapter.Description())
	}
	if adapter.Parameters() == nil {
		t.Error("Parameters() should not be nil")
	}
}

func TestAdapter_PassThrough_NilEngine(t *testing.T) {
	wrapped := &mockWrappedTool{name: "team_tasks", result: NewResult("original")}
	adapter := NewArdennTeamTasksAdapter(wrapped, nil)

	ctx := ardennCtx(true, false)
	result := adapter.Execute(ctx, map[string]any{"action": "create", "subject": "test", "assignee": "bot"})

	if result.ForLLM != "original" {
		t.Errorf("expected pass-through with nil engine, got %q", result.ForLLM)
	}
}

// ---------------------------------------------------------------------------
// Task 2 tests
// ---------------------------------------------------------------------------

func TestAdapter_Create_MapsToArdennRun(t *testing.T) {
	wrapped := &mockWrappedTool{name: "team_tasks", result: NewResult("original")}
	runID := uuid.New()
	engine := &mockArdennEngine{runID: runID}
	adapter := NewArdennTeamTasksAdapter(wrapped, engine)

	ctx := ardennCtx(true, false)
	result := adapter.Execute(ctx, map[string]any{
		"action":      "create",
		"subject":     "Review PR",
		"assignee":    "code-reviewer",
		"description": "Please review this PR",
		"priority":    float64(2),
	})

	if !engine.wasStartCalled() {
		t.Fatal("engine.StartRun should have been called")
	}
	if engine.lastReq.Subject != "Review PR" {
		t.Errorf("subject = %q, want %q", engine.lastReq.Subject, "Review PR")
	}
	if engine.lastReq.Assignee != "code-reviewer" {
		t.Errorf("assignee = %q, want %q", engine.lastReq.Assignee, "code-reviewer")
	}
	if engine.lastReq.Priority != 2 {
		t.Errorf("priority = %d, want 2", engine.lastReq.Priority)
	}
	if result.IsError {
		t.Errorf("result should not be an error: %s", result.ForLLM)
	}
	// Result should contain the run ID
	if !contains(result.ForLLM, runID.String()) {
		t.Errorf("result should contain run ID %s, got: %s", runID, result.ForLLM)
	}
	if !contains(result.ForLLM, `"engine":"ardenn"`) {
		t.Errorf("result should contain engine marker, got: %s", result.ForLLM)
	}
}

func TestAdapter_Create_MissingSubject_Fallback(t *testing.T) {
	wrapped := &mockWrappedTool{name: "team_tasks", result: NewResult("validation-error")}
	engine := &mockArdennEngine{runID: uuid.New()}
	adapter := NewArdennTeamTasksAdapter(wrapped, engine)

	ctx := ardennCtx(true, false)
	result := adapter.Execute(ctx, map[string]any{
		"action":   "create",
		"assignee": "bot",
		// missing subject
	})

	if result.ForLLM != "validation-error" {
		t.Errorf("expected fallback to wrapped tool, got %q", result.ForLLM)
	}
	if engine.wasStartCalled() {
		t.Error("engine should not be called without subject")
	}
}

func TestAdapter_Create_EngineFails_Fallback(t *testing.T) {
	wrapped := &mockWrappedTool{name: "team_tasks", result: NewResult("fallback-result")}
	engine := &mockArdennEngine{
		runID:    uuid.New(),
		startErr: errors.New("engine unavailable"),
	}
	adapter := NewArdennTeamTasksAdapter(wrapped, engine)

	ctx := ardennCtx(true, false)
	result := adapter.Execute(ctx, map[string]any{
		"action":   "create",
		"subject":  "test",
		"assignee": "bot",
	})

	if result.ForLLM != "fallback-result" {
		t.Errorf("expected fallback on engine error, got %q", result.ForLLM)
	}
}

func TestAdapter_Create_WithBlockedBy(t *testing.T) {
	wrapped := &mockWrappedTool{name: "team_tasks", result: NewResult("original")}
	engine := &mockArdennEngine{runID: uuid.New()}
	adapter := NewArdennTeamTasksAdapter(wrapped, engine)

	id1 := uuid.New()
	id2 := uuid.New()

	ctx := ardennCtx(true, false)
	adapter.Execute(ctx, map[string]any{
		"action":     "create",
		"subject":    "Blocked task",
		"assignee":   "bot",
		"blocked_by": []any{id1.String(), id2.String()},
	})

	if len(engine.lastReq.BlockedBy) != 2 {
		t.Fatalf("expected 2 blocked_by IDs, got %d", len(engine.lastReq.BlockedBy))
	}
	if engine.lastReq.BlockedBy[0] != id1 {
		t.Errorf("blocked_by[0] = %s, want %s", engine.lastReq.BlockedBy[0], id1)
	}
}

// ---------------------------------------------------------------------------
// Task 3 tests
// ---------------------------------------------------------------------------

func TestAdapter_Complete_WakesEngine(t *testing.T) {
	wrapped := &mockWrappedTool{name: "team_tasks", result: NewResult("original")}
	runID := uuid.New()
	engine := &mockArdennEngine{runID: runID}
	adapter := NewArdennTeamTasksAdapter(wrapped, engine)

	ctx := ardennCtx(true, false)
	result := adapter.Execute(ctx, map[string]any{
		"action":  "complete",
		"task_id": runID.String(),
		"result":  "Done",
	})

	if !engine.wasWakeCalled() {
		t.Fatal("engine.Wake should have been called")
	}
	if result.IsError {
		t.Errorf("result should not be error: %s", result.ForLLM)
	}
	if !contains(result.ForLLM, `"status":"completed"`) {
		t.Errorf("result should contain completed status, got: %s", result.ForLLM)
	}
}

func TestAdapter_Cancel_WakesEngine(t *testing.T) {
	wrapped := &mockWrappedTool{name: "team_tasks", result: NewResult("original")}
	runID := uuid.New()
	engine := &mockArdennEngine{runID: runID}
	adapter := NewArdennTeamTasksAdapter(wrapped, engine)

	ctx := ardennCtx(true, false)
	result := adapter.Execute(ctx, map[string]any{
		"action":  "cancel",
		"task_id": runID.String(),
		"text":    "No longer needed",
	})

	if !engine.wasWakeCalled() {
		t.Fatal("engine.Wake should have been called")
	}
	if !contains(result.ForLLM, `"status":"cancelled"`) {
		t.Errorf("result should contain cancelled status, got: %s", result.ForLLM)
	}
}

func TestAdapter_Complete_InvalidTaskID_Fallback(t *testing.T) {
	wrapped := &mockWrappedTool{name: "team_tasks", result: NewResult("fallback")}
	engine := &mockArdennEngine{runID: uuid.New()}
	adapter := NewArdennTeamTasksAdapter(wrapped, engine)

	ctx := ardennCtx(true, false)
	result := adapter.Execute(ctx, map[string]any{
		"action":  "complete",
		"task_id": "not-a-uuid",
	})

	if result.ForLLM != "fallback" {
		t.Errorf("expected fallback for invalid UUID, got %q", result.ForLLM)
	}
	if engine.wasWakeCalled() {
		t.Error("engine.Wake should not be called with invalid UUID")
	}
}

func TestAdapter_Complete_EmptyTaskID_Fallback(t *testing.T) {
	wrapped := &mockWrappedTool{name: "team_tasks", result: NewResult("fallback")}
	engine := &mockArdennEngine{runID: uuid.New()}
	adapter := NewArdennTeamTasksAdapter(wrapped, engine)

	ctx := ardennCtx(true, false)
	result := adapter.Execute(ctx, map[string]any{
		"action": "complete",
	})

	if result.ForLLM != "fallback" {
		t.Errorf("expected fallback for empty task_id, got %q", result.ForLLM)
	}
}

func TestAdapter_Complete_EngineFails_Fallback(t *testing.T) {
	wrapped := &mockWrappedTool{name: "team_tasks", result: NewResult("fallback")}
	engine := &mockArdennEngine{
		runID:   uuid.New(),
		wakeErr: errors.New("engine down"),
	}
	adapter := NewArdennTeamTasksAdapter(wrapped, engine)

	ctx := ardennCtx(true, false)
	result := adapter.Execute(ctx, map[string]any{
		"action":  "complete",
		"task_id": uuid.New().String(),
		"result":  "Done",
	})

	if result.ForLLM != "fallback" {
		t.Errorf("expected fallback on wake error, got %q", result.ForLLM)
	}
}

// ---------------------------------------------------------------------------
// Task 4 tests: Shadow mode
// ---------------------------------------------------------------------------

func TestAdapter_Shadow_RunsBothSystems(t *testing.T) {
	wrapped := &mockWrappedTool{name: "team_tasks", result: NewResult("original-result")}
	runID := uuid.New()
	engine := &mockArdennEngine{runID: runID}
	adapter := NewArdennTeamTasksAdapter(wrapped, engine)
	adapter.SetShadowMode(true)

	ctx := ardennCtx(true, false)
	result := adapter.Execute(ctx, map[string]any{
		"action":   "create",
		"subject":  "Shadow test",
		"assignee": "bot",
	})

	// Should return the original result
	if result.ForLLM != "original-result" {
		t.Errorf("shadow should return original result, got %q", result.ForLLM)
	}

	// Wrapped tool should have been called
	if wrapped.getCallCount() == 0 {
		t.Error("wrapped tool should have been called")
	}

	// Give goroutine time to complete
	time.Sleep(50 * time.Millisecond)

	if !engine.wasStartCalled() {
		t.Error("engine.StartRun should have been called in shadow")
	}
}

func TestAdapter_Shadow_ViaTeamSettings(t *testing.T) {
	wrapped := &mockWrappedTool{name: "team_tasks", result: NewResult("original")}
	engine := &mockArdennEngine{runID: uuid.New()}
	adapter := NewArdennTeamTasksAdapter(wrapped, engine)
	// shadowMode field is false, but context has ardenn_shadow=true

	ctx := ardennCtx(true, true) // ardenn_enabled=true, ardenn_shadow=true
	result := adapter.Execute(ctx, map[string]any{
		"action":   "create",
		"subject":  "Shadow via settings",
		"assignee": "bot",
	})

	if result.ForLLM != "original" {
		t.Errorf("expected original result in shadow mode, got %q", result.ForLLM)
	}

	time.Sleep(50 * time.Millisecond)
	if !engine.wasStartCalled() {
		t.Error("engine should have been called in shadow mode via team settings")
	}
}

func TestAdapter_Shadow_ReadActionsNotShadowed(t *testing.T) {
	wrapped := &mockWrappedTool{name: "team_tasks", result: NewResult("list-data")}
	engine := &mockArdennEngine{runID: uuid.New()}
	adapter := NewArdennTeamTasksAdapter(wrapped, engine)
	adapter.SetShadowMode(true)

	ctx := ardennCtx(true, false)
	result := adapter.Execute(ctx, map[string]any{"action": "list"})

	if result.ForLLM != "list-data" {
		t.Errorf("expected pass-through for list, got %q", result.ForLLM)
	}
	// Read actions should not trigger shadow
	time.Sleep(50 * time.Millisecond)
	if engine.wasStartCalled() {
		t.Error("engine should not be called for read actions even in shadow mode")
	}
}

func TestAdapter_Shadow_MetricsIncrement(t *testing.T) {
	wrapped := &mockWrappedTool{name: "team_tasks", result: NewResult("ok")}
	engine := &mockArdennEngine{runID: uuid.New()}
	adapter := NewArdennTeamTasksAdapter(wrapped, engine)
	adapter.SetShadowMode(true)

	ctx := ardennCtx(true, false)
	adapter.Execute(ctx, map[string]any{
		"action":   "create",
		"subject":  "Metrics test",
		"assignee": "bot",
	})

	time.Sleep(50 * time.Millisecond)

	matches, discrepancies := adapter.ShadowMetrics()
	total := matches + discrepancies
	if total == 0 {
		t.Error("expected at least one shadow comparison to have run")
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
