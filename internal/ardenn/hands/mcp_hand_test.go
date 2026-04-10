package hands

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	engine "github.com/nextlevelbuilder/goclaw/internal/ardenn"
)

// mockMCPTool implements MCPTool for testing.
type mockMCPTool struct {
	name   string
	result *MCPToolResult
}

func (m *mockMCPTool) Name() string { return m.name }
func (m *mockMCPTool) Execute(_ context.Context, _ map[string]any) *MCPToolResult {
	return m.result
}

func TestMCPHand_Execute_Success(t *testing.T) {
	mock := &mockMCPTool{
		name:   "mcp_gitlab__list_projects",
		result: &MCPToolResult{ForLLM: `[{"id":1,"name":"test"}]`},
	}

	lookup := func(name string) (MCPTool, bool) {
		if name == mock.name {
			return mock, true
		}
		return nil, false
	}

	hand := NewMCPHand(lookup)

	req := engine.HandRequest{
		RunID:     uuid.New(),
		StepRunID: uuid.New(),
		Name:      "gitlab:list_projects",
		Input:     `{}`,
		Metadata:  map[string]any{},
	}

	result := hand.Execute(context.Background(), req)

	if result.Error != nil {
		t.Fatalf("expected no error, got %v", result.Error)
	}
	if result.Output != `[{"id":1,"name":"test"}]` {
		t.Errorf("unexpected output: %q", result.Output)
	}
}

func TestMCPHand_Execute_ToolNotFound(t *testing.T) {
	lookup := func(name string) (MCPTool, bool) {
		return nil, false
	}

	hand := NewMCPHand(lookup)

	req := engine.HandRequest{
		RunID:     uuid.New(),
		StepRunID: uuid.New(),
		Name:      "unknown:tool",
		Input:     `{}`,
		Metadata:  map[string]any{},
	}

	result := hand.Execute(context.Background(), req)

	if result.Error == nil {
		t.Fatal("expected error for unknown tool, got nil")
	}
	errMsg := result.Error.Error()
	if !contains(errMsg, "tool not found") {
		t.Errorf("expected error containing 'tool not found', got %q", errMsg)
	}
}

func TestMCPHand_Execute_InvalidInput(t *testing.T) {
	mock := &mockMCPTool{
		name:   "mcp_gitlab__list_projects",
		result: &MCPToolResult{ForLLM: "ok"},
	}

	lookup := func(name string) (MCPTool, bool) {
		if name == mock.name {
			return mock, true
		}
		return nil, false
	}

	hand := NewMCPHand(lookup)

	req := engine.HandRequest{
		RunID:     uuid.New(),
		StepRunID: uuid.New(),
		Name:      "gitlab:list_projects",
		Input:     `{not valid json`,
		Metadata:  map[string]any{},
	}

	result := hand.Execute(context.Background(), req)

	if result.Error == nil {
		t.Fatal("expected unmarshal error, got nil")
	}
	errMsg := result.Error.Error()
	if !contains(errMsg, "unmarshal") {
		t.Errorf("expected error containing 'unmarshal', got %q", errMsg)
	}
}

func TestMCPHand_Execute_ToolError(t *testing.T) {
	mock := &mockMCPTool{
		name:   "mcp_serena__get_file",
		result: &MCPToolResult{ForLLM: "file not found", IsError: true},
	}

	lookup := func(name string) (MCPTool, bool) {
		if name == mock.name {
			return mock, true
		}
		return nil, false
	}

	hand := NewMCPHand(lookup)

	req := engine.HandRequest{
		RunID:     uuid.New(),
		StepRunID: uuid.New(),
		Name:      "serena:get_file",
		Input:     `{"path":"/missing"}`,
		Metadata:  map[string]any{},
		Timeout:   5 * time.Second,
	}

	result := hand.Execute(context.Background(), req)

	if result.Error == nil {
		t.Fatal("expected tool error, got nil")
	}
	errMsg := result.Error.Error()
	if !contains(errMsg, "MCP tool error") {
		t.Errorf("expected error containing 'MCP tool error', got %q", errMsg)
	}
}

func TestMCPHand_ParseToolName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
		wantErr  bool
	}{
		{"gitlab:list_projects", "mcp_gitlab__list_projects", false},
		{"serena:get_file", "mcp_serena__get_file", false},
		{"bad_format", "", true},
		{":tool", "", true},
		{"server:", "", true},
		{"", "", true},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got, err := parseMCPToolName(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Errorf("expected error for input %q, got nil", tc.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for input %q: %v", tc.input, err)
			}
			if got != tc.expected {
				t.Errorf("parseMCPToolName(%q) = %q, want %q", tc.input, got, tc.expected)
			}
		})
	}
}
