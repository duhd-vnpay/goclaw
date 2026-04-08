package harness

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultConfig_HarnessDisabled(t *testing.T) {
	cfg := DefaultConfig()
	assert.False(t, cfg.Enabled)
	assert.Equal(t, "off", cfg.DependencyLayers.Enforcement)
	assert.Equal(t, "compaction", cfg.Continuity.Strategy)
	assert.Equal(t, 70, cfg.Continuity.Adaptive.ContextUsagePct)
	assert.Equal(t, 40, cfg.Continuity.Adaptive.MessageCount)
	assert.True(t, cfg.Continuity.Adaptive.TaskBoundary)
	assert.True(t, cfg.Evaluation.EvalLoopUpgrade.BackwardCompatible)
	assert.Equal(t, "workflows/", cfg.Orchestrator.WorkflowDir)
}
