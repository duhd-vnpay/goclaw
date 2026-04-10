package hands

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	engine "github.com/nextlevelbuilder/goclaw/internal/ardenn"
)

// UserHand dispatches work to a human user and blocks until completion via CompletionRegistry.
type UserHand struct {
	completion *CompletionRegistry
}

// NewUserHand creates a UserHand wired to the given CompletionRegistry.
func NewUserHand(completion *CompletionRegistry) *UserHand {
	return &UserHand{completion: completion}
}

func (h *UserHand) Type() engine.HandType { return engine.HandUser }

func (h *UserHand) Execute(ctx context.Context, req engine.HandRequest) engine.HandResult {
	start := time.Now()

	ch := h.completion.Register(req.StepRunID)

	slog.Info("ardenn.user_hand: dispatched",
		"step_run_id", req.StepRunID,
		"assigned_to", req.Name,
		"run_id", req.RunID,
	)

	timeout := req.Timeout
	if timeout == 0 {
		timeout = 24 * time.Hour // Human tasks default to 24h
	}

	select {
	case result := <-ch:
		result.Duration = time.Since(start)
		return result
	case <-time.After(timeout):
		h.completion.Deregister(req.StepRunID)
		return engine.HandResult{
			Error:    fmt.Errorf("timeout after %s waiting for user %s", timeout, req.Name),
			Duration: time.Since(start),
		}
	case <-ctx.Done():
		h.completion.Deregister(req.StepRunID)
		return engine.HandResult{
			Error:    ctx.Err(),
			Duration: time.Since(start),
		}
	}
}

func (h *UserHand) Cancel(_ context.Context, runID uuid.UUID) error {
	slog.Info("ardenn.user_hand: cancel requested", "run_id", runID)
	return nil
}
