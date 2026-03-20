package cmd

// Local VNPAY tool additions — kept in a separate file to avoid merge conflicts
// with upstream gateway_builtin_tools.go changes.
//
// This file:
// 1. Appends local-only builtin tools to the seed data
// 2. Registers local tool group members (policy.go groups)
//
// When upstream adds the same tools, remove them from here.

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/nextlevelbuilder/goclaw/internal/store"
	"github.com/nextlevelbuilder/goclaw/internal/tools"
)

// localBuiltinTools returns tool definitions specific to this fork.
func localBuiltinTools() []store.BuiltinToolDef {
	return []store.BuiltinToolDef{
		// teams — team_message (upstream only has team_tasks)
		{Name: "team_message", DisplayName: "Team Message", Description: "Send a direct message or broadcast to teammates in the agent team", Category: "teams", Enabled: true,
			Requires: []string{"managed_mode", "teams"},
		},

		// gateway — internal_api for project management
		{Name: "internal_api", DisplayName: "Internal API", Description: "Call GoClaw's project management REST API (allowlist-controlled). Agents use this to resolve projects and configure MCP overrides.", Category: "gateway", Enabled: true,
			Settings: json.RawMessage(`{"allowed_routes":[{"method":"GET","prefix":"/v1/projects/by-chat"},{"method":"POST","prefix":"/v1/projects"},{"method":"PUT","prefix":"/v1/projects/"}]}`),
			Metadata: json.RawMessage(`{"config_hint":"Edit allowed_routes to control which endpoints agents can call"}`),
		},
	}
}

// seedLocalBuiltinTools seeds local-only tools into the database.
// Called after upstream seedBuiltinTools to add fork-specific tools.
func seedLocalBuiltinTools(ctx context.Context, bts store.BuiltinToolStore) {
	seeds := localBuiltinTools()
	if err := bts.Seed(ctx, seeds); err != nil {
		slog.Error("failed to seed local builtin tools", "error", err)
		return
	}
	slog.Info("local builtin tools seeded", "count", len(seeds))
}

// registerLocalToolGroups adds local tools to the "goclaw" policy group
// so they are included in the default tool allowlist.
func registerLocalToolGroups() {
	tools.AppendToToolGroup("goclaw", "team_message", "internal_api")
	tools.AppendToToolGroup("team", "team_message")
}
