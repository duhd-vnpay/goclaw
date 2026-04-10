package ardenn

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Event represents a single immutable fact in the Ardenn event log.
type Event struct {
	ID        uuid.UUID      `db:"id"         json:"id"`
	TenantID  uuid.UUID      `db:"tenant_id"  json:"tenant_id"`
	RunID     uuid.UUID      `db:"run_id"     json:"run_id"`
	StepID    *uuid.UUID     `db:"step_id"    json:"step_id,omitempty"`
	Sequence  int64          `db:"sequence"   json:"sequence"`
	Type      string         `db:"event_type" json:"event_type"`
	ActorType string         `db:"actor_type" json:"actor_type"`
	ActorID   *uuid.UUID     `db:"actor_id"   json:"actor_id,omitempty"`
	Payload   map[string]any `db:"payload"    json:"payload"`
	CreatedAt time.Time      `db:"created_at" json:"created_at"`
}

const (
	EventRunCreated   = "run.created"
	EventRunStarted   = "run.started"
	EventRunCompleted = "run.completed"
	EventRunFailed    = "run.failed"
	EventRunCancelled = "run.cancelled"
	EventRunPaused    = "run.paused"
	EventRunResumed   = "run.resumed"

	EventStepReady      = "step.ready"
	EventStepDispatched = "step.dispatched"
	EventStepProgress   = "step.progress"
	EventStepResult     = "step.result"
	EventStepFailed     = "step.failed"
	EventStepSkipped    = "step.skipped"
	EventStepCancelled  = "step.cancelled"
	EventStepCompleted  = "step.completed"

	EventEvalStarted      = "eval.started"
	EventEvalSensorResult = "eval.sensor_result"
	EventEvalRoundPassed  = "eval.round_passed"
	EventEvalRoundFailed  = "eval.round_failed"
	EventEvalEscalated    = "eval.escalated"

	EventConstraintChecked  = "constraint.checked"
	EventConstraintViolated = "constraint.violated"

	EventGatePending    = "gate.pending"
	EventGateApproved   = "gate.approved"
	EventGateRejected   = "gate.rejected"
	EventGateAutoPassed = "gate.auto_passed"
	EventGateTimedOut   = "gate.timed_out"

	EventHandInvoked  = "hand.invoked"
	EventHandReturned = "hand.returned"
	EventHandFailed   = "hand.failed"
	EventHandRetried  = "hand.retried"

	EventContinuityCheckpoint = "continuity.checkpoint"
	EventContinuityResume     = "continuity.resume"
)

const (
	ActorUser   = "user"
	ActorAgent  = "agent"
	ActorSystem = "system"
	ActorEngine = "engine"
)

// EventQuery controls event retrieval from the store.
type EventQuery struct {
	RunID        uuid.UUID
	FromSequence int64
	StepID       *uuid.UUID
	EventType    string
	Limit        int
}

// EventStore is the append-only event log — source of truth for Ardenn.
type EventStore interface {
	Emit(ctx context.Context, e Event) (int64, error)
	GetEvents(ctx context.Context, q EventQuery) ([]Event, error)
	GetLastSequence(ctx context.Context, runID uuid.UUID) (int64, error)
}
