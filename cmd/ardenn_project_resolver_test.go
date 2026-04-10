//go:build !sqlite && !sqliteonly

package cmd

import (
	"encoding/json"
	"testing"
)

func TestParseMCPOverrides(t *testing.T) {
	config := map[string]any{
		"server_name":   "gitlab-mcp",
		"env_overrides": map[string]any{"GITLAB_PROJECT_PATH": "group/repo"},
	}
	raw, _ := json.Marshal(config)

	rows := []mcpConfigRow{{Config: raw}}
	overrides := parseMCPOverrides(rows)
	if len(overrides) != 1 {
		t.Fatalf("got %d overrides, want 1", len(overrides))
	}
	if overrides[0].ServerName != "gitlab-mcp" {
		t.Errorf("server_name = %q, want gitlab-mcp", overrides[0].ServerName)
	}
	path, ok := overrides[0].EnvOverrides["GITLAB_PROJECT_PATH"].(string)
	if !ok || path != "group/repo" {
		t.Errorf("GITLAB_PROJECT_PATH = %q, want group/repo", path)
	}
}

func TestParseMCPOverrides_BadJSON(t *testing.T) {
	rows := []mcpConfigRow{{Config: []byte("not json")}}
	overrides := parseMCPOverrides(rows)
	if len(overrides) != 0 {
		t.Errorf("expected 0 overrides for bad JSON, got %d", len(overrides))
	}
}
