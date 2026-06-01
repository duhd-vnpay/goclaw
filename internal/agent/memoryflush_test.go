package agent

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

// TestBuildMemoryFlushPromptConfig_AgentUUIDPopulated asserts the
// SystemPromptConfig returned by buildMemoryFlushPromptConfig carries the
// AgentUUID. loop_history.go always set it but memoryflush.go historically
// did not, which would have caused identity drift in downstream DomainEvents
// if AgentUUID ever reached the stable cache prefix.
func TestBuildMemoryFlushPromptConfig_AgentUUIDPopulated(t *testing.T) {
	u := uuid.New()
	cfg := buildMemoryFlushPromptConfig(
		"test-agent",
		u.String(),
		"claude-opus",
		"/workspace",
		[]string{"read_file", "write_file"},
		true,
		"anthropic",
	)

	if cfg.AgentUUID != u.String() {
		t.Errorf("AgentUUID = %q, want %q", cfg.AgentUUID, u.String())
	}
	if cfg.AgentID != "test-agent" {
		t.Errorf("AgentID = %q, want %q", cfg.AgentID, "test-agent")
	}
	if cfg.Model != "claude-opus" {
		t.Errorf("Model = %q, want %q", cfg.Model, "claude-opus")
	}
	if cfg.Workspace != "/workspace" {
		t.Errorf("Workspace = %q, want %q", cfg.Workspace, "/workspace")
	}
	if cfg.Mode != PromptMinimal {
		t.Errorf("Mode = %q, want %q", cfg.Mode, PromptMinimal)
	}
	if !cfg.HasMemory {
		t.Error("HasMemory should be true")
	}
	if cfg.ProviderType != "anthropic" {
		t.Errorf("ProviderType = %q, want %q", cfg.ProviderType, "anthropic")
	}
	if len(cfg.ToolNames) != 2 {
		t.Errorf("ToolNames len = %d, want 2", len(cfg.ToolNames))
	}
}

// TestBuildMemoryFlushPromptConfig_EmptyAgentUUID guards against a regression where
// a Loop with uuid.Nil (zero value) would still produce a non-empty AgentUUID string.
// The .String() of uuid.Nil is "00000000-0000-0000-0000-000000000000" — not empty,
// but it MUST be identifiable downstream so the publish-time observer in eventbus
// can still warn on it after Fix C wires validateAgentID into bus.Publish.
func TestBuildMemoryFlushPromptConfig_ZeroUUIDStringified(t *testing.T) {
	cfg := buildMemoryFlushPromptConfig(
		"zero-agent",
		uuid.Nil.String(),
		"m",
		"/w",
		nil,
		false,
		"p",
	)

	if cfg.AgentUUID != uuid.Nil.String() {
		t.Errorf("AgentUUID = %q, want %q", cfg.AgentUUID, uuid.Nil.String())
	}
}

// TestShouldRunMemoryFlush_SkipsCronSession asserts that cron sessions never
// trigger memory-flush. Memory-flush runs as a synchronous sub-agent (~90s
// blocking) and is followed by compaction that summarizes ~70% of messages.
// For cron workflows (weekly analytics report, daily ops report), this strips
// pending workflow steps (delegate, send_file) from the agent's effective
// context — the model decides "task done" after the just-written file even
// when SOUL still requires further steps.
//
// Regression guard: incident 2026-06-01 — ai-usage-analyst weekly cron
// skipped Step 6 delegate to report-docx-converter 3 runs in a row.
//
// The early-return for cron happens BEFORE the sessions store calls, so a
// minimal Loop without sessions can exercise this branch.
func TestShouldRunMemoryFlush_SkipsCronSession(t *testing.T) {
	t.Parallel()

	loop := &Loop{
		id:        "ai-usage-analyst",
		hasMemory: true,
	}
	settings := &MemoryFlushSettings{Enabled: true}

	cronKey := "agent:ai-usage-analyst:cron:019e4d84-76c0-7612-b14f-a86cb6b2289b"
	if loop.shouldRunMemoryFlush(context.Background(), cronKey, 80000, settings) {
		t.Fatalf("shouldRunMemoryFlush must return false for cron session, got true (key=%s)", cronKey)
	}
}

// TestShouldRunMemoryFlush_DisabledSettings asserts the early-return for
// nil/disabled settings still works (defense in depth before the cron check).
func TestShouldRunMemoryFlush_DisabledSettings(t *testing.T) {
	t.Parallel()

	loop := &Loop{id: "x", hasMemory: true}

	if loop.shouldRunMemoryFlush(context.Background(), "any-key", 100, nil) {
		t.Fatal("expected false with nil settings")
	}

	if loop.shouldRunMemoryFlush(context.Background(), "any-key", 100, &MemoryFlushSettings{Enabled: false}) {
		t.Fatal("expected false with disabled settings")
	}
}
