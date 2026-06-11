package mcp_shim

import (
	"context"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/nextlevelbuilder/goclaw/internal/bus"
	"github.com/nextlevelbuilder/goclaw/internal/mcp"
	"github.com/nextlevelbuilder/goclaw/internal/tools"
)

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
		allow, ok := ctx.Value(allowlistCtxKey{}).(map[string]bool)
		if !ok || !allow[toolName] {
			return mcpgo.NewToolResultError("tool not granted for this ACP session: " + toolName), nil
		}

		args := req.GetArguments()

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
			return mcpgo.NewToolResultError(result.ForLLM), nil
		}

		// Forward media files to the outbound bus so they reach the user as
		// attachments. Reuses bridge_server.ForwardMediaToOutbound to avoid
		// drift between the two MCP server entry points.
		mcp.ForwardMediaToOutbound(ctx, msgBus, toolName, result)

		return mcpgo.NewToolResultText(result.ForLLM), nil
	}
}
