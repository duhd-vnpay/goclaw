// Package hands implements the Ardenn Hand interface for different dispatch targets.
package hands

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	engine "github.com/nextlevelbuilder/goclaw/internal/ardenn"
	"github.com/nextlevelbuilder/goclaw/internal/bus"
)

// AgentHand dispatches work to an agent via the MessageBus and waits for completion.
type AgentHand struct {
	msgBus     *bus.MessageBus
	completion *CompletionRegistry
}

// NewAgentHand creates an AgentHand wired to the given MessageBus and CompletionRegistry.
func NewAgentHand(msgBus *bus.MessageBus, completion *CompletionRegistry) *AgentHand {
	return &AgentHand{
		msgBus:     msgBus,
		completion: completion,
	}
}

func (h *AgentHand) Type() engine.HandType {
	return engine.HandAgent
}

func (h *AgentHand) Execute(ctx context.Context, req engine.HandRequest) engine.HandResult {
	start := time.Now()

	// Circuit breaker
	dispatchCount := 0
	if dc, ok := req.Metadata["dispatch_count"].(int); ok {
		dispatchCount = dc
	}
	if dispatchCount >= engine.MaxDispatches {
		return engine.HandResult{
			Error:    fmt.Errorf("circuit breaker: max %d dispatches exceeded", engine.MaxDispatches),
			Duration: time.Since(start),
		}
	}

	// Register completion channel before dispatch
	ch := h.completion.Register(req.StepRunID)

	// Build InboundMessage
	msg := bus.InboundMessage{
		Channel:  "system",
		SenderID: "ardenn:engine",
		ChatID:   req.RunID.String(),
		Content:  req.Input,
		AgentID:  req.Name,
		Metadata: buildDispatchMetadata(req),
	}

	if tenantStr, ok := req.Metadata["tenant_id"].(string); ok {
		if tid, err := uuid.Parse(tenantStr); err == nil {
			msg.TenantID = tid
		}
	}

	if !h.msgBus.TryPublishInbound(msg) {
		h.completion.Deregister(req.StepRunID)
		return engine.HandResult{
			Error:    fmt.Errorf("message bus full, dispatch dropped"),
			Duration: time.Since(start),
		}
	}

	slog.Info("ardenn.agent_hand: dispatched",
		"step_run_id", req.StepRunID,
		"agent", req.Name,
		"run_id", req.RunID,
	)

	// Wait for completion or timeout
	timeout := req.Timeout
	if timeout == 0 {
		timeout = 30 * time.Minute
	}

	select {
	case result := <-ch:
		result.Duration = time.Since(start)
		return result
	case <-time.After(timeout):
		h.completion.Deregister(req.StepRunID)
		return engine.HandResult{
			Error:    fmt.Errorf("timeout after %s waiting for agent %s", timeout, req.Name),
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

func (h *AgentHand) Cancel(_ context.Context, runID uuid.UUID) error {
	slog.Info("ardenn.agent_hand: cancel requested", "run_id", runID)
	return nil
}

func buildDispatchMetadata(req engine.HandRequest) map[string]string {
	meta := map[string]string{
		"ardenn_run_id":      req.RunID.String(),
		"ardenn_step_run_id": req.StepRunID.String(),
		"ardenn_hand_type":   "agent",
	}
	for k, v := range req.Metadata {
		if s, ok := v.(string); ok {
			meta[k] = s
		}
	}
	return meta
}
