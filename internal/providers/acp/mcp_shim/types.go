package mcp_shim

import (
	"encoding/json"
	"sync"
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

// DefaultToolTimeout is the per-tool dispatch timeout applied when a
// ToolDescriptor leaves TimeoutMs at zero. 60s mirrors the mcp.Manager
// timeout floor used for ops-mcp / cve-intel SSE clients.
const DefaultToolTimeout = 60 * time.Second

// ToolDescriptor is the resolved view of a tool that the shim will advertise
// over MCP for one ACP session. It is the resolver's output (Task 2) and
// feeds the per-session allowlist computed by the server (Task 3).
//
// Schema mirrors mcp-go's Tool.InputSchema shape — a JSON-Schema object
// describing the tool's argument shape. It is forwarded as-is to the
// underlying mark3labs StreamableHTTPServer via mcp.ConvertToMCPTool in
// bridge_server.go.
type ToolDescriptor struct {
	Name        string          `json:"name"`        // wire name, e.g. "mcp_ops__litellm_psql_query"
	Description string          `json:"description"` // human-readable summary for tools/list
	Schema      json.RawMessage `json:"schema"`      // JSON-Schema object — mirrors mcp-go Tool.InputSchema
	Kind        HandlerKind     `json:"kind"`
	MCPServer   string          `json:"mcpServer,omitempty"`   // for HandlerKindMCPManager: server key (e.g. "ops")
	MCPName     string          `json:"mcpName,omitempty"`     // for HandlerKindMCPManager: tool name without the "mcp_<server>__" prefix
	BuiltinName string          `json:"builtinName,omitempty"` // for HandlerKindBuiltin: registry name (e.g. "write_file")
	Timeout     time.Duration   `json:"timeout,omitempty"`     // per-tool timeout; zero → DefaultToolTimeout
}

// CronContext is the tuple captured at ACP session registration and
// injected by the dispatcher for builtin tool calls that need the outer
// cron-run identity (notably write_file deliver=true). PeerKind and
// SessionKey were added under Revision 1 to satisfy the routing context
// expected by tools.ExecuteWithContext and outbound bus metadata.
type CronContext struct {
	AgentID       string `json:"agentId"`
	RunID         string `json:"runId"`
	ChannelID     string `json:"channelId"`
	DeliverTarget string `json:"deliverTarget"`
	PeerKind      string `json:"peerKind"`   // "private" | "group" — needed for outbound bus metadata
	SessionKey    string `json:"sessionKey"` // session_key for cron sessions (used by tool routing context)
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

// rateBucket is a simple token-bucket limiter used to cap the volume of
// `tools/call` invocations the shim accepts from a single ACP session.
// Tokens are refilled in one shot whenever refillEvery has elapsed since
// the last refill — that is cheaper than continuous refill and is good
// enough for the burst-control profile we want (100 calls per 5 minutes
// per session). Concurrent take() calls are serialized by mu.
type rateBucket struct {
	mu          sync.Mutex
	tokens      int
	capacity    int
	refillEvery time.Duration
	lastRefill  time.Time
}

// newRateBucket constructs a full bucket with the given capacity and
// refill interval. The bucket starts at capacity tokens with lastRefill
// set to time.Now() so the first window starts fresh.
func newRateBucket(capacity int, refillEvery time.Duration) *rateBucket {
	return &rateBucket{
		tokens:      capacity,
		capacity:    capacity,
		refillEvery: refillEvery,
		lastRefill:  time.Now(),
	}
}

// take attempts to consume one token. If at least refillEvery has elapsed
// since lastRefill, the bucket is refilled to capacity before the take is
// evaluated. Returns true on success, false when the bucket is empty.
func (b *rateBucket) take(now time.Time) bool {
	if b == nil {
		return true
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.refillEvery > 0 && now.Sub(b.lastRefill) >= b.refillEvery {
		b.tokens = b.capacity
		b.lastRefill = now
	}
	if b.tokens <= 0 {
		return false
	}
	b.tokens--
	return true
}
