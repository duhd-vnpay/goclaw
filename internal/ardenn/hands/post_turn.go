package hands

import (
	"log/slog"

	"github.com/google/uuid"

	engine "github.com/nextlevelbuilder/goclaw/internal/ardenn"
)

// ResolveAgentCompletion checks if an agent run was dispatched by Ardenn
// and signals completion to the waiting AgentHand.
// Call this from the consumer's post-turn processing.
//
// Returns true if this was an Ardenn-dispatched run (handled), false otherwise.
func ResolveAgentCompletion(
	metadata map[string]string,
	result string,
	runErr error,
	completion *CompletionRegistry,
) bool {
	stepRunIDStr, ok := metadata["ardenn_step_run_id"]
	if !ok {
		return false
	}

	stepRunID, err := uuid.Parse(stepRunIDStr)
	if err != nil {
		slog.Warn("ardenn.post_turn: invalid step_run_id in metadata",
			"step_run_id", stepRunIDStr, "error", err)
		return false
	}

	handResult := engine.HandResult{
		Output: result,
		Error:  runErr,
	}

	completion.Complete(stepRunID, handResult)

	slog.Info("ardenn.post_turn: agent completion signaled",
		"step_run_id", stepRunID,
		"has_error", runErr != nil,
	)

	return true
}
