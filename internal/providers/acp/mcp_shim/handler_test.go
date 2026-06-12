package mcp_shim

import (
	"path/filepath"
	"testing"
)

// TestApplyTeamRelocate covers the 5 edge cases listed in the Phase 5.1
// design spec (docs/superpowers/specs/2026-06-12-phase5.1-acp-team-workspace-routing-design.md
// §Edge cases).
func TestApplyTeamRelocate(t *testing.T) {
	const base = "/app/data"

	cases := []struct {
		name     string
		teamID   string
		tool     string
		path     string
		wantPath string // expected args["path"] after call
	}{
		{
			name:     "team write_file relative — rewritten",
			teamID:   "TEAM-A",
			tool:     "write_file",
			path:     "report.md",
			wantPath: filepath.Join(base, "teams", "TEAM-A", "report.md"),
		},
		{
			name:     "non-team session — skip",
			teamID:   "",
			tool:     "write_file",
			path:     "report.md",
			wantPath: "report.md",
		},
		{
			name:     "team but absolute path — skip",
			teamID:   "TEAM-A",
			tool:     "write_file",
			path:     "/tmp/x.md",
			wantPath: "/tmp/x.md",
		},
		{
			name:     "team but tool not in whitelist — skip",
			teamID:   "TEAM-A",
			tool:     "read_file",
			path:     "report.md",
			wantPath: "report.md",
		},
		{
			name:     "team path escape (..) — rejected, unchanged",
			teamID:   "TEAM-A",
			tool:     "write_file",
			path:     "../escape.md",
			wantPath: "../escape.md",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			args := map[string]any{"path": tc.path}
			applyTeamRelocate(base, tc.teamID, tc.tool, args)
			got, _ := args["path"].(string)
			if got != tc.wantPath {
				t.Errorf("args[path]=%q want %q", got, tc.wantPath)
			}
		})
	}
}

// TestApplyTeamRelocate_NoPathArg verifies that calls without a "path"
// argument (e.g. tools whose schema does not include one) are no-ops even
// when the session is team-scoped.
func TestApplyTeamRelocate_NoPathArg(t *testing.T) {
	args := map[string]any{"content": "hello"}
	applyTeamRelocate("/app/data", "TEAM-A", "write_file", args)
	if _, ok := args["path"]; ok {
		t.Errorf("path arg should not be created when absent on input; got %v", args)
	}
}

// TestApplyTeamRelocate_EmptyArgs verifies nil-safety on the args map.
func TestApplyTeamRelocate_EmptyArgs(t *testing.T) {
	applyTeamRelocate("/app/data", "TEAM-A", "write_file", nil)
}

// TestNeedsTeamRelocate locks the whitelist to its current shape so future
// edits to the function must update this test (a forcing function — the spec
// requires the whitelist stay narrow).
func TestNeedsTeamRelocate(t *testing.T) {
	want := map[string]bool{
		"write_file":    true,
		"create_image":  true,
		"tts":           true,
		"read_file":     false,
		"list_files":    false,
		"unknown_tool":  false,
	}
	for name, expected := range want {
		if got := needsTeamRelocate(name); got != expected {
			t.Errorf("needsTeamRelocate(%q)=%v want %v", name, got, expected)
		}
	}
}
