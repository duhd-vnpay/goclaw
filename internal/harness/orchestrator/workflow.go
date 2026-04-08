package orchestrator

import "time"

// Workflow is a declarative definition of a multi-step agent pipeline.
type Workflow struct {
	ID        string         `json:"id" yaml:"id"`
	Name      string         `json:"name" yaml:"name"`
	Version   string         `json:"version" yaml:"version"`
	Trigger   TriggerConfig  `json:"trigger" yaml:"trigger"`
	Variables map[string]any `json:"variables,omitempty" yaml:"variables"`
	Steps     []Step         `json:"steps" yaml:"steps"`
	OnFailure FailurePolicy  `json:"on_failure" yaml:"on_failure"`
}

// Step is a single unit of work in a workflow.
type Step struct {
	ID        string      `json:"id" yaml:"id"`
	Name      string      `json:"name" yaml:"name"`
	Agent     string      `json:"agent" yaml:"agent"`
	Task      string      `json:"task" yaml:"task"`
	DependsOn []string    `json:"depends_on,omitempty" yaml:"depends_on"`
	Harness   StepHarness `json:"harness,omitempty" yaml:"harness"`
	Gate      *GateConfig `json:"gate,omitempty" yaml:"gate"`
	When      string      `json:"when,omitempty" yaml:"when"`
	Timeout   YAMLDuration `json:"timeout,omitempty" yaml:"timeout"`
}

// StepHarness configures per-step harness controls (L1-L3).
type StepHarness struct {
	DependencyLayer string      `json:"dependency_layer,omitempty" yaml:"dependency_layer"`
	ExtraGuards     []string    `json:"extra_guards,omitempty" yaml:"extra_guards"`
	ContextStrategy string      `json:"context_strategy,omitempty" yaml:"context_strategy"`
	Evaluation      *EvalConfig `json:"evaluation,omitempty" yaml:"evaluation"`
}

// EvalConfig configures evaluation for a step.
type EvalConfig struct {
	Computational []string   `json:"computational,omitempty" yaml:"computational"`
	Inferential   *InfConfig `json:"inferential,omitempty" yaml:"inferential"`
	MaxRounds     int        `json:"max_rounds,omitempty" yaml:"max_rounds"`
}

// InfConfig configures the inferential evaluator for a step.
type InfConfig struct {
	Evaluator string `json:"evaluator,omitempty" yaml:"evaluator"`
	RubricRef string `json:"rubric_ref,omitempty" yaml:"rubric_ref"`
}

// GateConfig configures an approval gate.
type GateConfig struct {
	Type     string       `json:"type" yaml:"type"`         // "human","auto","conditional"
	Approver string       `json:"approver,omitempty" yaml:"approver"`
	AutoPass string       `json:"auto_pass,omitempty" yaml:"auto_pass"` // expression
	Timeout  YAMLDuration `json:"timeout,omitempty" yaml:"timeout"`
}

// TriggerConfig configures how a workflow is started.
type TriggerConfig struct {
	Type  string `json:"type" yaml:"type"` // "manual","cron","event","webhook"
	Event string `json:"event,omitempty" yaml:"event"`
	Cron  string `json:"cron,omitempty" yaml:"cron"`
}

// FailurePolicy configures what happens when steps fail.
type FailurePolicy struct {
	Default string            `json:"default" yaml:"default"` // "stop","skip","retry","human"
	PerStep map[string]string `json:"per_step,omitempty" yaml:"per_step"`
}

// YAMLDuration wraps time.Duration for YAML/JSON string parsing.
type YAMLDuration struct {
	time.Duration
}

// UnmarshalText parses duration strings like "24h", "30m", "5s".
func (d *YAMLDuration) UnmarshalText(text []byte) error {
	dur, err := time.ParseDuration(string(text))
	if err != nil {
		return err
	}
	d.Duration = dur
	return nil
}

// UnmarshalYAML handles YAML duration values.
func (d *YAMLDuration) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var s string
	if err := unmarshal(&s); err != nil {
		return err
	}
	if s == "" {
		return nil
	}
	dur, err := time.ParseDuration(s)
	if err != nil {
		return err
	}
	d.Duration = dur
	return nil
}
