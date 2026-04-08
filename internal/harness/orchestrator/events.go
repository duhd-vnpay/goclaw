package orchestrator

import (
	"sync"
	"time"

	"github.com/google/uuid"
)

// WorkflowEvent is emitted during workflow execution.
type WorkflowEvent struct {
	Type      string    `json:"type"` // "workflow.started","step.completed","gate.pending", etc.
	RunID     uuid.UUID `json:"run_id"`
	StepID    string    `json:"step_id,omitempty"`
	Payload   any       `json:"payload,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// EventHandler processes workflow events.
type EventHandler func(event WorkflowEvent)

// EventBus provides pub/sub for workflow events.
type EventBus struct {
	handlers map[string][]EventHandler
	mu       sync.RWMutex
}

// NewEventBus creates a new event bus.
func NewEventBus() *EventBus {
	return &EventBus{handlers: make(map[string][]EventHandler)}
}

// Subscribe registers a handler for the given event type.
func (b *EventBus) Subscribe(eventType string, handler EventHandler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers[eventType] = append(b.handlers[eventType], handler)
}

// Emit dispatches an event to all registered handlers (async).
func (b *EventBus) Emit(eventType string, runID uuid.UUID, stepID string, payload any) {
	b.mu.RLock()
	handlers := b.handlers[eventType]
	b.mu.RUnlock()

	event := WorkflowEvent{
		Type:      eventType,
		RunID:     runID,
		StepID:    stepID,
		Payload:   payload,
		Timestamp: time.Now(),
	}

	for _, h := range handlers {
		go h(event)
	}
}
