package store

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
)

// ProjectData represents a row from the projects table.
type ProjectData struct {
	ID          uuid.UUID       `json:"id" db:"id"`
	Name        string          `json:"name" db:"name"`
	Slug        string          `json:"slug" db:"slug"`
	ChannelType string          `json:"channel_type" db:"channel_type"`
	ChatID      string          `json:"chat_id" db:"chat_id"`
	TenantID    uuid.UUID       `json:"tenant_id" db:"tenant_id"`
	DomainID    *uuid.UUID      `json:"domain_id,omitempty" db:"domain_id"`
	Settings    json.RawMessage `json:"settings" db:"settings"`
}

// ProjectSettings represents parsed project settings JSONB.
type ProjectSettings struct {
	AgentAllowlist []string `json:"agent_allowlist,omitempty"`
	AgentDenylist  []string `json:"agent_denylist,omitempty"`
}

// ParseSettings parses the Settings JSONB into ProjectSettings.
func (p *ProjectData) ParseSettings() ProjectSettings {
	var s ProjectSettings
	if len(p.Settings) > 0 {
		_ = json.Unmarshal(p.Settings, &s)
	}
	return s
}

// ProjectStore manages project lookups and MCP override resolution.
type ProjectStore interface {
	// GetProjectByChatID looks up a project by channel type and chat ID.
	// Returns nil, nil when no project is bound to this chat.
	GetProjectByChatID(ctx context.Context, channelType, chatID string) (*ProjectData, error)

	// GetMCPOverrides returns MCP server env overrides for a project.
	// Outer map key = server name, inner map = env var name → value.
	GetMCPOverrides(ctx context.Context, projectID uuid.UUID) (map[string]map[string]string, error)
}
