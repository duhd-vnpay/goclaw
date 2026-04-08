package evaluation

import (
	"time"

	"github.com/google/uuid"
)

// SensorKind distinguishes computational (fast, deterministic) from inferential (LLM-based) sensors.
type SensorKind string

const (
	Computational SensorKind = "computational"
	Inferential   SensorKind = "inferential"
)

// Sensor evaluates agent output and returns a result.
type Sensor interface {
	Name() string
	Kind() SensorKind
	Applies(ctx EvalContext) bool
	Evaluate(ctx EvalContext) SensorResult
}

// EvalContext provides context for sensor evaluation.
type EvalContext struct {
	AgentID  uuid.UUID
	AgentKey string
	ToolName string
	Output   string
	Task     string
}

// SensorResult is the outcome of a single sensor evaluation.
type SensorResult struct {
	Pass     bool          `json:"pass"`
	Score    *float64      `json:"score,omitempty"`
	Feedback string        `json:"feedback"`
	Issues   []Issue       `json:"issues,omitempty"`
	Duration time.Duration `json:"duration"`
	Kind     SensorKind    `json:"kind"`
	Name     string        `json:"name"`
}

// Issue describes a specific problem found by a sensor.
type Issue struct {
	Severity string `json:"severity"` // "critical","high","medium","low"
	Location string `json:"location"` // file:line or section name
	Problem  string `json:"problem"`
	Fix      string `json:"fix"`
}

// EvalResult is the outcome of a full evaluation pipeline run.
type EvalResult struct {
	Pass     bool           `json:"pass"`
	Round    int            `json:"round"`
	Track    string         `json:"track"` // "computational","inferential","all"
	Feedback string         `json:"feedback"`
	Escalate bool           `json:"escalate"`
	Results  []SensorResult `json:"results"`
}
