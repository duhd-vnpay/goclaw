package constraints

// Phase represents the lifecycle hook point at which a guard runs.
type Phase string

const (
	BeforeRun      Phase = "before_run"
	BeforeToolCall Phase = "before_tool"
	AfterToolCall  Phase = "after_tool"
	AfterRun       Phase = "after_run"
)

// Kind classifies a guard's evaluation strategy.
type Kind string

const (
	// Computational guards use deterministic logic (e.g., regex, budget checks).
	Computational Kind = "computational"
	// Inferential guards use LLM-based or heuristic evaluation.
	Inferential Kind = "inferential"
)

// GuardContext carries all relevant context for a guard evaluation.
type GuardContext struct {
	AgentID   string
	AgentKey  string
	UserID    string
	ToolName  string
	ToolInput string
	Output    string
	SessionID string
	TenantID  string
}

// GuardResult is the outcome of a single guard evaluation.
type GuardResult struct {
	Pass      bool   `json:"pass"`
	Action    string `json:"action"`     // "allow", "warn", "block", "rewrite"
	Feedback  string `json:"feedback"`
	GuardName string `json:"guard_name"`
}

// Guard is the interface every harness constraint must implement.
type Guard interface {
	// Name returns a stable, unique identifier for this guard.
	Name() string
	// Phase returns the lifecycle hook at which this guard should run.
	Phase() Phase
	// Kind returns whether this guard is computational or inferential.
	Kind() Kind
	// Check evaluates the guard against the provided context.
	Check(ctx GuardContext) GuardResult
}
