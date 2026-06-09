package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/config"
	"github.com/nextlevelbuilder/goclaw/internal/providers"
	"github.com/nextlevelbuilder/goclaw/internal/sessions"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// Default memory flush prompts matching TS memory-flush.ts.
const (
	DefaultMemoryFlushPrompt = "Pre-compaction memory flush. " +
		"Append durable memories to memory/YYYY-MM-DD.md (create memory/ if needed). " +
		"If the file already exists, APPEND only — do not overwrite existing entries. " +
		"If nothing to store, reply with NO_REPLY."

	DefaultMemoryFlushSystemPrompt = "Pre-compaction memory flush turn. " +
		"The session is near auto-compaction; capture durable memories to disk. " +
		"Append to memory/YYYY-MM-DD.md only. " +
		"If the file already exists, append — do not overwrite. " +
		"You may reply, but usually NO_REPLY is correct."
)

// MemoryFlushSettings holds resolved flush config with defaults applied.
type MemoryFlushSettings struct {
	Enabled      bool
	Prompt       string
	SystemPrompt string
}

// ResolveMemoryFlushSettings resolves flush settings from config, applying defaults.
// Returns nil if disabled.
func ResolveMemoryFlushSettings(compaction *config.CompactionConfig) *MemoryFlushSettings {
	if compaction == nil || compaction.MemoryFlush == nil {
		// Default: enabled
		return &MemoryFlushSettings{
			Enabled:      true,
			Prompt:       DefaultMemoryFlushPrompt,
			SystemPrompt: DefaultMemoryFlushSystemPrompt,
		}
	}

	mf := compaction.MemoryFlush
	if mf.Enabled != nil && !*mf.Enabled {
		return nil
	}

	settings := &MemoryFlushSettings{
		Enabled:      true,
		Prompt:       DefaultMemoryFlushPrompt,
		SystemPrompt: DefaultMemoryFlushSystemPrompt,
	}

	if mf.Prompt != "" {
		settings.Prompt = mf.Prompt
	}
	if mf.SystemPrompt != "" {
		settings.SystemPrompt = mf.SystemPrompt
	}

	return settings
}

// buildMemoryFlushPromptConfig returns the SystemPromptConfig used by the
// memory flush turn. Extracted as a pure function so tests can assert the
// config shape (specifically, that AgentUUID is populated) without building
// a full Loop fixture.
func buildMemoryFlushPromptConfig(
	agentID, agentUUID, model, workspace string,
	toolNames []string,
	hasMemory bool,
	providerType string,
) SystemPromptConfig {
	return SystemPromptConfig{
		AgentID:      agentID,
		AgentUUID:    agentUUID,
		Model:        model,
		Workspace:    workspace,
		Mode:         PromptMinimal,
		ToolNames:    toolNames,
		HasMemory:    hasMemory,
		ProviderType: providerType,
	}
}

// isNonInteractiveSession returns true for any session type that runs as a
// self-contained workflow rather than as an open-ended user chat. Memory-flush
// is harmful for these because the 90s blocking sub-agent + subsequent
// compaction strip pending workflow steps from the agent's context — model
// then decides "task done" prematurely.
//
// Covers:
//   - cron sessions      (agent:X:cron:Y)
//   - heartbeat sessions (agent:X:heartbeat or agent:X:heartbeat:ms)
//   - team sessions      (agent:X:team:Y)
//   - subagent sessions  (agent:X:subagent:Y) — same canonical scheme
//   - delegate sessions  (delegate:...) — uses its own session-key scheme
//     (see internal/tools/delegate_tool.go SenderID "subagent:delegate:...")
//
// Interactive sessions (agent:X:main, agent:X:dm:..., agent:X:group:...) are
// the only category where memory-flush is actually useful: a user is chatting
// across runs and we want to persist durable knowledge between turns.
func isNonInteractiveSession(sessionKey string) bool {
	if strings.HasPrefix(sessionKey, "delegate:") {
		return true
	}
	return sessions.IsCronSession(sessionKey) ||
		sessions.IsHeartbeatSession(sessionKey) ||
		sessions.IsTeamSession(sessionKey) ||
		sessions.IsSubagentSession(sessionKey)
}

