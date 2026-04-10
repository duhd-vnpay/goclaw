//go:build !sqlite && !sqliteonly

package cmd

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// MCPOverride represents an MCP server override for a project.
type MCPOverride struct {
	ServerName   string         `json:"server_name"`
	EnvOverrides map[string]any `json:"env_overrides"`
}

// mcpConfigRow holds raw config bytes from project_resources.
type mcpConfigRow struct {
	Config []byte `db:"config"`
}

// ResolveProjectMCPOverrides loads MCP server resources for a project.
// First tries project_resources table, falls back to project_mcp_overrides_compat view.
func ResolveProjectMCPOverrides(ctx context.Context, db *sqlx.DB, projectID uuid.UUID) ([]MCPOverride, error) {
	// Primary: read from project_resources
	var rows []mcpConfigRow
	err := db.SelectContext(ctx, &rows,
		`SELECT config FROM project_resources
		 WHERE project_id = $1 AND resource_type = 'mcp_server' AND enabled = true`,
		projectID,
	)
	if err == nil && len(rows) > 0 {
		return parseMCPOverrides(rows), nil
	}

	// Fallback: read from compat view (during migration period)
	slog.Debug("ardenn: falling back to project_mcp_overrides_compat",
		"project_id", projectID)
	type compatRow struct {
		ServerName   string `db:"server_name"`
		EnvOverrides []byte `db:"env_overrides"`
	}
	var compatRows []compatRow
	err = db.SelectContext(ctx, &compatRows,
		`SELECT server_name, env_overrides FROM project_mcp_overrides_compat
		 WHERE project_id = $1`,
		projectID,
	)
	if err != nil {
		return nil, err
	}

	var overrides []MCPOverride
	for _, cr := range compatRows {
		override := MCPOverride{ServerName: cr.ServerName}
		if len(cr.EnvOverrides) > 0 {
			var envMap map[string]any
			if jsonErr := json.Unmarshal(cr.EnvOverrides, &envMap); jsonErr == nil {
				override.EnvOverrides = envMap
			}
		}
		overrides = append(overrides, override)
	}
	return overrides, nil
}

// parseMCPOverrides extracts MCPOverride values from raw config JSON rows.
func parseMCPOverrides(rows []mcpConfigRow) []MCPOverride {
	var overrides []MCPOverride
	for _, r := range rows {
		var config map[string]any
		if err := json.Unmarshal(r.Config, &config); err != nil {
			continue
		}
		override := MCPOverride{}
		if sn, ok := config["server_name"].(string); ok {
			override.ServerName = sn
		}
		if env, ok := config["env_overrides"].(map[string]any); ok {
			override.EnvOverrides = env
		}
		if override.ServerName != "" {
			overrides = append(overrides, override)
		}
	}
	return overrides
}
