package ardenn

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type HandType string

const (
	HandAgent HandType = "agent"
	HandUser  HandType = "user"
	HandAPI   HandType = "api"
	HandMCP   HandType = "mcp"
)

// MaxDispatches is the circuit breaker limit for AgentHand.
const MaxDispatches = 3

type HandRequest struct {
	RunID     uuid.UUID
	StepRunID uuid.UUID
	Name      string
	Input     string
	Context   []Event
	Metadata  map[string]any
	Timeout   time.Duration
}

type HandResult struct {
	Output   string
	Error    error
	Duration time.Duration
}

type Hand interface {
	Type() HandType
	Execute(ctx context.Context, req HandRequest) HandResult
	Cancel(ctx context.Context, runID uuid.UUID) error
}

type HandRegistry struct {
	hands map[HandType]Hand
}

func NewHandRegistry() *HandRegistry {
	return &HandRegistry{hands: map[HandType]Hand{}}
}

func (r *HandRegistry) Register(h Hand) {
	r.hands[h.Type()] = h
}

func (r *HandRegistry) Get(ht HandType) (Hand, error) {
	h, ok := r.hands[ht]
	if !ok {
		return nil, fmt.Errorf("unknown hand type: %s", ht)
	}
	return h, nil
}

func ResolveHandType(dispatchTo string) HandType {
	switch dispatchTo {
	case "user", "role", "department":
		return HandUser
	case "api":
		return HandAPI
	case "mcp":
		return HandMCP
	default:
		return HandAgent
	}
}
