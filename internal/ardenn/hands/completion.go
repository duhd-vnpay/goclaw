package hands

import (
	"sync"

	engine "github.com/nextlevelbuilder/goclaw/internal/ardenn"

	"github.com/google/uuid"
)

// CompletionRegistry tracks pending hand executions and delivers results.
// AgentHand registers a channel before dispatch; post-turn processing
// completes it when the agent finishes.
type CompletionRegistry struct {
	mu       sync.RWMutex
	channels map[uuid.UUID]chan engine.HandResult
}

func NewCompletionRegistry() *CompletionRegistry {
	return &CompletionRegistry{
		channels: make(map[uuid.UUID]chan engine.HandResult),
	}
}

func (r *CompletionRegistry) Register(stepRunID uuid.UUID) <-chan engine.HandResult {
	ch := make(chan engine.HandResult, 1)
	r.mu.Lock()
	r.channels[stepRunID] = ch
	r.mu.Unlock()
	return ch
}

func (r *CompletionRegistry) Complete(stepRunID uuid.UUID, result engine.HandResult) {
	r.mu.Lock()
	ch, ok := r.channels[stepRunID]
	if ok {
		delete(r.channels, stepRunID)
	}
	r.mu.Unlock()
	if ok {
		ch <- result
	}
}

func (r *CompletionRegistry) Deregister(stepRunID uuid.UUID) {
	r.mu.Lock()
	delete(r.channels, stepRunID)
	r.mu.Unlock()
}
