package mcp_shim

import (
	"log/slog"

	"github.com/nextlevelbuilder/goclaw/internal/providers/acp"
)

// Handle adapts *Server to acp.ShimHandle (defined in internal/providers/acp).
// The acp package keeps ShimHandle as `RegisterSession(entry any)` to avoid an
// import cycle (mcp_shim depends on internal/tools + internal/mcp, both of
// which are reachable from acp). This adapter performs the type assertion
// at the call site so production wiring stays type-safe — a caller passing
// the wrong concrete type triggers a logged warning instead of a panic.
type Handle struct {
	srv *Server
}

// NewHandle wraps *Server so it satisfies acp.ShimHandle.
func NewHandle(s *Server) *Handle {
	return &Handle{srv: s}
}

// URL returns the base /mcp URL the underlying listener exposes.
func (h *Handle) URL() string {
	if h == nil || h.srv == nil {
		return ""
	}
	return h.srv.URL()
}

// SessionURL returns the per-session URL claude-agent-acp connects to.
func (h *Handle) SessionURL(sid string) string {
	if h == nil || h.srv == nil {
		return ""
	}
	return h.srv.SessionURL(sid)
}

// RegisterSession asserts entry is one of:
//   - mcp_shim.SessionEntry (value or pointer) — direct path used by internal
//     callers and tests.
//   - acp.ShimSessionEntry — the cycle-free shape that acp_provider.go
//     constructs without importing this package. We translate the fields
//     onto mcp_shim.SessionEntry before storing.
//
// A type-mismatch is treated as a configuration bug and logged at WARN —
// the session will then 404 on connect, which surfaces loudly enough to
// catch in smoke tests.
func (h *Handle) RegisterSession(entry any) {
	if h == nil || h.srv == nil {
		return
	}
	switch e := entry.(type) {
	case SessionEntry:
		h.srv.RegisterSession(e)
	case *SessionEntry:
		if e != nil {
			h.srv.RegisterSession(*e)
		}
	case acp.ShimSessionEntry:
		h.srv.RegisterSession(fromACP(e))
	case *acp.ShimSessionEntry:
		if e != nil {
			h.srv.RegisterSession(fromACP(*e))
		}
	default:
		slog.Warn("acp.shim.register_type_mismatch",
			"want", "mcp_shim.SessionEntry or acp.ShimSessionEntry")
	}
}

// fromACP adapts the cycle-free acp.ShimSessionEntry into the concrete
// mcp_shim.SessionEntry the server stores. Keeps the two structs decoupled
// so future fields can be added to either side without touching the other.
func fromACP(e acp.ShimSessionEntry) SessionEntry {
	return SessionEntry{
		SID:       e.SID,
		Allowlist: e.Allowlist,
		Cron: CronContext{
			AgentID:       e.AgentID,
			AgentUUID:     e.AgentUUID,
			TenantID:      e.TenantID,
			ChannelID:     e.ChannelID,
			DeliverTarget: e.DeliverTarget,
			PeerKind:      e.PeerKind,
			SessionKey:    e.SessionKey,
			TeamID:        e.TeamID,
		},
	}
}

// UnregisterSession removes the session from the shim's registry.
func (h *Handle) UnregisterSession(sid string) {
	if h == nil || h.srv == nil {
		return
	}
	h.srv.UnregisterSession(sid)
}

// SetMCPSessionBuilder accepts the cycle-free any-typed callback and forwards
// it to the concrete *Server. Production wiring passes the MCPSessionBuilder
// type; anything else is logged + ignored so a wiring bug surfaces loudly in
// the startup log instead of silently degrading sessions to global catalog.
func (h *Handle) SetMCPSessionBuilder(fn any) {
	if h == nil || h.srv == nil {
		return
	}
	if fn == nil {
		h.srv.SetMCPSessionBuilder(nil)
		return
	}
	builder, ok := fn.(MCPSessionBuilder)
	if !ok {
		slog.Warn("acp.shim.set_builder_type_mismatch",
			"want", "mcp_shim.MCPSessionBuilder")
		return
	}
	h.srv.SetMCPSessionBuilder(builder)
}

// Underlying exposes the wrapped *Server so startup wiring can defer Close
// without re-importing the listener. Returns nil if Handle was constructed
// against a nil server.
func (h *Handle) Underlying() *Server {
	if h == nil {
		return nil
	}
	return h.srv
}
