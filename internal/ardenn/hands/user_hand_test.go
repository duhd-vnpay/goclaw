package hands

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	engine "github.com/nextlevelbuilder/goclaw/internal/ardenn"
)

func TestUserHand_Execute_CompletionSignal(t *testing.T) {
	cr := NewCompletionRegistry()
	hand := NewUserHand(cr)

	stepRunID := uuid.New()
	req := engine.HandRequest{
		RunID:     uuid.New(),
		StepRunID: stepRunID,
		Name:      "alice",
		Input:     "please review this PR",
	}

	go func() {
		time.Sleep(10 * time.Millisecond)
		cr.Complete(stepRunID, engine.HandResult{Output: "approved"})
	}()

	result := hand.Execute(context.Background(), req)

	if result.Error != nil {
		t.Fatalf("expected no error, got %v", result.Error)
	}
	if result.Output != "approved" {
		t.Errorf("expected output 'approved', got %q", result.Output)
	}
	if result.Duration == 0 {
		t.Error("expected non-zero duration")
	}
}

func TestUserHand_Execute_Timeout(t *testing.T) {
	cr := NewCompletionRegistry()
	hand := NewUserHand(cr)

	req := engine.HandRequest{
		RunID:     uuid.New(),
		StepRunID: uuid.New(),
		Name:      "bob",
		Input:     "approve deployment",
		Timeout:   50 * time.Millisecond,
	}

	result := hand.Execute(context.Background(), req)

	if result.Error == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if got := result.Error.Error(); !contains(got, "timeout") {
		t.Errorf("expected error containing 'timeout', got %q", got)
	}
}

func TestUserHand_Execute_ContextCancelled(t *testing.T) {
	cr := NewCompletionRegistry()
	hand := NewUserHand(cr)

	ctx, cancel := context.WithCancel(context.Background())

	req := engine.HandRequest{
		RunID:     uuid.New(),
		StepRunID: uuid.New(),
		Name:      "carol",
		Input:     "review task",
	}

	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	result := hand.Execute(ctx, req)

	if result.Error == nil {
		t.Fatal("expected context cancelled error, got nil")
	}
	if result.Error != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", result.Error)
	}
}

func TestUserHand_Cancel(t *testing.T) {
	cr := NewCompletionRegistry()
	hand := NewUserHand(cr)

	runID := uuid.New()
	err := hand.Cancel(context.Background(), runID)
	if err != nil {
		t.Fatalf("expected no error from Cancel, got %v", err)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchSubstring(s, substr)
}

func searchSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
