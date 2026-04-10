package tools

import (
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/ardenn"
)

func TestArdennWorkflowTool_Name(t *testing.T) {
	tool := &ArdennWorkflowTool{}
	if tool.Name() != "ardenn_workflow" {
		t.Errorf("Name() = %q, want %q", tool.Name(), "ardenn_workflow")
	}
}

func TestArdennWorkflowTool_Parameters(t *testing.T) {
	tool := &ArdennWorkflowTool{}
	params := tool.Parameters()
	props, ok := params["properties"].(map[string]any)
	if !ok {
		t.Fatal("parameters missing properties")
	}
	for _, key := range []string{"action", "workflow_slug", "run_id", "step_id", "variables", "tier", "feedback"} {
		if _, ok := props[key]; !ok {
			t.Errorf("missing parameter %q", key)
		}
	}
}

func TestArdennWorkflowTool_Execute_UnknownAction(t *testing.T) {
	tool := &ArdennWorkflowTool{}
	result := tool.Execute(nil, map[string]any{"action": "invalid"})
	if !result.IsError {
		t.Error("expected error for unknown action")
	}
}

func TestParseRunID(t *testing.T) {
	_, err := parseRunID(map[string]any{"run_id": "550e8400-e29b-41d4-a716-446655440000"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, err = parseRunID(map[string]any{})
	if err == nil {
		t.Fatal("expected error for missing run_id")
	}
	_, err = parseRunID(map[string]any{"run_id": "not-a-uuid"})
	if err == nil {
		t.Fatal("expected error for invalid run_id")
	}
}

func TestFormatRunState(t *testing.T) {
	state := &ardenn.RunState{
		ID:     uuid.New(),
		Status: "running",
		Tier:   ardenn.TierStandard,
	}
	result := formatRunState(state)
	if result == "" {
		t.Error("expected non-empty output")
	}
	if !strings.Contains(result, "running") {
		t.Error("expected status in output")
	}
}
