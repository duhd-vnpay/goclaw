package mcp_shim

import (
	"encoding/json"
	"time"
)

// HandlerKind discriminates between the MCP-manager-backed dispatch path
// (Phase 2 SSE pool, now reused via mcp.NewBridgeServer) and the in-process
// builtin dispatch path (write_file, etc.). New kinds may be added later.
type HandlerKind string

const (
	HandlerKindMCPManager HandlerKind = "mcp_manager"
	HandlerKindBuiltin    HandlerKind = "builtin"
)

// ToolDescriptor is the resolved view of a tool that the shim will advertise
// over MCP for one ACP session. It is the resolver's output (Task 2) and
// feeds the per-session allowlist computed by the server (Task 3).
//
// Schema mirrors mcp-go's Tool.InputSchema shape — a JSON-Schema object
// describing the tool's argument shape. It is forwarded as-is to the
// underlying mark3labs StreamableHTTPServer via convertToMCPTool in
// bridge_server.go.
type ToolDescriptor struct {
	Name        string          // wire name, e.g. "mcp_ops__litellm_psql_query"
	Description string          // human-readable summary for tools/list
	Schema      json.RawMessage // JSON-Schema object — mirrors mcp-go Tool.InputSchema
	Kind        HandlerKind
	MCPServer   string // for HandlerKindMCPManager: server key (e.g. "ops")
	MCPName     string // for HandlerKindMCPManager: tool name without the "mcp_<server>__" prefix
	BuiltinName string // for HandlerKindBuiltin: registry name (e.g. "write_file")
	TimeoutMs   int    // per-tool timeout; 0 → default 60_000
}

// CronContext is the tuple captured at ACP session registration and
// injected by the dispatcher for builtin tool calls that need the outer
// cron-run identity (notably write_file deliver=true). PeerKind and
// SessionKey were added under Revision 1 to satisfy the routing context
// expected by tools.ExecuteWithContext and outbound bus metadata.
type CronContext struct {
	AgentID       string
	RunID         string
	ChannelID     string
	DeliverTarget string
	PeerKind      string // "private" | "group" — needed for outbound bus metadata
	SessionKey    string // session_key for cron sessions (used by tool routing context)
}

// SessionEntry is the per-ACP-session state held by the shim's sync.Map.
// All fields are immutable for the lifetime of the session.
//
// Revision 1: the underlying mark3labs MCP server is constructed once at
// startup with every candidate tool registered. Per-session filtering is
// done by name via Allowlist (a string slice) — consulted by the
// session-aware tool handler. ToolDescriptor stays as the resolver's
// output type, but SessionEntry stores only the names that survived
// resolution + blacklist filtering.
type SessionEntry struct {
	SID        string
	Cron       CronContext
	Allowlist  []string // per-session subset of registered tool names
	CreatedAt  time.Time
	rateBucket *rateBucket // unexported; see server.go (Task 8)
}

// AllowlistSet returns the per-session allowlist as a map for O(1) lookup
// in the session-aware tool handler. Helpers like this keep handler.go
// (Task 3) compact.
func (e *SessionEntry) AllowlistSet() map[string]bool {
	out := make(map[string]bool, len(e.Allowlist))
	for _, n := range e.Allowlist {
		out[n] = true
	}
	return out
}

// rateBucket is a forward-reference stub. Task 8 will replace this with
// the real rate limiter implementation in server.go.
type rateBucket struct{}
