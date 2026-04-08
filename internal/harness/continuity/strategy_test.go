package continuity

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStrategyResolver_Adaptive_ResetOnHighContext(t *testing.T) {
	cfg := AdaptiveStrategyConfig{ContextUsagePct: 70, MessageCount: 40, TaskBoundary: true}
	r := NewStrategyResolver("adaptive", cfg)

	result := r.Resolve(SessionState{ContextUsagePct: 75, MessageCount: 20, IsLongRunning: true})
	assert.Equal(t, StrategyReset, result)
}

func TestStrategyResolver_Adaptive_CompactOnShortSession(t *testing.T) {
	cfg := AdaptiveStrategyConfig{ContextUsagePct: 70, MessageCount: 40, TaskBoundary: true}
	r := NewStrategyResolver("adaptive", cfg)

	result := r.Resolve(SessionState{ContextUsagePct: 75, MessageCount: 10, IsLongRunning: false})
	assert.Equal(t, StrategyCompaction, result)
}

func TestStrategyResolver_Adaptive_ResetOnTaskBoundary(t *testing.T) {
	cfg := AdaptiveStrategyConfig{ContextUsagePct: 70, MessageCount: 40, TaskBoundary: true}
	r := NewStrategyResolver("adaptive", cfg)

	result := r.Resolve(SessionState{ContextUsagePct: 30, MessageCount: 15, IsTaskBoundary: true})
	assert.Equal(t, StrategyReset, result)
}

func TestStrategyResolver_ExplicitStrategy(t *testing.T) {
	r := NewStrategyResolver("reset", AdaptiveStrategyConfig{})
	assert.Equal(t, StrategyReset, r.Resolve(SessionState{}))

	r2 := NewStrategyResolver("compaction", AdaptiveStrategyConfig{})
	assert.Equal(t, StrategyCompaction, r2.Resolve(SessionState{}))
}

func TestStrategyResolver_Adaptive_ExplicitReset(t *testing.T) {
	cfg := AdaptiveStrategyConfig{ContextUsagePct: 70, MessageCount: 40}
	r := NewStrategyResolver("adaptive", cfg)

	result := r.Resolve(SessionState{ContextUsagePct: 10, MessageCount: 5, ExplicitReset: true})
	assert.Equal(t, StrategyReset, result)
}
