package hands

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	engine "github.com/nextlevelbuilder/goclaw/internal/ardenn"
)

const mcpDefaultTimeout = 60 * time.Second

// MCPToolResult mirrors the fields from tools.Result that MCPHand needs.
type MCPToolResult struct {
	ForLLM  string
	IsError bool
}

// MCPTool is the interface for MCP tool execution, mirroring tools.Tool
// to avoid importing the full tools package (which has heavy transitive deps).
type MCPTool interface {
	Name() string
	Execute(ctx context.Context, args map[string]any) *MCPToolResult
}

// MCPToolLookup resolves a tool name to an MCPTool instance.
type MCPToolLookup func(name string) (MCPTool, bool)

// MCPHand calls MCP server tools via the GoClaw MCP bridge.
type MCPHand struct {
	toolLookup MCPToolLookup
}

// NewMCPHand creates an MCPHand with the given tool lookup function.
func NewMCPHand(lookup MCPToolLookup) *MCPHand {
	return &MCPHand{toolLookup: lookup}
}

func (h *MCPHand) Type() engine.HandType { return engine.HandMCP }

func (h *MCPHand) Execute(ctx context.Context, req engine.HandRequest) engine.HandResult {
	start := time.Now()

	timeout := req.Timeout
	if timeout == 0 {
		timeout = mcpDefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Parse "server:tool" → "mcp_{server}__{tool}"
	registeredName, err := parseMCPToolName(req.Name)
	if err != nil {
		return engine.HandResult{Error: err, Duration: time.Since(start)}
	}

	tool, ok := h.toolLookup(registeredName)
	if !ok {
		return engine.HandResult{
			Error:    fmt.Errorf("MCP tool not found: %s (resolved: %s)", req.Name, registeredName),
			Duration: time.Since(start),
		}
	}

	// Deserialize input JSON
	var args map[string]any
	if req.Input != "" {
		if err := json.Unmarshal([]byte(req.Input), &args); err != nil {
			return engine.HandResult{
				Error:    fmt.Errorf("unmarshal MCP input: %w", err),
				Duration: time.Since(start),
			}
		}
	}
	if args == nil {
		args = map[string]any{}
	}

	slog.Info("ardenn.mcp_hand: calling tool",
		"step_run_id", req.StepRunID,
		"tool", registeredName,
		"run_id", req.RunID,
	)

	result := tool.Execute(ctx, args)

	if result.IsError {
		return engine.HandResult{
			Error:    fmt.Errorf("MCP tool error: %s", result.ForLLM),
			Duration: time.Since(start),
		}
	}

	return engine.HandResult{
		Output:   result.ForLLM,
		Duration: time.Since(start),
	}
}

func (h *MCPHand) Cancel(_ context.Context, runID uuid.UUID) error {
	slog.Info("ardenn.mcp_hand: cancel requested (no-op)", "run_id", runID)
	return nil
}

// parseMCPToolName converts "server:tool" to "mcp_{server}__{tool}".
func parseMCPToolName(name string) (string, error) {
	parts := strings.SplitN(name, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", fmt.Errorf("invalid MCP tool name %q: expected 'server:tool' format", name)
	}
	return "mcp_" + parts[0] + "__" + parts[1], nil
}
