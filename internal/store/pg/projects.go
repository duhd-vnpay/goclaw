package pg

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// PGProjectStore implements store.ProjectStore backed by Postgres.
type PGProjectStore struct {
	db *sql.DB
}

// NewPGProjectStore creates a PGProjectStore with the given connection.
func NewPGProjectStore(db *sql.DB) *PGProjectStore {
	return &PGProjectStore{db: db}
}

// GetProjectByChatID looks up a project by channel type + chat ID.
// Returns nil, nil when no project is bound to this chat.
func (s *PGProjectStore) GetProjectByChatID(ctx context.Context, channelType, chatID string) (*store.ProjectData, error) {
	if channelType == "" || chatID == "" {
		return nil, nil
	}

	tid := store.TenantIDFromContext(ctx)
	if tid == uuid.Nil {
		tid = store.MasterTenantID
	}

	var p store.ProjectData
	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, COALESCE(slug, '') AS slug, COALESCE(channel_type, '') AS channel_type,
		        COALESCE(chat_id, '') AS chat_id, tenant_id, domain_id, COALESCE(settings, '{}') AS settings
		 FROM projects
		 WHERE channel_type = $1 AND chat_id = $2 AND tenant_id = $3
		 LIMIT 1`,
		channelType, chatID, tid,
	).Scan(&p.ID, &p.Name, &p.Slug, &p.ChannelType, &p.ChatID, &p.TenantID, &p.DomainID, &p.Settings)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get project by chat_id: %w", err)
	}
	return &p, nil
}

// GetMCPOverrides returns MCP server env overrides for a project.
// Reads from project_resources where resource_type = 'mcp_server'.
func (s *PGProjectStore) GetMCPOverrides(ctx context.Context, projectID uuid.UUID) (map[string]map[string]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT config FROM project_resources
		 WHERE project_id = $1 AND resource_type = 'mcp_server' AND enabled = true`,
		projectID,
	)
	if err != nil {
		return nil, fmt.Errorf("get mcp overrides: %w", err)
	}
	defer rows.Close()

	overrides := make(map[string]map[string]string)
	for rows.Next() {
		var configBytes []byte
		if err := rows.Scan(&configBytes); err != nil {
			continue
		}
		var config map[string]any
		if err := json.Unmarshal(configBytes, &config); err != nil {
			continue
		}
		serverName, _ := config["server_name"].(string)
		if serverName == "" {
			continue
		}
		envMap, ok := config["env_overrides"].(map[string]any)
		if !ok {
			continue
		}
		envStr := make(map[string]string)
		for k, v := range envMap {
			if vs, ok := v.(string); ok {
				envStr[k] = vs
			} else {
				envStr[k] = fmt.Sprintf("%v", v)
			}
		}
		overrides[serverName] = envStr
	}
	return overrides, rows.Err()
}
