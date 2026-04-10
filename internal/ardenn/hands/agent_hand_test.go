package hands

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	engine "github.com/nextlevelbuilder/goclaw/internal/ardenn"
	"github.com/nextlevelbuilder/goclaw/internal/bus"
)

func TestAgentHand_Type(t *testing.T) {
	h := NewAgentHand(nil, nil)
	if h.Type() != engine.HandAgent {
		t.Errorf("Type() = %v, want agent", h.Type())
	}
}

func TestAgentHand_SuccessfulDispatch(t *testing.T) {
	msgBus := bus.New()
	completion := NewCompletionRegistry()
	h := NewAgentHand(msgBus, completion)

	stepRunID := uuid.New()
	runID := uuid.New()

	go func() {
		time.Sleep(50 * time.Millisecond)
		completion.Complete(stepRunID, engine.HandResult{Output: "task done"})
	}()

	result := h.Execute(context.Background(), engine.HandRequest{
		RunID:     runID,
		StepRunID: stepRunID,
		Name:      "test-agent",
		Input:     "Do something",
		Metadata:  map[string]any{"tenant_id": uuid.New().String()},
		Timeout:   5 * time.Second,
	})

	if result.Error != nil {
		t.Fatalf("unexpected error: %v", result.Error)
	}
	if result.Output != "task done" {
		t.Errorf("output = %q, want 'task done'", result.Output)
	}
	if result.Duration == 0 {
		t.Error("duration should be > 0")
	}
}

func TestAgentHand_Timeout(t *testing.T) {
	msgBus := bus.New()
	completion := NewCompletionRegistry()
	h := NewAgentHand(msgBus, completion)

	result := h.Execute(context.Background(), engine.HandRequest{
		RunID:     uuid.New(),
		StepRunID: uuid.New(),
		Name:      "slow-agent",
		Input:     "This will timeout",
		Metadata:  map[string]any{"tenant_id": uuid.New().String()},
		Timeout:   100 * time.Millisecond,
	})

	if result.Error == nil {
		t.Fatal("expected timeout error")
	}
}

func TestAgentHand_CircuitBreaker(t *testing.T) {
	msgBus := bus.New()
	completion := NewCompletionRegistry()
	h := NewAgentHand(msgBus, completion)

	result := h.Execute(context.Background(), engine.HandRequest{
		RunID:     uuid.New(),
		StepRunID: uuid.New(),
		Name:      "broken-agent",
		Input:     "Will be rejected",
		Metadata:  map[string]any{"dispatch_count": engine.MaxDispatches},
		Timeout:   1 * time.Second,
	})

	if result.Error == nil {
		t.Fatal("expected circuit breaker error")
	}
}

func TestAgentHand_ContextCancellation(t *testing.T) {
	msgBus := bus.New()
	completion := NewCompletionRegistry()
	h := NewAgentHand(msgBus, completion)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	result := h.Execute(ctx, engine.HandRequest{
		RunID:     uuid.New(),
		StepRunID: uuid.New(),
		Name:      "test-agent",
		Input:     "Will be cancelled",
		Metadata:  map[string]any{"tenant_id": uuid.New().String()},
		Timeout:   5 * time.Second,
	})

	if result.Error == nil {
		t.Fatal("expected context cancelled error")
	}
}
