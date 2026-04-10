// Package extension defines the Extension interface for external packages to hook into the agent loop.
// This is a separate package to avoid import cycles between agent and harness.
package extension

import (
	"context"

	"github.com/google/uuid"
)

// Extension is the interface for external packages to hook into the agent loop
// without modifying core agent code. Harness Engineering implements this interface.
type Extension interface {
	// Name returns the extension identifier
	Name() string

	// Enabled returns true if the extension is active
	Enabled() bool

	// OnBuildSystemPrompt is called when building the system prompt.
	// Returns additional context to inject (or empty string).
	OnBuildSystemPrompt(ctx context.Context, tenantID, agentID uuid.UUID, userID string) string

	// OnRunStart is called at the start of a run
	OnRunStart(ctx context.Context, runID string, req RunRequest)

	// OnRunComplete is called when a run completes
	OnRunComplete(ctx context.Context, runID string, result *RunResult)
}

// RunRequest contains the request data for a run (subset of agent.RunRequest).
// Defined here to avoid import cycle with agent package.
type RunRequest struct {
	SessionKey string
	Message    string
	UserID     string
	Channel    string
}

// RunResult contains the result of a run (subset of agent.RunResult).
// Defined here to avoid import cycle with agent package.
type RunResult struct {
	Content string
	IsError bool
}
