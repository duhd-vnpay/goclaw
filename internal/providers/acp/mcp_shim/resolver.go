package mcp_shim

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"time"
)

// GrantedTool is the resolver's input shape — what the grants store returns
// per granted tool. The store layer (real or fake) is responsible for
// hydrating Schema from the source registry (builtin_tools.schema JSONB or
// mcp.Manager.GetToolSchema for MCP tools).
type GrantedTool struct {
	Name        string
	Description string
	Schema      json.RawMessage
	Kind        HandlerKind
	MCPServer   string
	MCPName     string
	BuiltinName string
	Timeout     time.Duration
}

// GrantsStore is the resolver's dependency. Production code wires a thin
// adapter over store/pg that joins (BridgeToolNames ∩ agents.acp_tools) ∪
// mcp_agent_grants.tool_allow with the source schemas.
//
// PG implementation deferred to Task 7+ wiring.
type GrantsStore interface {
	ListGrantedTools(ctx context.Context, agentID, tenantID string) ([]GrantedTool, error)
}

// hardBlacklist lists glob patterns of tool names that are never exposed
// over the shim, regardless of agent_grants. See design doc § Layer 2.
var hardBlacklist = []string{
	// Recursion vectors
	"delegate",
	"subagent_*",
	"cron_*",
	"agent_dispatch",
	"agent_create",
	"agent_update",
	"agent_delete",
	"team_*",
	"mcp_tool_search",

	// Admin escalation
	"provider_*",
	"mcp_server_*",
	"custom_tool_*",
	"skill_manage",
	"skill_publish_*",

	// Reserved
	"agent_hooks_*",
	"shell_exec_write",
}

// Resolver filters granted tools by the hard blacklist for ACP exposure.
type Resolver struct {
	store GrantsStore
}

// NewResolver constructs a resolver. store may be nil for tests that only
// exercise the predicate; ResolveToolSlice will then return an error.
func NewResolver(store GrantsStore) *Resolver {
	return &Resolver{store: store}
}

// ResolveToolSlice returns the tools the resolver agent is allowed to
// invoke from an ACP sub-session, with the hard blacklist applied.
func (r *Resolver) ResolveToolSlice(ctx context.Context, agentID, tenantID string) ([]ToolDescriptor, error) {
	grants, err := r.store.ListGrantedTools(ctx, agentID, tenantID)
	if err != nil {
		return nil, err
	}
	out := make([]ToolDescriptor, 0, len(grants))
	for _, g := range grants {
		if r.isBlacklisted(g.Name) {
			continue
		}
		out = append(out, ToolDescriptor{
			Name:        g.Name,
			Description: g.Description,
			Schema:      g.Schema,
			Kind:        g.Kind,
			MCPServer:   g.MCPServer,
			MCPName:     g.MCPName,
			BuiltinName: g.BuiltinName,
			Timeout:     g.Timeout,
		})
	}
	return out, nil
}

// init validates every hardBlacklist pattern at process start so that a
// malformed entry surfaces immediately instead of silently letting tools
// slip through the blacklist at runtime (this is a security defense gate,
// so we fail-fast at startup rather than fail-open at request time).
func init() {
	for _, pat := range hardBlacklist {
		if _, err := filepath.Match(pat, "_validate"); err != nil {
			panic("mcp_shim: malformed hardBlacklist pattern " + pat + ": " + err.Error())
		}
	}
}

// isBlacklisted runs filepath.Match for glob patterns. Exact matches are
// also globs (no wildcard) so the same function handles both. Patterns are
// validated at package init so filepath.Match errors are not expected here;
// if one occurs anyway we treat the tool as blacklisted (fail-closed for
// a security-relevant defense layer).
func (r *Resolver) isBlacklisted(name string) bool {
	lower := strings.ToLower(name)
	for _, pat := range hardBlacklist {
		ok, err := filepath.Match(pat, lower)
		if err != nil {
			return true
		}
		if ok {
			return true
		}
	}
	return false
}
