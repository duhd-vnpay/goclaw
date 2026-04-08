package orchestrator

import (
	"sync"

	"github.com/google/uuid"
)

// WorkflowRun tracks execution state for a single workflow instance.
type WorkflowRun struct {
	ID        uuid.UUID
	Workflow  Workflow
	Variables map[string]any
	Steps     map[string]*StepState
	Status    string // "pending","running","completed","failed","cancelled","paused"
	mu        sync.RWMutex
}

// StepState tracks execution state for a single step.
type StepState struct {
	Step   Step
	Status string // "pending","running","completed","failed","blocked","skipped","waiting_gate"
}

// StateManager manages workflow execution state (in-memory for v1).
type StateManager struct{}

// NewStateManager creates a new state manager.
func NewStateManager() *StateManager {
	return &StateManager{}
}

// CreateRun initializes a workflow run with merged variables and conditional step evaluation.
func (sm *StateManager) CreateRun(wf Workflow, input map[string]any) *WorkflowRun {
	vars := make(map[string]any)
	for k, v := range wf.Variables {
		vars[k] = v
	}
	for k, v := range input {
		vars[k] = v
	}

	steps := make(map[string]*StepState, len(wf.Steps))
	for _, s := range wf.Steps {
		status := "pending"
		if s.When != "" {
			pass, err := EvalExpr(s.When, vars)
			if err != nil || !pass {
				status = "skipped"
			}
		}
		steps[s.ID] = &StepState{Step: s, Status: status}
	}

	return &WorkflowRun{
		ID:        uuid.New(),
		Workflow:  wf,
		Variables: vars,
		Steps:     steps,
		Status:    "running",
	}
}

// GetReadySteps returns all steps whose dependencies are satisfied and status is "pending".
func (sm *StateManager) GetReadySteps(run *WorkflowRun) []Step {
	run.mu.RLock()
	defer run.mu.RUnlock()

	var ready []Step
	for _, ss := range run.Steps {
		if ss.Status != "pending" {
			continue
		}
		if sm.depsComplete(run, ss.Step.DependsOn) {
			ready = append(ready, ss.Step)
		}
	}
	return ready
}

// MarkStep updates a step's status.
func (sm *StateManager) MarkStep(run *WorkflowRun, stepID, status string) {
	run.mu.Lock()
	defer run.mu.Unlock()
	if ss, ok := run.Steps[stepID]; ok {
		ss.Status = status
	}
}

// IsComplete returns true when all steps are in a terminal state.
func (sm *StateManager) IsComplete(run *WorkflowRun) bool {
	run.mu.RLock()
	defer run.mu.RUnlock()
	for _, ss := range run.Steps {
		switch ss.Status {
		case "completed", "skipped", "failed":
			continue
		default:
			return false
		}
	}
	return true
}

func (sm *StateManager) depsComplete(run *WorkflowRun, deps []string) bool {
	for _, dep := range deps {
		ss, ok := run.Steps[dep]
		if !ok {
			return false
		}
		if ss.Status != "completed" && ss.Status != "skipped" {
			return false
		}
	}
	return true
}
