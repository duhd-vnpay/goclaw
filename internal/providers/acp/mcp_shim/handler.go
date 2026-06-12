package mcp_shim

import (
	"context"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/nextlevelbuilder/goclaw/internal/bus"
	"github.com/nextlevelbuilder/goclaw/internal/mcp"
	"github.com/nextlevelbuilder/goclaw/internal/tools"
)

// workspaceBase is the host-side root that team workspaces live under. It
// defaults to /app/workspace (matching GOCLAW_WORKSPACE on the production
// container) and is set once at package init from the GOCLAW_WORKSPACE
// environment variable. Phase 5.1 shim relocate joins it with
// "teams/<teamID>/<relPath>" when a tool in needsTeamRelocate's whitelist is
// called inside a team session (sess.Cron.TeamID != "").
//
// NB: we use GOCLAW_WORKSPACE (not GOCLAW_DATA_DIR) so the relocated path
// stays inside the write_file workspace sandbox — write_file's path-escape
// check rejects any path outside its workspace root. The canonical team
// workspace storage layout on disk is {GOCLAW_WORKSPACE}/teams/{teamID}/.
var workspaceBase = func() string {
	if v := os.Getenv("GOCLAW_WORKSPACE"); v != "" {
		return v
	}
	return "/app/workspace"
}()

// needsTeamRelocate reports whether a tool creates a file under the workspace
// and therefore needs its 'path' argument rewritten under the team workspace
// root for team-dispatched ACP sessions. The whitelist is intentionally narrow
// — add new file-creating tools explicitly. Read-side tools (read_file,
// list_files) are NOT included; they already receive the team-scoped workspace
// via tools.ToolWorkspaceFromCtx.
func needsTeamRelocate(toolName string) bool {
	switch toolName {
	case "write_file", "create_image", "tts":
		return true
	}
	return false
}

// applyTeamRelocate rewrites args["path"] to land under
// <base>/teams/<teamID>/<path> when the session is team-scoped and the tool is
// in needsTeamRelocate. It is a no-op when teamID is empty, the tool is not
// whitelisted, the path is absolute, the path is empty, or the path attempts
// a ".." escape after cleaning. Mutates args in place.
//
// The pure function shape (no SessionEntry dep, base passed in) keeps it
// trivially testable from a table-driven test.
func applyTeamRelocate(base, teamID, toolName string, args map[string]any) {
	if teamID == "" || !needsTeamRelocate(toolName) {
		return
	}
	rawPath, ok := getStringArg(args, "path")
	if !ok || rawPath == "" {
		return
	}
	// POSIX semantics — runtime container is Linux; the ACP wrapper sends
	// forward-slash paths. Using path.IsAbs (not filepath.IsAbs) keeps the
	// detection consistent across dev platforms (the Windows filepath.IsAbs
	// rejects POSIX absolute paths, which would re-rewrite '/tmp/x').
	if path.IsAbs(rawPath) {
		return
	}
	cleaned := filepath.Clean(rawPath)
	if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		slog.Warn("acp.shim.path_relocate_rejected",
			"tool", toolName, "team_id", teamID,
			"reason", "path_escape", "raw", rawPath)
		return
	}
	teamRoot := filepath.Join(base, "teams", teamID)
	newPath := filepath.Join(teamRoot, cleaned)
	args["path"] = newPath
	slog.Info("acp.shim.path_relocated",
		"tool", toolName, "team_id", teamID,
		"from", rawPath, "to", newPath)
}

