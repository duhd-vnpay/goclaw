package ardenn

import (
	"time"

	"github.com/google/uuid"
)

// RunState is the in-memory projection of a workflow run, built from events.
type RunState struct {
	ID           uuid.UUID
	TenantID     uuid.UUID
	WorkflowID   uuid.UUID
	ProjectID    *uuid.UUID
	TriggeredBy  *uuid.UUID
	Variables    map[string]any
	Tier         Tier
	Status       string
	StepRuns     map[uuid.UUID]*StepRunState
	LastSequence int64
	StartedAt    *time.Time
	CompletedAt  *time.Time
	// UserPermissions is the resolved permission map of the triggering user
	// (from Identity UserProfile). Populated by Engine.StartRun when a
	// ProfileResolver is wired. Not persisted in events — re-resolved on
	// rebuild via the same resolver, or left nil for guards to fall back to
	// their own checker.
	UserPermissions map[string]bool
}

// StepRunState is the in-memory projection of a single step within a run.
type StepRunState struct {
	ID            uuid.UUID
	StepID        uuid.UUID
	Status        string
	AssignedUser  *uuid.UUID
	AssignedAgent *uuid.UUID
	HandType      string
	Result        string
	DispatchCount int
	EvalRound     int
	EvalScore     float64
	EvalPassed    *bool
	GateStatus    string
	GateDecidedBy *uuid.UUID
	DependsOn     []uuid.UUID
	Metadata      map[string]any
	LastSequence  int64
}

// Apply folds a single event into the run state, mutating it in place.
func (s *RunState) Apply(e Event) {
	s.LastSequence = e.Sequence

	switch e.Type {
	case EventRunCreated:
		s.ID = e.RunID
		s.TenantID = e.TenantID
		s.Status = "pending"
		if wfID, ok := e.Payload["workflow_id"].(string); ok {
			if id, err := uuid.Parse(wfID); err == nil {
				s.WorkflowID = id
			}
		}
		if tierStr, ok := e.Payload["tier"].(string); ok {
			s.Tier, _ = ParseTier(tierStr)
		}
		if vars, ok := e.Payload["variables"].(map[string]any); ok {
			s.Variables = vars
		}
		if tb, ok := e.Payload["triggered_by"].(string); ok && tb != "" {
			if id, err := uuid.Parse(tb); err == nil {
				s.TriggeredBy = &id
			}
		}
		if s.StepRuns == nil {
			s.StepRuns = map[uuid.UUID]*StepRunState{}
		}
	case EventRunStarted:
		s.Status = "running"
		now := time.Now()
		s.StartedAt = &now
	case EventRunCompleted:
		s.Status = "completed"
		now := time.Now()
		s.CompletedAt = &now
	case EventRunFailed:
		s.Status = "failed"
		now := time.Now()
		s.CompletedAt = &now
	case EventRunCancelled:
		s.Status = "cancelled"
		now := time.Now()
		s.CompletedAt = &now
	case EventRunPaused:
		s.Status = "paused"
	case EventRunResumed:
		s.Status = "running"
	}

	// Step-level events require a StepID.
	if e.StepID == nil {
		return
	}
	stepID := *e.StepID
	sr, ok := s.StepRuns[stepID]
	if !ok {
		sr = &StepRunState{StepID: stepID, Metadata: map[string]any{}}
		s.StepRuns[stepID] = sr
	}
	sr.LastSequence = e.Sequence

	switch e.Type {
	case EventStepReady:
		sr.Status = "pending"
	case EventStepDispatched:
		sr.Status = "running"
		if ht, ok := e.Payload["hand_type"].(string); ok {
			sr.HandType = ht
		}
		if dc, ok := e.Payload["dispatch_count"].(float64); ok {
			sr.DispatchCount = int(dc)
		}
	case EventStepResult:
		if out, ok := e.Payload["output"].(string); ok {
			sr.Result = out
		}
	case EventStepCompleted:
		sr.Status = "completed"
	case EventStepFailed:
		sr.Status = "failed"
	case EventStepSkipped:
		sr.Status = "skipped"
	case EventStepCancelled:
		sr.Status = "cancelled"
	case EventGatePending:
		sr.Status = "waiting_gate"
		sr.GateStatus = "pending"
	case EventGateApproved:
		sr.GateStatus = "approved"
		if by, ok := e.Payload["decided_by"].(string); ok {
			if id, err := uuid.Parse(by); err == nil {
				sr.GateDecidedBy = &id
			}
		}
	case EventGateRejected:
		sr.GateStatus = "rejected"
		sr.Status = "running"
	case EventEvalRoundPassed:
		if score, ok := e.Payload["score"].(float64); ok {
			sr.EvalScore = score
		}
		passed := true
		sr.EvalPassed = &passed
	case EventEvalRoundFailed:
		if round, ok := e.Payload["round"].(float64); ok {
			sr.EvalRound = int(round)
		}
	}
}

// GetReadySteps returns step IDs that are pending and have all dependencies completed.
func (s *RunState) GetReadySteps() []uuid.UUID {
	var ready []uuid.UUID
	for stepID, sr := range s.StepRuns {
		if sr.Status != "pending" {
			continue
		}
		allDepsComplete := true
		for _, depID := range sr.DependsOn {
			dep, ok := s.StepRuns[depID]
			if !ok || dep.Status != "completed" {
				allDepsComplete = false
				break
			}
		}
		if allDepsComplete {
			ready = append(ready, stepID)
		}
	}
	return ready
}

// IsTerminal returns true when the run has reached a final status.
func (s *RunState) IsTerminal() bool {
	switch s.Status {
	case "completed", "failed", "cancelled":
		return true
	default:
		return false
	}
}

// HasFailedSteps returns true if any step in the run has failed.
func (s *RunState) HasFailedSteps() bool {
	for _, sr := range s.StepRuns {
		if sr.Status == "failed" {
			return true
		}
	}
	return false
}
