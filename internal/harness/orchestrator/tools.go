package orchestrator

import (
	"encoding/json"
	"fmt"
)

// ToolInput is the JSON input for harness_workflow tool.
type ToolInput struct {
	Action     string         `json:"action"` // "start","resume","status","cancel","approve","reject"
	WorkflowID string         `json:"workflow_id"`
	RunID      string         `json:"run_id"`
	Variables  map[string]any `json:"variables"`
	StepID     string         `json:"step_id"`
	Feedback   string         `json:"feedback"`
}

// WorkflowToolHandler handles the harness_workflow builtin tool.
type WorkflowToolHandler struct {
	loader *WorkflowLoader
	state  *StateManager
	events *EventBus
	gate   *GateKeeper
}

// NewWorkflowToolHandler creates a handler with the given dependencies.
func NewWorkflowToolHandler(loader *WorkflowLoader, state *StateManager, events *EventBus, gate *GateKeeper) *WorkflowToolHandler {
	return &WorkflowToolHandler{loader: loader, state: state, events: events, gate: gate}
}

// Handle dispatches to the appropriate action handler.
func (h *WorkflowToolHandler) Handle(input json.RawMessage) (string, error) {
	var in ToolInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("parse input: %w", err)
	}

	switch in.Action {
	case "start":
		return h.handleStart(in)
	case "status":
		return h.handleStatus(in)
	default:
		return "", fmt.Errorf("unsupported action: %s (supported: start, status)", in.Action)
	}
}

func (h *WorkflowToolHandler) handleStart(in ToolInput) (string, error) {
	workflows, err := h.loader.LoadAll()
	if err != nil {
		return "", fmt.Errorf("load workflows: %w", err)
	}

	wf, ok := workflows[in.WorkflowID]
	if !ok {
		available := make([]string, 0, len(workflows))
		for id := range workflows {
			available = append(available, id)
		}
		return "", fmt.Errorf("workflow not found: %s (available: %v)", in.WorkflowID, available)
	}

	run := h.state.CreateRun(wf, in.Variables)
	h.events.Emit("workflow.started", run.ID, "", in.Variables)

	ready := h.state.GetReadySteps(run)
	readyNames := make([]string, len(ready))
	for i, s := range ready {
		readyNames[i] = s.Name
	}

	return fmt.Sprintf("Workflow '%s' v%s started (run: %s). %d steps ready: %v",
		wf.Name, wf.Version, run.ID.String()[:8], len(ready), readyNames), nil
}

func (h *WorkflowToolHandler) handleStatus(in ToolInput) (string, error) {
	return fmt.Sprintf("Status query for run %s — v1 uses in-memory state only.", in.RunID), nil
}