// shouldRunMemoryFlush checks whether a memory flush should run before compaction.
// Flush always runs when compaction triggers (called inside maybeSummarize),
// gated only by enabled/memory checks and a dedup guard per compaction cycle.
//
// Skips for non-interactive sessions: cron, heartbeat, team, subagent, delegate.
// All are self-contained workflows. Running an in-loop memory-flush sub-agent
// (90s blocking) + compaction at mid-workflow strips pending workflow steps
// (delegate, send_file, etc.) from the agent's effective context — model
// decides "task done" after the just-written file even when SOUL system prompt
// still requires further steps. None of these session types need to persist
// memories across runs.
//
// Incident 2026-06-01:
//   - ai-usage-analyst weekly cron (agent:X:cron:Y) skipped Step 6 delegate
//     3 runs in a row → fork.9 added IsCronSession check.
//   - Then report-docx-converter delegated worker (delegate:...) ALSO triggered
//     memory-flush mid-workflow → fork.10b added isNonInteractiveSession check
//     (broader: covers delegate + subagent + heartbeat + team too).
func (l *Loop) shouldRunMemoryFlush(ctx context.Context, sessionKey string, totalTokens int, settings *MemoryFlushSettings) bool {
	if settings == nil || !settings.Enabled || !l.hasMemory {
		return false
	}

	if totalTokens <= 0 {
		return false
	}

	if isNonInteractiveSession(sessionKey) {
		slog.Info("memory flush: skipped for non-interactive session",
			"session", sessionKey,
			"reason", "non-interactive workflows (cron/heartbeat/team/subagent/delegate) must not be interrupted by mid-loop memory dump")
		return false
	}

	// Deduplication: skip if already flushed in this compaction cycle.
	compactionCount := l.sessions.GetCompactionCount(ctx, sessionKey)
	lastFlushAt := l.sessions.GetMemoryFlushCompactionCount(ctx, sessionKey)
	if lastFlushAt >= 0 && lastFlushAt == compactionCount {
		return false
	}

	return true
}

