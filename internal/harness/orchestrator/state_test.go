package orchestrator

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func stepIDs(steps []Step) []string {
	ids := make([]string, len(steps))
	for i, s := range steps {
		ids[i] = s.ID
	}
	return ids
}

func TestStateManager_GetReadySteps_NoDeps(t *testing.T) {
	wf := Workflow{
		Steps: []Step{
			{ID: "a", DependsOn: nil},
			{ID: "b", DependsOn: []string{"a"}},
			{ID: "c", DependsOn: nil},
		},
	}

	sm := NewStateManager()
	run := sm.CreateRun(wf, nil)

	ready := sm.GetReadySteps(run)
	ids := stepIDs(ready)
	assert.Contains(t, ids, "a")
	assert.Contains(t, ids, "c")
	assert.NotContains(t, ids, "b")
}

func TestStateManager_GetReadySteps_AfterComplete(t *testing.T) {
	wf := Workflow{
		Steps: []Step{
			{ID: "a", DependsOn: nil},
			{ID: "b", DependsOn: []string{"a"}},
		},
	}

	sm := NewStateManager()
	run := sm.CreateRun(wf, nil)

	sm.MarkStep(run, "a", "completed")

	ready := sm.GetReadySteps(run)
	ids := stepIDs(ready)
	assert.Contains(t, ids, "b")
}

func TestStateManager_IsComplete(t *testing.T) {
	wf := Workflow{
		Steps: []Step{
			{ID: "a", DependsOn: nil},
			{ID: "b", DependsOn: []string{"a"}},
		},
	}

	sm := NewStateManager()
	run := sm.CreateRun(wf, nil)

	assert.False(t, sm.IsComplete(run))
	sm.MarkStep(run, "a", "completed")
	assert.False(t, sm.IsComplete(run))
	sm.MarkStep(run, "b", "completed")
	assert.True(t, sm.IsComplete(run))
}

func TestStateManager_ConditionalStep_SkippedWhenFalse(t *testing.T) {
	wf := Workflow{
		Steps: []Step{
			{ID: "a", DependsOn: nil, When: "variables.pipeline_type in ['full']"},
		},
		Variables: map[string]any{"pipeline_type": "bug-fix"},
	}

	sm := NewStateManager()
	run := sm.CreateRun(wf, nil)

	ready := sm.GetReadySteps(run)
	assert.Len(t, ready, 0)
	assert.True(t, sm.IsComplete(run))
}

func TestStateManager_ConditionalStep_RunsWhenTrue(t *testing.T) {
	wf := Workflow{
		Steps: []Step{
			{ID: "a", DependsOn: nil, When: "variables.pipeline_type in ['full']"},
		},
		Variables: map[string]any{"pipeline_type": "full"},
	}

	sm := NewStateManager()
	run := sm.CreateRun(wf, nil)

	ready := sm.GetReadySteps(run)
	assert.Len(t, ready, 1)
	assert.Equal(t, "a", ready[0].ID)
}
