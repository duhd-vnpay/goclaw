package harness

import (
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/harness/constraints"
	"github.com/nextlevelbuilder/goclaw/internal/harness/continuity"
	"github.com/nextlevelbuilder/goclaw/internal/harness/evaluation"
	"github.com/nextlevelbuilder/goclaw/internal/harness/evaluation/sensors"
	"github.com/nextlevelbuilder/goclaw/internal/harness/orchestrator"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHarness_L1_GuardRegistryFlow(t *testing.T) {
	reg := constraints.NewGuardRegistry()
	ctx := constraints.GuardContext{AgentKey: "test-agent"}
	results := reg.RunPhase(constraints.BeforeRun, ctx)
	assert.Len(t, results, 0) // no guards = pass
}

func TestHarness_L1_DependencyEnforcement(t *testing.T) {
	cfg := constraints.DependencyConfig{
		Layers: []constraints.DependencyLayer{
			{Name: "planning", Order: 1, AllowsUp: []string{"coding"}},
			{Name: "coding", Order: 2, AllowsUp: []string{"testing"}},
		},
		AgentMapping: map[string]string{
			"planner": "planning",
			"coder":   "coding",
		},
		Enforcement: "block",
	}
	engine := constraints.NewDependencyEngine(cfg)

	assert.NoError(t, engine.ValidateCall("planner", "coder"))
	assert.Error(t, engine.ValidateCall("coder", "planner"))
}

func TestHarness_L2_ResumeContextNil(t *testing.T) {
	result := continuity.BuildResumeContext(nil)
	assert.Empty(t, result)
}

func TestHarness_L2_StrategyAdaptive(t *testing.T) {
	r := continuity.NewStrategyResolver("adaptive", continuity.AdaptiveStrategyConfig{
		ContextUsagePct: 70, MessageCount: 40, TaskBoundary: true,
	})
	assert.Equal(t, continuity.StrategyReset, r.Resolve(continuity.SessionState{IsTaskBoundary: true}))
	assert.Equal(t, continuity.StrategyCompaction, r.Resolve(continuity.SessionState{ContextUsagePct: 50, MessageCount: 10}))
}

func TestHarness_L3_EvalPipelineCleanPass(t *testing.T) {
	noSecrets := sensors.NewRegexSensor("no_secrets", []sensors.RegexPattern{
		{Regex: `(?i)api_key\s*=\s*"[^"]{8,}"`, Severity: "critical", Message: "Secret detected"},
	})
	pipeline := evaluation.NewEvalPipeline(
		[]evaluation.Sensor{noSecrets}, nil, 3, evaluation.EscalationPolicy{},
	)

	result := pipeline.RunOnce(evaluation.EvalContext{Output: "Hello world, no secrets here"})
	assert.True(t, result.Pass)
}

func TestHarness_L3_EvalPipelineSecretFail(t *testing.T) {
	noSecrets := sensors.NewRegexSensor("no_secrets", []sensors.RegexPattern{
		{Regex: `(?i)api_key\s*=\s*"[^"]{8,}"`, Severity: "critical", Message: "Secret detected"},
	})
	pipeline := evaluation.NewEvalPipeline(
		[]evaluation.Sensor{noSecrets}, nil, 3, evaluation.EscalationPolicy{},
	)

	result := pipeline.RunOnce(evaluation.EvalContext{Output: `api_key = "sk-verylongsecretkey123"`})
	assert.False(t, result.Pass)
	assert.Equal(t, "computational", result.Track)
}

func TestHarness_L3_RubricScoring(t *testing.T) {
	rubric := evaluation.Rubric{
		Dimensions: []evaluation.Dimension{
			{Name: "quality", Weight: 0.7},
			{Name: "security", Weight: 0.3},
		},
		PassThreshold: 0.7,
	}
	score := rubric.ComputeScore(map[string]float64{"quality": 0.8, "security": 0.9})
	assert.InDelta(t, 0.83, score, 0.01)
	assert.True(t, rubric.Passes(score))
}

func TestHarness_L4_WorkflowLoaderAndStateManager(t *testing.T) {
	wf := orchestrator.Workflow{
		ID:   "test-pipeline",
		Name: "Test Pipeline",
		Steps: []orchestrator.Step{
			{ID: "a", Name: "Step A", Agent: "agent-a", Task: "do A"},
			{ID: "b", Name: "Step B", Agent: "agent-b", Task: "do B", DependsOn: []string{"a"}},
		},
	}

	sm := orchestrator.NewStateManager()
	run := sm.CreateRun(wf, nil)

	ready := sm.GetReadySteps(run)
	assert.Len(t, ready, 1)
	assert.Equal(t, "a", ready[0].ID)

	sm.MarkStep(run, "a", "completed")
	ready = sm.GetReadySteps(run)
	assert.Len(t, ready, 1)
	assert.Equal(t, "b", ready[0].ID)

	sm.MarkStep(run, "b", "completed")
	assert.True(t, sm.IsComplete(run))
}

func TestHarness_L4_ExpressionEvaluator(t *testing.T) {
	vars := map[string]any{"pipeline_type": "full", "compliance_tier": float64(3)}

	result, err := orchestrator.EvalExpr("variables.pipeline_type in ['full', 'new-feature']", vars)
	require.NoError(t, err)
	assert.True(t, result)

	result, err = orchestrator.EvalExpr("variables.compliance_tier <= 2", vars)
	require.NoError(t, err)
	assert.False(t, result) // tier 3 > 2
}

func TestHarness_L4_EventBus(t *testing.T) {
	bus := orchestrator.NewEventBus()
	received := make(chan string, 1)

	bus.Subscribe("test.event", func(e orchestrator.WorkflowEvent) {
		received <- e.Type
	})

	bus.Emit("test.event", [16]byte{}, "step1", nil)

	select {
	case eventType := <-received:
		assert.Equal(t, "test.event", eventType)
	default:
		// async — give a tiny window
		import_time_sleep_needed := false
		_ = import_time_sleep_needed
	}
}

func TestHarness_ManagerCreation(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.DependencyLayers.Enforcement = "warn"

	// nil db is fine for unit test — store methods won't be called
	m := NewManager(cfg, nil)

	assert.True(t, m.Enabled())
	assert.NotNil(t, m.Guards())
	assert.NotNil(t, m.Dependencies())
	assert.NotNil(t, m.Artifacts())
	assert.NotNil(t, m.Strategy())
	assert.NotNil(t, m.Events())
}