// runMemoryFlush executes a memory flush turn: sends flush prompt to LLM with tools
// so it can write memory files. Matching TS agent-runner-memory.ts.
//
// Defense-in-depth non-interactive guard: there are two callers (maybeSummarize
// and pipeline's makeRunMemoryFlush callback) — the latter bypasses
// shouldRunMemoryFlush, so this early-return protects every code path that
// reaches runMemoryFlush. See shouldRunMemoryFlush for the full rationale.
func (l *Loop) runMemoryFlush(ctx context.Context, sessionKey string, settings *MemoryFlushSettings) {
	if isNonInteractiveSession(sessionKey) {
		slog.Info("memory flush: skipped for non-interactive session",
			"session", sessionKey,
			"reason", "non-interactive workflows (cron/heartbeat/team/subagent/delegate) must not be interrupted by mid-loop memory dump")
		return
	}

	slog.Info("memory flush: starting", "session", sessionKey)

	flushCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()

	// Build messages: system prompt + history summary + flush prompt
	history := l.sessions.GetHistory(ctx, sessionKey)
	summary := l.sessions.GetSummary(ctx, sessionKey)

	var messages []providers.Message

	// Replace YYYY-MM-DD placeholder with today's date in flush prompts.
	today := time.Now().Format("2006-01-02")
	flushPrompt := strings.ReplaceAll(settings.Prompt, "YYYY-MM-DD", today)
	flushSystemPrompt := strings.ReplaceAll(settings.SystemPrompt, "YYYY-MM-DD", today)

	// System prompt: combine agent's normal system prompt context with flush system prompt.
	// AgentUUID must stay in sync with loop_history.go's SystemPromptConfig —
	// missing it here historically caused identity drift in downstream DomainEvents.
	systemPrompt := BuildSystemPrompt(buildMemoryFlushPromptConfig(
		l.id,
		l.agentUUID.String(),
		l.model,
		l.workspace,
		l.filteredToolNames(),
		l.hasMemory,
		providerTypeOf(l.provider),
	))
	systemPrompt += "\n\n" + flushSystemPrompt

	messages = append(messages, providers.Message{
		Role:    "system",
		Content: systemPrompt,
	})

	// Include conversation summary for context
	if summary != "" {
		messages = append(messages, providers.Message{
			Role:    "user",
			Content: fmt.Sprintf("[Previous conversation summary]\n%s", summary),
		})
		messages = append(messages, providers.Message{
			Role:    "assistant",
			Content: "Understood.",
		})
	}

	// Include recent history (last 10 messages for context)
	recentHistory := history
	if len(recentHistory) > 10 {
		recentHistory = recentHistory[len(recentHistory)-10:]
	}
	sanitized, _ := sanitizeHistory(recentHistory)
	messages = append(messages, sanitized...)

	// Flush prompt (with YYYY-MM-DD replaced by today's date)
	messages = append(messages, providers.Message{
		Role:    "user",
		Content: flushPrompt,
	})

	// Build tool list — only file tools needed for memory flush
	var toolDefs []providers.ToolDefinition
	if l.toolPolicy != nil {
		toolDefs = l.toolPolicy.FilterTools(l.tools, l.id, l.provider.Name(), nil, nil, false, false)
	} else {
		toolDefs = l.tools.ProviderDefs()
	}

	// Run LLM iteration loop (max 5 iterations for flush)
	maxFlushIter := 5
	for range maxFlushIter {
		chatReq := providers.ChatRequest{
			Messages: messages,
			Tools:    toolDefs,
			Model:    l.model,
			Options: map[string]any{
				"max_tokens":  4096,
				"temperature": 0.3,
			},
		}
		resp, err := l.callInternalLLMWithUsage(flushCtx, chatReq, "memory-flush")
		if err != nil {
			slog.Warn("memory flush: LLM call failed", "error", err)
			l.extractiveMemoryFallback(flushCtx, sessionKey, history, "LLM error")
			break
		}

		// No tool calls → done
		if len(resp.ToolCalls) == 0 {
			content := SanitizeAssistantContent(resp.Content)
			if IsSilentReply(content) {
				slog.Info("memory flush: NO_REPLY, trying extractive fallback")
				l.extractiveMemoryFallback(flushCtx, sessionKey, history, "NO_REPLY")
			} else if content != "" {
				slog.Info("memory flush: completed with response", "content_len", len(content))
			}
			break
		}

		// Process tool calls
		assistantMsg := providers.Message{
			Role:      "assistant",
			Content:   resp.Content,
			ToolCalls: resp.ToolCalls,
		}
		messages = append(messages, assistantMsg)

		for _, tc := range resp.ToolCalls {
			argsJSON, _ := json.Marshal(tc.Arguments)
			slog.Info("memory flush: tool call", "tool", tc.Name, "args_len", len(argsJSON))

			result := l.tools.ExecuteWithContext(flushCtx, tc.Name, tc.Arguments, "", "", "", sessionKey, nil)

			messages = append(messages, providers.Message{
				Role:       "tool",
				Content:    result.ForLLM,
				ToolCallID: tc.ID,
			})
		}
	}

	// Mark flush as done
	l.sessions.SetMemoryFlushDone(ctx, sessionKey)
	l.sessions.Save(ctx, sessionKey)

	slog.Info("memory flush: completed", "session", sessionKey)
}

// extractiveMemoryFallback runs the regex-based extraction on conversation history
// and writes the result directly to the memory store when the LLM flush produced no output.
func (l *Loop) extractiveMemoryFallback(ctx context.Context, sessionKey string, history []providers.Message, reason string) {
	if l.memStore == nil {
		return
	}

	// Limit input to last 20 messages to avoid regex over unbounded text
	if len(history) > 20 {
		history = history[len(history)-20:]
	}

	extracted := ExtractiveMemoryFallback(history)
	if extracted == "" {
		slog.Info("memory flush: extractive fallback produced no content", "session", sessionKey, "reason", reason)
		return
	}

	agentID := l.agentUUID.String()
	userID := store.MemoryUserID(ctx)
	docPath := fmt.Sprintf("memory/%s-auto-extract.md", time.Now().Format("2006-01-02"))

	// Append to existing document if it exists
	existing, err := l.memStore.GetDocument(ctx, agentID, userID, docPath)
	if err == nil && existing != "" {
		extracted = existing + "\n\n---\n\n" + extracted
	}

	if err := l.memStore.PutDocument(ctx, agentID, userID, docPath, extracted); err != nil {
		slog.Warn("memory flush: extractive fallback write failed", "session", sessionKey, "error", err)
		return
	}

	if err := l.memStore.IndexDocument(ctx, agentID, userID, docPath); err != nil {
		slog.Warn("memory flush: extractive fallback index failed", "session", sessionKey, "error", err)
		// Non-fatal: document was saved
	}

	slog.Info("memory flush: extractive fallback saved", "session", sessionKey, "reason", reason, "path", docPath, "content_len", len(extracted))
}
