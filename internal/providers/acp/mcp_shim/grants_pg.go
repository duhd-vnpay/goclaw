package mcp_shim

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/mcp"
)

// ACPToolsLookup is the narrow surface the GrantsStore needs to read the
// per-agent builtin tool allowlist (agents.acp_tools JSONB, migration 092).
// PGAgentStore satisfies it via GetAgentACPTools.
//
// Kept as a separate interface so the mcp_shim package does not pull the
// full AgentStore CRUD surface — and so SQLite / desktop builds (which do
// not run the ACP shim) are not forced to implement it.
type ACPToolsLookup interface {
	GetAgentACPTools(ctx context.Context, agentID uuid.UUID) ([]string, error)
}

// MCPAccessInfoView is the minimal projection grants_pg needs from
// store.MCPAccessInfo. Defined locally to avoid importing internal/store
// here (which would create a cycle: store → providers → acp/mcp_shim →
// store). The wiring layer adapts store.MCPAccessInfo to this view.
type MCPAccessInfoView struct {
	ServerName string
	ToolAllow  []string
	ToolDeny   []string
}

// MCPAccessLookup returns the per-agent accessible MCP servers + grant
// allow/deny filters. The wiring layer adapts store.MCPServerStore.ListAccessible
// to this surface to keep the cycle off the import graph.
type MCPAccessLookup interface {
	ListAccessibleForAgent(ctx context.Context, agentID uuid.UUID) ([]MCPAccessInfoView, error)
}

// PGGrantsStore implements GrantsStore against the PG store layer.
//
// Output composition:
//   - Builtin tools: agents.acp_tools ∩ mcp.BridgeToolNames
//     (the BridgeToolNames intersect is the security gate noted in
//     resolver.go — without it, registry tools outside BridgeToolNames
//     could surface if a session's Allowlist includes them.)
//   - MCP tools: for every grant in mcp_agent_grants for the agent, emit
//     one GrantedTool per name in tool_allow (or "*" → unsupported here,
//     since the shim needs explicit names for the per-session allowlist).
//
// The resolver layer (resolver.go) applies the hard blacklist on top, so
// admin/recursion vectors are blocked regardless of grant config.
type PGGrantsStore struct {
	agents ACPToolsLookup
	mcp    MCPAccessLookup
}

// NewPGGrantsStore wires the PG-backed grants source.
func NewPGGrantsStore(agents ACPToolsLookup, mcp MCPAccessLookup) *PGGrantsStore {
	return &PGGrantsStore{agents: agents, mcp: mcp}
}

// ListGrantedTools returns the union of (acp_tools ∩ BridgeToolNames) builtin
// entries and explicit mcp_<server>__<tool> entries from agent grants.
//
// tenantID is informational here: the underlying queries either rely on
// scopeClause-from-ctx (mcp grants) or are unscoped (agents.acp_tools is a
// raw per-agent lookup). Callers wanting tenant scoping should attach it via
// store.WithTenantID(ctx, ...) before calling.
func (s *PGGrantsStore) ListGrantedTools(ctx context.Context, agentID, tenantID string) ([]GrantedTool, error) {
	aid, err := uuid.Parse(agentID)
	if err != nil {
		return nil, fmt.Errorf("mcp_shim grants: invalid agent_id: %w", err)
	}

	var out []GrantedTool

	// 1. Builtin tools — agents.acp_tools ∩ mcp.BridgeToolNames.
	if s.agents != nil {
		acpTools, err := s.agents.GetAgentACPTools(ctx, aid)
		if err != nil {
			return nil, fmt.Errorf("mcp_shim grants: read acp_tools: %w", err)
		}
		for _, name := range acpTools {
			if !mcp.BridgeToolNames[name] {
				// SECURITY: silently drop entries outside BridgeToolNames.
				// The DB seed should not contain them, but treating them as
				// no-ops here keeps a misconfigured row from leaking the
				// agent_files / spawn / recursion tools.
				continue
			}
			out = append(out, GrantedTool{
				Name:        name,
				Description: "",
				Schema:      nil, // populated downstream from registry via ConvertToMCPTool
				Kind:        HandlerKindBuiltin,
				BuiltinName: name,
			})
		}
	}

	// 2. MCP-server tools — per-server tool_allow from mcp_agent_grants.
	//
	// The wiring layer adapts store.MCPServerStore.ListAccessible (joins
	// mcp_servers ⋈ mcp_agent_grants, enabled=true both sides, tenant scope
	// from ctx) to the MCPAccessLookup interface. user_id="" upstream → skip
	// per-user grant join (system identity at registration).
	if s.mcp != nil {
		accessible, err := s.mcp.ListAccessibleForAgent(ctx, aid)
		if err != nil {
			return nil, fmt.Errorf("mcp_shim grants: list mcp_agent_grants: %w", err)
		}
		for _, info := range accessible {
			prefix := "mcp_" + info.ServerName + "__"
			// tool_allow nil = "all tools from server"; the shim cannot
			// enumerate the upstream catalog without an MCP call, so we
			// require explicit names. Silently skip wildcard grants here —
			// operators must list tool names per grant for ACP exposure.
			if len(info.ToolAllow) == 0 {
				continue
			}
			denied := make(map[string]bool, len(info.ToolDeny))
			for _, d := range info.ToolDeny {
				denied[d] = true
			}
			for _, mcpName := range info.ToolAllow {
				if denied[mcpName] {
					continue
				}
				out = append(out, GrantedTool{
					Name:        prefix + mcpName,
					Description: "",
					Schema:      nil, // populated downstream via mcp.Manager.GetToolSchema (deferred)
					Kind:        HandlerKindMCPManager,
					MCPServer:   info.ServerName,
					MCPName:     mcpName,
				})
			}
		}
	}

	return out, nil
}

// AssertGrantsStore is a compile-time guard that *PGGrantsStore satisfies the
// GrantsStore interface so the resolver constructor accepts it.
var _ GrantsStore = (*PGGrantsStore)(nil)

// MarshalDebugGrants is a small helper used in startup logs to surface the
// resolved per-agent tool counts without dumping schemas. Keeps the JSON
// import paired with the file even when the rest of the package shrinks.
func MarshalDebugGrants(tools []GrantedTool) ([]byte, error) {
	type entry struct {
		Name string      `json:"name"`
		Kind HandlerKind `json:"kind"`
	}
	out := make([]entry, 0, len(tools))
	for _, t := range tools {
		out = append(out, entry{Name: t.Name, Kind: t.Kind})
	}
	return json.Marshal(out)
}
