package ardenn

import (
	"context"

	"github.com/google/uuid"
)

// Projector rebuilds or incrementally updates a RunState from the event store.
type Projector struct {
	store EventStore
}

// NewProjector creates a Projector backed by the given EventStore.
func NewProjector(store EventStore) *Projector {
	return &Projector{store: store}
}

// Rebuild replays all events for a run to produce a fresh RunState.
func (p *Projector) Rebuild(ctx context.Context, runID uuid.UUID) (*RunState, error) {
	events, err := p.store.GetEvents(ctx, EventQuery{RunID: runID, FromSequence: 0})
	if err != nil {
		return nil, err
	}
	state := &RunState{StepRuns: map[uuid.UUID]*StepRunState{}}
	for _, e := range events {
		state.Apply(e)
	}
	return state, nil
}

// Update applies only the events that occurred after the state's last known sequence.
func (p *Projector) Update(ctx context.Context, state *RunState) error {
	events, err := p.store.GetEvents(ctx, EventQuery{
		RunID:        state.ID,
		FromSequence: state.LastSequence,
	})
	if err != nil {
		return err
	}
	for _, e := range events {
		state.Apply(e)
	}
	return nil
}
