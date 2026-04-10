package ardenn

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

// Engine is the unified entry point for Ardenn. Wires all components.
type Engine struct {
	events       EventStore
	projector    *Projector
	executor     *StepExecutor
	orchestrator *Orchestrator
}

// NewEngine creates a fully wired Ardenn engine.
func NewEngine(events EventStore, hands *HandRegistry) *Engine {
	projector := NewProjector(events)
	gates := NewGateKeeper(events)
	executor := &StepExecutor{
		events: events,
		hands:  hands,
		gates:  gates,
	}
	orchestrator := NewOrchestrator(events, projector, executor)

	return &Engine{
		events:       events,
		projector:    projector,
		executor:     executor,
		orchestrator: orchestrator,
	}
}

// StartRunRequest contains parameters to start a new workflow run.
type StartRunRequest struct {
	TenantID    uuid.UUID
	WorkflowID  uuid.UUID
	ProjectID   *uuid.UUID
	TriggeredBy *uuid.UUID
	Tier        string
	Variables   map[string]any
	StepDefs    map[uuid.UUID]*StepDef
}

// StartRun creates a new workflow run and begins execution.
func (e *Engine) StartRun(ctx context.Context, req StartRunRequest) (uuid.UUID, error) {
	runID := uuid.New()
	tier, err := ParseTier(req.Tier)
	if err != nil {
		return uuid.Nil, fmt.Errorf("invalid tier: %w", err)
	}

	// Emit run.created
	_, err = e.events.Emit(ctx, Event{
		TenantID:  req.TenantID,
		RunID:     runID,
		Type:      EventRunCreated,
		ActorType: ActorEngine,
		Payload: map[string]any{
			"workflow_id":  req.WorkflowID.String(),
			"tier":         tier.String(),
			"variables":    req.Variables,
			"triggered_by": uuidStr(req.TriggeredBy),
		},
	})
	if err != nil {
		return uuid.Nil, fmt.Errorf("emit run.created: %w", err)
	}

	// Initialize step runs
	for stepID := range req.StepDefs {
		sid := stepID
		e.events.Emit(ctx, Event{
			TenantID: req.TenantID, RunID: runID, StepID: &sid,
			Type: EventStepReady, ActorType: ActorEngine,
		})
	}

	// Emit run.started
	e.events.Emit(ctx, Event{
		TenantID: req.TenantID, RunID: runID,
		Type: EventRunStarted, ActorType: ActorEngine,
	})

	// Register step defs and set dependencies in projected state
	e.orchestrator.RegisterStepDefs(runID, req.StepDefs)

	state, err := e.projector.Rebuild(ctx, runID)
	if err != nil {
		return runID, fmt.Errorf("rebuild: %w", err)
	}

	// Set dependencies from step defs into state
	for stepID, def := range req.StepDefs {
		if sr, ok := state.StepRuns[stepID]; ok {
			sr.DependsOn = def.DependsOn
		}
	}

	// Begin execution asynchronously
	go e.orchestrator.ProcessRunWithState(context.Background(), runID, state)

	return runID, nil
}

// Wake resumes processing of a run after an external event.
func (e *Engine) Wake(ctx context.Context, runID uuid.UUID) error {
	return e.orchestrator.Wake(ctx, runID)
}

// GetRunState returns the current projected state of a run.
func (e *Engine) GetRunState(ctx context.Context, runID uuid.UUID) (*RunState, error) {
	return e.projector.Rebuild(ctx, runID)
}

// GetEvents returns events for a run.
func (e *Engine) GetEvents(ctx context.Context, q EventQuery) ([]Event, error) {
	return e.events.GetEvents(ctx, q)
}

func uuidStr(u *uuid.UUID) string {
	if u == nil {
		return ""
	}
	return u.String()
}
