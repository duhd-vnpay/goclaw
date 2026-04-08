package constraints

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// mockGuard implements the Guard interface for testing.
type mockGuard struct {
	name   string
	phase  Phase
	kind   Kind
	action string
	called *[]string // pointer so we can track call order across instances
}

func (m *mockGuard) Name() string  { return m.name }
func (m *mockGuard) Phase() Phase  { return m.phase }
func (m *mockGuard) Kind() Kind    { return m.kind }
func (m *mockGuard) Check(ctx GuardContext) GuardResult {
	if m.called != nil {
		*m.called = append(*m.called, m.name)
	}
	return GuardResult{
		Pass:      m.action != "block",
		Action:    m.action,
		Feedback:  "feedback from " + m.name,
		GuardName: m.name,
	}
}

// TestGuardRegistry_RunPhase_OrdersComputationalFirst verifies that when an
// inferential guard and a computational guard are registered in that order,
// the computational guard runs first.
func TestGuardRegistry_RunPhase_OrdersComputationalFirst(t *testing.T) {
	callOrder := []string{}

	inferential := &mockGuard{
		name:   "inferential-guard",
		phase:  BeforeRun,
		kind:   Inferential,
		action: "allow",
		called: &callOrder,
	}
	computational := &mockGuard{
		name:   "computational-guard",
		phase:  BeforeRun,
		kind:   Computational,
		action: "allow",
		called: &callOrder,
	}

	registry := NewGuardRegistry()
	registry.Register(inferential) // register inferential first
	registry.Register(computational)

	ctx := GuardContext{AgentID: "agent-1"}
	results := registry.RunPhase(BeforeRun, ctx)

	assert.Len(t, results, 2)
	assert.Equal(t, "computational-guard", callOrder[0], "computational guard should run first")
	assert.Equal(t, "inferential-guard", callOrder[1], "inferential guard should run second")
}

// TestGuardRegistry_RunPhase_StopsOnBlock verifies that RunPhase stops
// processing guards after a "block" action is returned.
func TestGuardRegistry_RunPhase_StopsOnBlock(t *testing.T) {
	callOrder := []string{}

	blocker := &mockGuard{
		name:   "blocking-guard",
		phase:  BeforeRun,
		kind:   Computational,
		action: "block",
		called: &callOrder,
	}
	subsequent := &mockGuard{
		name:   "subsequent-guard",
		phase:  BeforeRun,
		kind:   Inferential,
		action: "allow",
		called: &callOrder,
	}

	registry := NewGuardRegistry()
	registry.Register(blocker)
	registry.Register(subsequent)

	ctx := GuardContext{AgentID: "agent-1"}
	results := registry.RunPhase(BeforeRun, ctx)

	assert.Len(t, results, 1, "only 1 result should be returned when a block occurs")
	assert.Equal(t, "block", results[0].Action)
	assert.Equal(t, "blocking-guard", results[0].GuardName)
	assert.Len(t, callOrder, 1, "subsequent guard should not have been called")
}

// TestGuardRegistry_RunPhase_FiltersPhase verifies that RunPhase only runs
// guards registered for the queried phase.
func TestGuardRegistry_RunPhase_FiltersPhase(t *testing.T) {
	callOrder := []string{}

	beforeRunGuard := &mockGuard{
		name:   "before-run-guard",
		phase:  BeforeRun,
		kind:   Computational,
		action: "allow",
		called: &callOrder,
	}
	afterRunGuard := &mockGuard{
		name:   "after-run-guard",
		phase:  AfterRun,
		kind:   Computational,
		action: "allow",
		called: &callOrder,
	}

	registry := NewGuardRegistry()
	registry.Register(beforeRunGuard)
	registry.Register(afterRunGuard)

	ctx := GuardContext{AgentID: "agent-1"}
	results := registry.RunPhase(BeforeRun, ctx)

	assert.Len(t, results, 1, "only guards for BeforeRun phase should run")
	assert.Equal(t, "before-run-guard", results[0].GuardName)
	assert.Len(t, callOrder, 1, "after-run guard should not have been called")
	assert.Equal(t, "before-run-guard", callOrder[0])
}
