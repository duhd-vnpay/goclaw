package continuity

// ContextStrategy determines how context transitions are handled.
type ContextStrategy string

const (
	StrategyCompaction ContextStrategy = "compaction"
	StrategyReset      ContextStrategy = "reset"
	StrategyAdaptive   ContextStrategy = "adaptive"
)

// AdaptiveStrategyConfig configures when adaptive strategy switches to reset.
type AdaptiveStrategyConfig struct {
	ContextUsagePct int  `json:"context_usage_pct"`
	MessageCount    int  `json:"message_count"`
	TaskBoundary    bool `json:"task_boundary"`
}

// SessionState holds the current session metrics for strategy resolution.
type SessionState struct {
	ContextUsagePct int
	MessageCount    int
	IsTaskBoundary  bool
	IsLongRunning   bool
	ExplicitReset   bool
}

// StrategyResolver decides which context strategy to use.
type StrategyResolver struct {
	strategy string
	adaptive AdaptiveStrategyConfig
}

// NewStrategyResolver creates a resolver with the given base strategy and adaptive config.
func NewStrategyResolver(strategy string, adaptive AdaptiveStrategyConfig) *StrategyResolver {
	return &StrategyResolver{strategy: strategy, adaptive: adaptive}
}

// Resolve determines the context strategy for the given session state.
func (r *StrategyResolver) Resolve(state SessionState) ContextStrategy {
	switch r.strategy {
	case "reset":
		return StrategyReset
	case "compaction":
		return StrategyCompaction
	case "adaptive":
		return r.resolveAdaptive(state)
	default:
		return StrategyCompaction
	}
}

func (r *StrategyResolver) resolveAdaptive(state SessionState) ContextStrategy {
	if state.ExplicitReset {
		return StrategyReset
	}
	if r.adaptive.TaskBoundary && state.IsTaskBoundary {
		return StrategyReset
	}
	if state.ContextUsagePct >= r.adaptive.ContextUsagePct {
		if state.IsLongRunning || state.MessageCount >= r.adaptive.MessageCount {
			return StrategyReset
		}
		return StrategyCompaction
	}
	if state.MessageCount >= r.adaptive.MessageCount {
		return StrategyReset
	}
	return StrategyCompaction
}
