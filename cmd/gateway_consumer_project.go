//go:build !sqlite && !sqliteonly

package cmd

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/nextlevelbuilder/goclaw/internal/bus"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// resolveProjectOverrides looks up the project bound to the message's channel+chatID.
// If a project is found, it injects the project ID and MCP overrides into the context.
// Returns the enriched context and the project data (nil if no project bound).
func resolveProjectOverrides(
	ctx context.Context,
	msg bus.InboundMessage,
	projectStore store.ProjectStore,
	channelType string,
) (context.Context, *store.ProjectData) {
	if projectStore == nil {
		return ctx, nil
	}

	project, err := projectStore.GetProjectByChatID(ctx, channelType, msg.ChatID)
	if err != nil {
		slog.Warn("project: lookup failed, continuing without project context",
			"channel_type", channelType, "chat_id", msg.ChatID, "error", err)
		return ctx, nil
	}
	if project == nil {
		return ctx, nil
	}

	slog.Debug("project: resolved",
		"project_id", project.ID,
		"name", project.Name,
		"channel_type", channelType,
		"chat_id", msg.ChatID)

	// Inject project ID into context (used by scope queries, memory isolation, workspace).
	ctx = store.WithProjectID(ctx, project.ID)

	// Load and inject MCP overrides.
	overrides, err := projectStore.GetMCPOverrides(ctx, project.ID)
	if err != nil {
		slog.Warn("project: failed to load MCP overrides, continuing without",
			"project_id", project.ID, "error", err)
	} else if len(overrides) > 0 {
		ctx = store.WithProjectOverrides(ctx, overrides)
		slog.Debug("project: MCP overrides loaded",
			"project_id", project.ID, "server_count", len(overrides))
	}

	return ctx, project
}

// checkProjectAgentAccess validates whether an agent is allowed in a project.
// Returns an error message if the agent is blocked, empty string if allowed.
func checkProjectAgentAccess(project *store.ProjectData, agentKey string) string {
	if project == nil {
		return ""
	}

	settings := project.ParseSettings()

	// Allowlist takes precedence: if set, agent MUST be in the list.
	if len(settings.AgentAllowlist) > 0 {
		for _, allowed := range settings.AgentAllowlist {
			if allowed == agentKey {
				return ""
			}
		}
		return fmt.Sprintf("Agent '%s' is not available in project '%s'. Allowed agents: %v",
			agentKey, project.Name, settings.AgentAllowlist)
	}

	// Denylist: if agent is in the list, block it.
	if len(settings.AgentDenylist) > 0 {
		for _, denied := range settings.AgentDenylist {
			if denied == agentKey {
				return fmt.Sprintf("Agent '%s' is not available in project '%s'",
					agentKey, project.Name)
			}
		}
	}

	return ""
}
