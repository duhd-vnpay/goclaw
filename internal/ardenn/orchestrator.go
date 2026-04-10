package ardenn

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

// Orchestrator is the stateless workflow engine. It rebuilds state from events,
// finds ready steps, executes them, and emits terminal events when done.
// Crash recovery is built-in: just call ProcessRun again after restart.
type Orchestrator struct {
	events    EventStore
	projector *Projector
	executor  *StepExecutor
	stepDefs  map[uuid.UUID]map[uuid.UUID]*StepDef // runID → stepID → def
}

// NewOrchestrator creates an Orchestrator wired to the given event store, projector, and executor.
func NewOrchestrator(events EventStore, projector *Projector, executor *StepExecutor) *Orchestrator {
	return &Orchestrator{
		events:    events,
		projector: projector,
		executor:  executor,
		stepDefs:  map[uuid.UUID]map[uuid.UUID]*StepDef{},
	}
}

// RegisterStepDefs associates step definitions with a run so the orchestrator
// knows how to execute each step.
func (o *Orchestrator) RegisterStepDefs(runID uuid.UUID, defs map[uuid.UUID]*StepDef) {
	o.stepDefs[runID] = defs
}

// ProcessRun rebuilds state from events and drives the run forward.
func (o *Orchestrator) ProcessRun(ctx context.Context, runID uuid.UUID) error {
	state, err := o.projector.Rebuild(ctx, runID)
	if err != nil {
		return fmt.Errorf("rebuild state: %w", err)
	}
	return o.ProcessRunWithState(ctx, runID, state)
}

// ProcessRunWithState drives a run forward from the given state snapshot.
// It loops: find ready steps → execute → update projection, until the run
// is terminal or no more steps are ready (park).
func (o *Orchestrator) ProcessRunWithState(ctx context.Context, runID uuid.UUID, state *RunState) error {
	defs := o.stepDefs[runID]
	if defs == nil {
		return fmt.Errorf("no step definitions registered for run %s", runID)
	}

	for !state.IsTerminal() {
		readySteps := state.GetReadySteps()
		if len(readySteps) == 0 {
			// Check if all steps are in a terminal status (no more work possible).
			allDone := true
			for _, sr := range state.StepRuns {
				switch sr.Status {
				case "completed", "failed", "skipped", "cancelled":
					continue
				default:
					allDone = false
				}
			}
			if allDone {
				break
			}
			// Some steps are still in-progress or blocked — park until woken.
			return nil
		}

		for _, stepID := range readySteps {
			def, ok := defs[stepID]
			if !ok {
				continue
			}
			if err := o.executor.Execute(ctx, state, def); err != nil {
				return fmt.Errorf("execute step %s: %w", stepID, err)
			}
			// Re-project state so subsequent iterations see updated statuses.
			if err := o.projector.Update(ctx, state); err != nil {
				return fmt.Errorf("update projection: %w", err)
			}
		}
	}

	// Emit terminal event.
	eventType := EventRunCompleted
	if state.HasFailedSteps() {
		eventType = EventRunFailed
	}
	o.events.Emit(ctx, Event{
		TenantID:  state.TenantID,
		RunID:     runID,
		Type:      eventType,
		ActorType: ActorEngine,
		Payload:   map[string]any{},
	})

	return nil
}

// Wake resumes processing for a run — used after external events (gate approval,
// user completion, timer) to re-drive the workflow.
func (o *Orchestrator) Wake(ctx context.Context, runID uuid.UUID) error {
	return o.ProcessRun(ctx, runID)
}
