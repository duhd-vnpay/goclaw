package agent

import (
	"context"
	"log/slog"

	"github.com/nextlevelbuilder/goclaw/internal/harness/continuity"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// buildHarnessResumeContext returns the L2 session resume context for the system prompt.
// Returns empty string if harness is disabled or no artifact exists.
func (l *Loop) buildHarnessResumeContext(ctx context.Context, userID string) string {
	if l.harness == nil || !l.harness.Enabled() {
		return ""
	}

	artifact, err := l.harness.Artifacts().GetLatest(ctx, l.tenantID, l.agentUUID, userID)
	if err != nil {
		slog.Warn("harness.resume_context_error", "agent", l.id, "user", userID, "error", err)
		return ""
	}

	return continuity.BuildResumeContext(artifact)
}

// harnessEnabled returns true if the harness layer is active.
func (l *Loop) harnessEnabled() bool {
	return l.harness != nil && l.harness.Enabled()
}

// harnessAgentKey returns the agent key for harness context, falling back to the Loop's
// agent key. Used in downstream harness operations that need the agent's identity.
func (l *Loop) harnessAgentKey() string {
	if key := store.AgentKeyFromContext(context.Background()); key != "" {
		return key
	}
	return l.id
}
