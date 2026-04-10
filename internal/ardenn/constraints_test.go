package ardenn

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

type alwaysPassGuard struct{ name string }

func (g *alwaysPassGuard) Name() string { return g.name }
func (g *alwaysPassGuard) Check(_ context.Context, _ ConstraintContext) GuardResult {
	return GuardResult{Name: g.name, Pass: true, Severity: "block"}
}

type alwaysBlockGuard struct{ name string }

func (g *alwaysBlockGuard) Name() string { return g.name }
func (g *alwaysBlockGuard) Check(_ context.Context, _ ConstraintContext) GuardResult {
	return GuardResult{Name: g.name, Pass: false, Reason: "denied", Severity: "block"}
}

func TestConstraintEngine_AllPass(t *testing.T) {
	events := &mockEventStore{}
	ce := NewConstraintEngine(events, &alwaysPassGuard{name: "g1"}, &alwaysPassGuard{name: "g2"})

	cc := ConstraintContext{
		TenantID: uuid.New(), RunID: uuid.New(), StepID: uuid.New(),
		AgentKey: "test", Variables: map[string]any{},
	}

	result := ce.Check(context.Background(), cc)
	if !result.Pass {
		t.Fatal("expected pass")
	}
	if result.Blocked {
		t.Fatal("expected not blocked")
	}
}

func TestConstraintEngine_BlockOnFailure(t *testing.T) {
	events := &mockEventStore{}
	ce := NewConstraintEngine(events,
		&alwaysPassGuard{name: "g1"},
		&alwaysBlockGuard{name: "g2"},
		&alwaysPassGuard{name: "g3"}, // should not run
	)

	cc := ConstraintContext{
		TenantID: uuid.New(), RunID: uuid.New(), StepID: uuid.New(),
		AgentKey: "test", Variables: map[string]any{},
	}

	result := ce.Check(context.Background(), cc)
	if result.Pass {
		t.Fatal("expected failure")
	}
	if !result.Blocked {
		t.Fatal("expected blocked")
	}
	// Only 2 guards should have run (short-circuit on block)
	if len(result.Results) != 2 {
		t.Fatalf("got %d results, want 2", len(result.Results))
	}

	// Should have constraint.violated event
	hasViolated := false
	for _, e := range events.events {
		if e.Type == EventConstraintViolated {
			hasViolated = true
		}
	}
	if !hasViolated {
		t.Error("expected constraint.violated event")
	}
}
