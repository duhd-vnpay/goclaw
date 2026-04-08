package harness

import "github.com/nextlevelbuilder/goclaw/internal/harness/constraints"

type Config struct {
	Enabled          bool                          `json:"enabled"`
	DependencyLayers constraints.DependencyConfig  `json:"dependency_layers"`
	Continuity       ContinuityConfig              `json:"continuity"`
	Evaluation       EvaluationConfig              `json:"evaluation"`
	Orchestrator     OrchestratorConfig            `json:"orchestrator"`
}

type ContinuityConfig struct {
	Strategy         string            `json:"strategy"`
	Adaptive         AdaptiveConfig    `json:"adaptive"`
	PipelineOverride map[string]string `json:"pipeline_override"`
}

type AdaptiveConfig struct {
	ContextUsagePct int  `json:"context_usage_pct"`
	MessageCount    int  `json:"message_count"`
	TaskBoundary    bool `json:"task_boundary"`
}

type EvaluationConfig struct {
	EvalLoopUpgrade EvalLoopUpgradeConfig `json:"evaluate_loop_upgrade"`
}

type EvalLoopUpgradeConfig struct {
	Enabled             bool `json:"enabled"`
	BackwardCompatible  bool `json:"backward_compatible"`
	PreferHarnessPipeline bool `json:"prefer_harness_pipeline"`
}

type OrchestratorConfig struct {
	Enabled     bool   `json:"enabled"`
	WorkflowDir string `json:"workflow_dir"`
}

func DefaultConfig() Config {
	return Config{
		Enabled:          false,
		DependencyLayers: constraints.DependencyConfig{Enforcement: "off"},
		Continuity: ContinuityConfig{
			Strategy: "compaction",
			Adaptive: AdaptiveConfig{ContextUsagePct: 70, MessageCount: 40, TaskBoundary: true},
		},
		Evaluation: EvaluationConfig{
			EvalLoopUpgrade: EvalLoopUpgradeConfig{BackwardCompatible: true},
		},
		Orchestrator: OrchestratorConfig{WorkflowDir: "workflows/"},
	}
}
