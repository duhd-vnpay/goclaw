package ardenn

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
)

// Wiring Guide (for consumer/gateway integration):
//
//	completion := hands.NewCompletionRegistry()
//	agentHand := hands.NewAgentHand(msgBus, completion)
//	registry := ardenn.NewHandRegistry()
//	registry.Register(agentHand)
//	engine := ardenn.NewEngine(eventStore, registry)
//
// In post-turn processing:
//
//	hands.ResolveAgentCompletion(metadata, result, err, completion)
//
// Engine is the unified entry point for Ardenn. Wires all components.
type Engine struct {
	events          EventStore
	projector       *Projector
	executor        *StepExecutor
	orchestrator    *Orchestrator
	resourceLoader  ProjectResourceLoader
	profileResolver ProfileResolver
}

// ProfileResolver is the minimal Ardenn-facing slice of store.ProfileResolver.
// Defined locally to keep internal/ardenn free of internal/store deps.
// PGProfileResolver in internal/store/pg satisfies this interface implicitly.
type ProfileResolver interface {
	ResolveByUserID(ctx context.Context, userID uuid.UUID) (*ResolvedUserProfile, error)
	IncrementWorkload(ctx context.Context, userID uuid.UUID) error
	DecrementWorkload(ctx context.Context, userID uuid.UUID) error
}

// ResolvedUserProfile is the projected slice of store.UserProfile that
// Ardenn cares about — just enough to feed guards and workload tracking.
type ResolvedUserProfile struct {
	ID          uuid.UUID
	Permissions map[string]bool
}

// EngineOptions holds optional components for the Ardenn engine.
type EngineOptions struct {
	Constraints     *ConstraintEngine
	EvalPipeline    *ArdennEvalPipeline
	ResourceLoader  ProjectResourceLoader
	ArtifactStore   ArdennArtifactStore
	ProfileResolver ProfileResolver
}

// NewEngine creates a fully wired Ardenn engine.
// opts is variadic for backward compatibility — existing callers without opts still work.
func NewEngine(events EventStore, hands *HandRegistry, opts ...EngineOptions) *Engine {
	projector := NewProjector(events)
	gates := NewGateKeeper(events)

	var constraints *ConstraintEngine
	var evalPipeline *ArdennEvalPipeline
	var resourceLoader ProjectResourceLoader
	var profileResolver ProfileResolver
	var contextBuilder *ContextBuilder
	var checkpointer *Checkpointer
	if len(opts) > 0 {
		constraints = opts[0].Constraints
		evalPipeline = opts[0].EvalPipeline
		resourceLoader = opts[0].ResourceLoader
		profileResolver = opts[0].ProfileResolver
		if opts[0].ArtifactStore != nil {
			contextBuilder = NewContextBuilder(events)
			checkpointer = NewCheckpointer(events, opts[0].ArtifactStore)
		}
	}

	executor := &StepExecutor{
		events:          events,
		hands:           hands,
		gates:           gates,
		constraints:    constraints,
		evalPipeline:   evalPipeline,
		contextBuilder: contextBuilder,
		checkpointer:   checkpointer,
		profileResolver: profileResolver,
	}
	orchestrator := NewOrchestrator(events, projector, executor)

	return &Engine{
		events:          events,
		projector:       projector,
		executor:        executor,
		orchestrator:    orchestrator,
		resourceLoader:  resourceLoader,
		profileResolver: profileResolver,
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

	// Resolve user permissions for triggering user (Identity-integrated path).
	// Best-effort: log + continue without permissions if resolution fails.
	var userPerms map[string]bool
	if req.TriggeredBy != nil && e.profileResolver != nil {
		profile, err := e.profileResolver.ResolveByUserID(ctx, *req.TriggeredBy)
		if err != nil {
			slog.Warn("ardenn: failed to resolve user profile",
				"user_id", req.TriggeredBy, "error", err)
		} else if profile != nil {
			userPerms = profile.Permissions
		}
	}

	// Inject project resources into variables
	if req.ProjectID != nil && e.resourceLoader != nil {
		projectVars, err := ResolveProjectVariables(ctx, e.resourceLoader, *req.ProjectID)
		if err != nil {
			// Non-fatal: log and continue without project vars
			slog.Warn("ardenn: failed to resolve project variables",
				"project_id", req.ProjectID, "error", err)
		} else {
			req.Variables = MergeVariables(req.Variables, projectVars)
		}
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

	// Inject resolved user permissions into the projected state so guards
	// can check them inline. Not persisted in events — re-resolved on rebuild.
	state.UserPermissions = userPerms

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

// RegisterStepDefs associates step definitions with a run.
// Called during startup recovery to restore in-memory defs lost on pod restart.
func (e *Engine) RegisterStepDefs(runID uuid.UUID, defs map[uuid.UUID]*StepDef) {
	e.orchestrator.RegisterStepDefs(runID, defs)
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

// GateDecide records an approve/reject decision for a step's gate and wakes the run.
func (e *Engine) GateDecide(ctx context.Context, runID, stepID uuid.UUID, approved bool, actorID *uuid.UUID, feedback string) error {
	eventType := EventGateApproved
	if !approved {
		eventType = EventGateRejected
	}
	payload := map[string]any{"feedback": feedback}
	if actorID != nil {
		payload["decided_by"] = actorID.String()
	}

	// Fetch the run's TenantID from the existing run.created event.
	// Required because ardenn_events has a FK constraint on tenant_id.
	var tenantID uuid.UUID
	runEvents, err := e.events.GetEvents(ctx, EventQuery{
		RunID:     runID,
		EventType: EventRunCreated,
		Limit:     1,
	})
	if err != nil {
		return fmt.Errorf("fetch run tenant: %w", err)
	}
	if len(runEvents) > 0 {
		tenantID = runEvents[0].TenantID
	}

	_, err = e.events.Emit(ctx, Event{
		TenantID:  tenantID,
		RunID:     runID,
		StepID:    &stepID,
		Type:      eventType,
		ActorType: ActorUser,
		ActorID:   actorID,
		Payload:   payload,
	})
	if err != nil {
		return fmt.Errorf("emit gate decision: %w", err)
	}

	// On approval: emit step.completed so that after Rebuild(), the step's
	// status is "completed" and downstream dependents are unlocked.
	// Without this, gate.approved leaves status="waiting_gate" and the
	// orchestrator parks again (no ready steps, not all done).
	if approved {
		e.events.Emit(ctx, Event{
			TenantID:  tenantID,
			RunID:     runID,
			StepID:    &stepID,
			Type:      EventStepCompleted,
			ActorType: ActorEngine,
			Payload:   map[string]any{},
		})
	}

	return e.Wake(ctx, runID)
}

func uuidStr(u *uuid.UUID) string {
	if u == nil {
		return ""
	}
	return u.String()
}
