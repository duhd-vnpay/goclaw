package ardenn

import (
	"context"
	"sort"

	"github.com/google/uuid"
)

const (
	ContinuityCompaction = "compaction"
	ContinuityReset      = "reset"
	ContinuityAdaptive   = "adaptive"

	maxGlobalEvents   = 20
	adaptiveThreshold = 100
)

// ContextBuilder constructs step-level context from the event stream (L2).
type ContextBuilder struct {
	events EventStore
}

func NewContextBuilder(events EventStore) *ContextBuilder {
	return &ContextBuilder{events: events}
}

// BuildStepContext returns the events relevant to a step's execution context.
func (cb *ContextBuilder) BuildStepContext(ctx context.Context, run *RunState, step *StepDef) ([]Event, error) {
	strategy := cb.resolveStrategy(ctx, run, step)

	switch strategy {
	case ContinuityReset:
		return cb.buildResetContext(ctx, run, step)
	case ContinuityCompaction:
		return cb.buildCompactionContext(ctx, run, step)
	default:
		return cb.buildCompactionContext(ctx, run, step)
	}
}

func (cb *ContextBuilder) resolveStrategy(ctx context.Context, run *RunState, step *StepDef) string {
	if s, ok := step.Continuity["strategy"].(string); ok && s != "" {
		if s != ContinuityAdaptive {
			return s
		}
		// Adaptive: check event count
		total, err := cb.events.GetLastSequence(ctx, run.ID)
		if err != nil || total < adaptiveThreshold {
			return ContinuityCompaction
		}
		return ContinuityReset
	}
	return ContinuityCompaction
}

func (cb *ContextBuilder) buildCompactionContext(ctx context.Context, run *RunState, step *StepDef) ([]Event, error) {
	seen := map[uuid.UUID]bool{}
	var result []Event

	// 1. Gather dependency step results
	sr := run.StepRuns[step.ID]
	if sr != nil {
		for _, depID := range sr.DependsOn {
			depEvents, err := cb.events.GetEvents(ctx, EventQuery{
				RunID:  run.ID,
				StepID: &depID,
				Limit:  10,
			})
			if err != nil {
				return nil, err
			}
			for _, e := range depEvents {
				if !seen[e.ID] {
					seen[e.ID] = true
					result = append(result, e)
				}
			}
		}
	}

	// 2. Append recent global events (no step ID)
	globalEvents, err := cb.events.GetEvents(ctx, EventQuery{
		RunID: run.ID,
		Limit: maxGlobalEvents,
	})
	if err != nil {
		return nil, err
	}
	for _, e := range globalEvents {
		if e.StepID == nil && !seen[e.ID] {
			seen[e.ID] = true
			result = append(result, e)
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Sequence < result[j].Sequence
	})
	return result, nil
}

func (cb *ContextBuilder) buildResetContext(ctx context.Context, run *RunState, step *StepDef) ([]Event, error) {
	stepID := step.ID
	return cb.events.GetEvents(ctx, EventQuery{
		RunID:  run.ID,
		StepID: &stepID,
		Limit:  50,
	})
}
