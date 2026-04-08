package orchestrator

import "github.com/google/uuid"

// GateKeeper manages approval gates for workflow steps.
type GateKeeper struct {
	events *EventBus
}

// NewGateKeeper creates a gate keeper backed by the given event bus.
func NewGateKeeper(events *EventBus) *GateKeeper {
	return &GateKeeper{events: events}
}

// RequestApproval evaluates a gate and returns true if approved.
// In v1: auto gates pass immediately, conditional gates evaluate expression,
// human gates emit gate.pending event and auto-approve (real blocking in v2).
func (gk *GateKeeper) RequestApproval(run *WorkflowRun, step Step) bool {
	if step.Gate == nil {
		return true
	}

	switch step.Gate.Type {
	case "auto":
		return true

	case "conditional":
		if step.Gate.AutoPass != "" {
			pass, err := EvalExpr(step.Gate.AutoPass, run.Variables)
			if err == nil && pass {
				return true
			}
		}
		// conditional but expression failed — emit gate.pending
		gk.events.Emit("gate.pending", run.ID, step.ID, map[string]string{
			"approver":  step.Gate.Approver,
			"gate_type": step.Gate.Type,
		})
		return true // v1: auto-approve after emitting

	case "human":
		gk.events.Emit("gate.pending", run.ID, step.ID, map[string]string{
			"approver":  step.Gate.Approver,
			"gate_type": step.Gate.Type,
		})
		return true // v1: auto-approve after emitting

	default:
		return true
	}
}

// ApproveGate is called externally (WebUI, Telegram) to approve/reject a gate.
func (gk *GateKeeper) ApproveGate(runID uuid.UUID, stepID string, approved bool, feedback string) {
	eventType := "gate.approved"
	if !approved {
		eventType = "gate.rejected"
	}
	gk.events.Emit(eventType, runID, stepID, map[string]string{"feedback": feedback})
}