// getStringArg returns args[key] coerced to string and a presence flag.
// Returns ("", false) when args is nil, the key is absent, or the value is
// not a string. Used by applyTeamRelocate to read the 'path' field safely.
func getStringArg(args map[string]any, key string) (string, bool) {
	if args == nil {
		return "", false
	}
	v, ok := args[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

// makeSessionAwareHandler returns a mcpserver.ToolHandlerFunc that:
//
//  1. Reads the per-session allowlist attached to ctx by Server.handleMCP;
//     if toolName is not in the allowlist, returns an MCP error result so the
//     caller (claude-agent-acp) surfaces the denial to the LLM rather than the
//     transport.
//
//  2. Reuses the same registry execution pattern as
//     mcp.NewBridgeServer's internal makeToolHandler — i.e. it calls
//     reg.ExecuteWithContext with the routing context already attached by
//     handleMCP, so per-tool semantics (sandbox, rate limit, panic recovery)
//     behave identically across the two paths.
//
//  3. Forwards tool-produced media to the outbound bus via
//     mcp.ForwardMediaToOutbound, so write_file deliver=true and similar
//     attachment-producing tools reach the user's channel even though the
//     Claude CLI never sees result.Media on its own.
//
// This is the security-relevant gate for the shim. If allowlistCtxKey is
// missing or the tool is not in the set, the handler short-circuits BEFORE
// invoking the registry.
func (s *Server) makeSessionAwareHandler(reg *tools.Registry, msgBus *bus.MessageBus, toolName string) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		start := time.Now()

		// Pull the *SessionEntry attached by handleMCP. May be nil in tests
		// that build a request without going through the HTTP path; treat
		// nil entry as "no rate limit, no sid" so the audit log degrades
		// gracefully instead of panicking.
		sess, _ := ctx.Value(sessionCtxKey{}).(*SessionEntry)
		sid := ""
		if sess != nil {
			sid = sess.SID
		}

		audit := func(status string) {
			slog.Info("acp.shim.tool_call",
				"sid", sid,
				"tool", toolName,
				"dur_ms", time.Since(start).Milliseconds(),
				"status", status,
			)
		}

		allow, ok := ctx.Value(allowlistCtxKey{}).(map[string]bool)
		if !ok || !allow[toolName] {
			audit("blocked")
			return mcpgo.NewToolResultError("tool not granted for this ACP session: " + toolName), nil
		}

		// Layer 4: per-session token-bucket rate limit. Empty bucket → reject
		// without dispatching to the registry. The error is returned as an
		// MCP tool result (not a transport error) so the LLM sees the denial
		// and can adapt rather than the ACP transport tearing down.
		if sess != nil && sess.rateBucket != nil && !sess.rateBucket.take(time.Now()) {
			audit("blocked")
			return mcpgo.NewToolResultError("rate_limit_exceeded: max 100 tool calls per 5 minutes per session"), nil
		}

		args := req.GetArguments()

		// Phase 5.1: relocate file-creation tools under the team workspace
		// root when this session is team-dispatched (sess.Cron.TeamID != "").
		// No-op for cron / direct sessions — preserves backward compat.
		if sess != nil {
			applyTeamRelocate(workspaceBase, sess.Cron.TeamID, toolName, args)
		}

		// Reuse the same routing context as mcp.NewBridgeServer's handler —
		// handleMCP already populated channel/chatID/peerKind/sessionKey from
		// SessionEntry.Cron, so reading them back here keeps the two dispatch
		// paths semantically identical.
		result := reg.ExecuteWithContext(ctx, toolName, args,
			tools.ToolChannelFromCtx(ctx),
			tools.ToolChatIDFromCtx(ctx),
			tools.ToolPeerKindFromCtx(ctx),
			tools.ToolSessionKeyFromCtx(ctx),
			nil,
		)

		if result.IsError {
			audit("error")
			return mcpgo.NewToolResultError(result.ForLLM), nil
		}

		// Forward media files to the outbound bus so they reach the user as
		// attachments. Reuses bridge_server.ForwardMediaToOutbound to avoid
		// drift between the two MCP server entry points.
		mcp.ForwardMediaToOutbound(ctx, msgBus, toolName, result)

		audit("ok")
		return mcpgo.NewToolResultText(result.ForLLM), nil
	}
}
