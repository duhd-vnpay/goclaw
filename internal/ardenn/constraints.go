// internal/ardenn/constraints.go
package ardenn

import (
	"context"

	"github.com/google/uuid"
)

// ConstraintContext carries the data guards need to evaluate constraints.
type ConstraintContext struct {
	TenantID  uuid.UUID
	RunID     uuid.UUID
	StepID    uuid.UUID
	AgentKey  string
	UserID    *uuid.UUID
	Input     string
	Variables map[string]any
	Metadata  map[string]any
	// UserPermissions, when non-nil, lets guards check permissions inline
	// without an external lookup. Populated by the engine from UserProfile
	// resolved at run start. If nil, guards fall back to their own
	// PermissionChecker (legacy / standalone path).
	UserPermissions map[string]bool
}

// GuardResult is the outcome of a single guard check.
type GuardResult struct {
	Name     string `json:"name"`
	Pass     bool   `json:"pass"`
	Reason   string `json:"reason,omitempty"`
	Severity string `json:"severity"` // "block" or "warn"
}

// Guard is a single constraint check. Guards are composable.
type Guard interface {
	Name() string
	Check(ctx context.Context, cc ConstraintContext) GuardResult
}

// ConstraintResult aggregates all guard results for a step.
type ConstraintResult struct {
	Pass    bool          `json:"pass"`
	Blocked bool          `json:"blocked"`
	Results []GuardResult `json:"results"`
}

// ConstraintEngine runs all registered guards and emits events.
type ConstraintEngine struct {
	guards []Guard
	events EventStore
}

// NewConstraintEngine creates a constraint engine with the given guards.
func NewConstraintEngine(events EventStore, guards ...Guard) *ConstraintEngine {
	return &ConstraintEngine{guards: guards, events: events}
}

// Check runs all guards against the constraint context. Returns early on first blocking failure.
func (ce *ConstraintEngine) Check(ctx context.Context, cc ConstraintContext) ConstraintResult {
	var results []GuardResult
	blocked := false

	for _, g := range ce.guards {
		result := g.Check(ctx, cc)
		results = append(results, result)

		stepID := cc.StepID
		if !result.Pass {
			ce.events.Emit(ctx, Event{
				TenantID:  cc.TenantID,
				RunID:     cc.RunID,
				StepID:    &stepID,
				Type:      EventConstraintViolated,
				ActorType: ActorEngine,
				Payload: map[string]any{
					"guard":    result.Name,
					"reason":   result.Reason,
					"severity": result.Severity,
				},
			})
			if result.Severity == "block" {
				blocked = true
				break
			}
		}
	}

	allPassed := !blocked
	for _, r := range results {
		if !r.Pass && r.Severity == "block" {
			allPassed = false
			break
		}
	}

	stepID := cc.StepID
	ce.events.Emit(ctx, Event{
		TenantID:  cc.TenantID,
		RunID:     cc.RunID,
		StepID:    &stepID,
		Type:      EventConstraintChecked,
		ActorType: ActorEngine,
		Payload: map[string]any{
			"pass":         allPassed,
			"guard_count":  len(ce.guards),
			"failed_count": countFailed(results),
		},
	})

	return ConstraintResult{
		Pass:    allPassed,
		Blocked: blocked,
		Results: results,
	}
}

func countFailed(results []GuardResult) int {
	count := 0
	for _, r := range results {
		if !r.Pass {
			count++
		}
	}
	return count
}
