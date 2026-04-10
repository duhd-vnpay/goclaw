package constraints

import "sort"

// phaseOrder returns a numeric ordering for phases so guards are consistently
// sorted: BeforeRun < BeforeToolCall < AfterToolCall < AfterRun.
func phaseOrder(p Phase) int {
	switch p {
	case BeforeRun:
		return 0
	case BeforeToolCall:
		return 1
	case AfterToolCall:
		return 2
	case AfterRun:
		return 3
	default:
		return 99
	}
}

// kindOrder returns a numeric ordering for kinds: Computational runs before
// Inferential so cheap, deterministic checks happen first.
func kindOrder(k Kind) int {
	switch k {
	case Computational:
		return 0
	case Inferential:
		return 1
	default:
		return 99
	}
}

// GuardRegistry holds all registered guards and runs them in phase/kind order.
type GuardRegistry struct {
	guards []Guard
}

// NewGuardRegistry returns an initialised, empty GuardRegistry.
func NewGuardRegistry() *GuardRegistry {
	return &GuardRegistry{}
}

// Register adds a guard to the registry and re-sorts by Phase then Kind
// (computational before inferential within the same phase).
func (r *GuardRegistry) Register(g Guard) {
	r.guards = append(r.guards, g)
	sort.SliceStable(r.guards, func(i, j int) bool {
		pi, pj := phaseOrder(r.guards[i].Phase()), phaseOrder(r.guards[j].Phase())
		if pi != pj {
			return pi < pj
		}
		return kindOrder(r.guards[i].Kind()) < kindOrder(r.guards[j].Kind())
	})
}

// RunPhase executes all guards registered for the given phase in order.
// It stops immediately when any guard returns an "block" action and returns
// only the results collected up to and including the blocking guard.
func (r *GuardRegistry) RunPhase(phase Phase, ctx GuardContext) []GuardResult {
	var results []GuardResult
	for _, g := range r.guards {
		if g.Phase() != phase {
			continue
		}
		result := g.Check(ctx)
		results = append(results, result)
		if result.Action == "block" {
			break
		}
	}
	return results
}
