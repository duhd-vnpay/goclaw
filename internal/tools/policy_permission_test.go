package tools

import (
	"context"
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/config"
)

// mockPermTool implements Tool + PermissionRequirer for testing.
type mockPermTool struct {
	name string
	perm string
}

func (t *mockPermTool) Name() string                                    { return t.name }
func (t *mockPermTool) Description() string                             { return "test tool " + t.name }
func (t *mockPermTool) Parameters() map[string]any                      { return nil }
func (t *mockPermTool) Execute(_ context.Context, _ map[string]any) *Result { return nil }
func (t *mockPermTool) RequiredPermission() string                      { return t.perm }

// mockPlainTool implements Tool without PermissionRequirer.
type mockPlainTool struct {
	name string
}

func (t *mockPlainTool) Name() string                                    { return t.name }
func (t *mockPlainTool) Description() string                             { return "test tool " + t.name }
func (t *mockPlainTool) Parameters() map[string]any                      { return nil }
func (t *mockPlainTool) Execute(_ context.Context, _ map[string]any) *Result { return nil }

func TestFilterToolsWithPermissions(t *testing.T) {
	// Set up a registry with both regular and permission-gated tools.
	reg := NewRegistry()
	reg.Register(&mockPlainTool{name: "read_file"})
	reg.Register(&mockPlainTool{name: "list_files"})
	reg.Register(&mockPermTool{name: "deploy", perm: "can_deploy"})
	reg.Register(&mockPermTool{name: "approve", perm: "can_approve"})

	pe := NewPolicyEngine(&config.ToolsConfig{Profile: "full"})

	tests := []struct {
		name        string
		permissions map[string]bool
		wantTools   map[string]bool
	}{
		{
			name:        "user with all permissions sees all tools",
			permissions: map[string]bool{"can_deploy": true, "can_approve": true},
			wantTools:   map[string]bool{"read_file": true, "list_files": true, "deploy": true, "approve": true},
		},
		{
			name:        "user with partial permissions sees subset",
			permissions: map[string]bool{"can_deploy": true},
			wantTools:   map[string]bool{"read_file": true, "list_files": true, "deploy": true},
		},
		{
			name:        "user with no matching permissions sees only plain tools",
			permissions: map[string]bool{},
			wantTools:   map[string]bool{"read_file": true, "list_files": true},
		},
		{
			name:        "unpaired user (nil permissions) sees only plain tools",
			permissions: nil,
			wantTools:   map[string]bool{"read_file": true, "list_files": true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defs := pe.FilterToolsWithPermissions(reg, "test-agent", "openai", nil, nil, false, false, tt.permissions)

			got := make(map[string]bool, len(defs))
			for _, d := range defs {
				got[d.Function.Name] = true
			}

			for wantName := range tt.wantTools {
				if !got[wantName] {
					t.Errorf("expected tool %q in result, but not found", wantName)
				}
			}

			for gotName := range got {
				if !tt.wantTools[gotName] {
					t.Errorf("unexpected tool %q in result", gotName)
				}
			}
		})
	}
}
