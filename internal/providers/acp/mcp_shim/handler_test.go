package mcp_shim

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/tools"
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
		// Idempotency cases (incident 2026-06-16) — path that already carries
		// the team segment must not get doubly nested under teamRoot.
		{
			name:     "idempotent — leading teams/<teamID>/ stripped",
			teamID:   "TEAM-A",
			tool:     "write_file",
			path:     "teams/TEAM-A/2026-06-16/world-news/draft.md",
			wantPath: filepath.Join(base, "teams", "TEAM-A", "2026-06-16/world-news/draft.md"),
		},
		{
			name:     "idempotent — leading <teamID>/ stripped",
			teamID:   "TEAM-A",
			tool:     "write_file",
			path:     "TEAM-A/2026-06-16/draft.md",
			wantPath: filepath.Join(base, "teams", "TEAM-A", "2026-06-16/draft.md"),
		},
		{
			name:     "idempotent — only ONE level stripped (no recursion)",
			teamID:   "TEAM-A",
			tool:     "write_file",
			path:     "teams/TEAM-A/teams/TEAM-A/2026-06-16/draft.md",
			wantPath: filepath.Join(base, "teams", "TEAM-A", "teams/TEAM-A/2026-06-16/draft.md"),
		},
		{
			name:     "other team prefix — preserved (not stripped)",
			teamID:   "TEAM-A",
			tool:     "write_file",
			path:     "teams/TEAM-B/foo.md",
			wantPath: filepath.Join(base, "teams", "TEAM-A", "teams/TEAM-B/foo.md"),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			args := map[string]any{"path": tc.path}
			applyTeamRelocate(base, tc.teamID, tc.tool, args, "")
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
	applyTeamRelocate("/app/data", "TEAM-A", "write_file", args, "")
	if _, ok := args["path"]; ok {
		t.Errorf("path arg should not be created when absent on input; got %v", args)
	}
}

// TestApplyTeamRelocate_EmptyArgs verifies nil-safety on the args map.
func TestApplyTeamRelocate_EmptyArgs(t *testing.T) {
	applyTeamRelocate("/app/data", "TEAM-A", "write_file", nil, "")
}

// TestNeedsTeamRelocate locks the whitelist to its current shape so future
// edits to the function must update this test (a forcing function — the spec
// requires the whitelist stay narrow).
func TestNeedsTeamRelocate(t *testing.T) {
	want := map[string]bool{
		"write_file":   true,
		"create_image": true,
		"tts":          true,
		"read_file":    false,
		"list_files":   false,
		"unknown_tool": false,
	}
	for name, expected := range want {
		if got := needsTeamRelocate(name); got != expected {
			t.Errorf("needsTeamRelocate(%q)=%v want %v", name, got, expected)
		}
	}
}

// TestTeamWorkspaceAnchor verifies the anchor derivation used by
// makeSessionAwareHandler: the ctx-attached team workspace wins (Option B,
// T0.1 2026-10-11), the canonical {GOCLAW_WORKSPACE}/teams/<teamID> is the
// fallback for callers that did not attach it, and non-team sessions get "" so
// applyTeamRelocate stays a no-op.
func TestTeamWorkspaceAnchor(t *testing.T) {
	ctxRoot := filepath.Join("ctx-scoped", "team-root")
	ctx := tools.WithToolWorkspace(context.Background(), ctxRoot)

	if got := teamWorkspaceAnchor(ctx, "TEAM-A"); got != ctxRoot {
		t.Errorf("ctx-attached anchor: got %q want %q", got, ctxRoot)
	}
	if got := teamWorkspaceAnchor(context.Background(), "TEAM-A"); got != teamWorkspaceRoot("TEAM-A") {
		t.Errorf("fallback anchor: got %q want %q", got, teamWorkspaceRoot("TEAM-A"))
	}
	if got := teamWorkspaceAnchor(ctx, ""); got != "" {
		t.Errorf("non-team anchor: got %q want empty", got)
	}
}

// TestApplyTeamRelocate_CtxTeamWorkspaceAnchor covers the Option B (T0.1,
// 2026-10-11) anchor: when handleMCP already attached the team workspace root to
// ctx, applyTeamRelocate must anchor on that value and never re-join
// base/teams/<teamID> on top of it (which would double-prefix an
// isolation-scoped team workspace). stripTeamPrefix idempotency (incident
// 2026-06-16) is preserved.
func TestApplyTeamRelocate_CtxTeamWorkspaceAnchor(t *testing.T) {
	const base = "/app/workspace"
	teamRoot := filepath.Join(base, "teams", "TEAM-A")
	isolationRoot := filepath.Join(teamRoot, "chat-7")

	cases := []struct {
		name      string
		teamID    string
		tool      string
		path      string
		ctxTeamWs string
		wantPath  string
	}{
		{
			name:      "relative path — anchored at ctx team workspace",
			teamID:    "TEAM-A",
			tool:      "write_file",
			path:      "2026-10-11/world-news/final_report.md",
			ctxTeamWs: teamRoot,
			wantPath:  filepath.Join(teamRoot, "2026-10-11/world-news/final_report.md"),
		},
		{
			name:      "teams/<T>/ prefixed path — prefix stripped, single level",
			teamID:    "TEAM-A",
			tool:      "write_file",
			path:      "teams/TEAM-A/2026-10-11/final_report.md",
			ctxTeamWs: teamRoot,
			wantPath:  filepath.Join(teamRoot, "2026-10-11/final_report.md"),
		},
		{
			name:      "bare <T>/ prefixed path — prefix stripped",
			teamID:    "TEAM-A",
			tool:      "write_file",
			path:      "TEAM-A/2026-10-11/final_report.md",
			ctxTeamWs: teamRoot,
			wantPath:  filepath.Join(teamRoot, "2026-10-11/final_report.md"),
		},
		{
			name:      "absolute path — untouched",
			teamID:    "TEAM-A",
			tool:      "write_file",
			path:      "/tmp/final_report.md",
			ctxTeamWs: teamRoot,
			wantPath:  "/tmp/final_report.md",
		},
		{
			name:      "isolation-scoped ctx workspace — no re-join of teams/<T>",
			teamID:    "TEAM-A",
			tool:      "write_file",
			path:      "draft.md",
			ctxTeamWs: isolationRoot,
			wantPath:  filepath.Join(isolationRoot, "draft.md"),
		},
		{
			name:      "non-team session — no-op even with ctx workspace",
			teamID:    "",
			tool:      "write_file",
			path:      "draft.md",
			ctxTeamWs: teamRoot,
			wantPath:  "draft.md",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			args := map[string]any{"path": tc.path}
			applyTeamRelocate(base, tc.teamID, tc.tool, args, tc.ctxTeamWs)
			got, _ := args["path"].(string)
			if got != tc.wantPath {
				t.Errorf("args[path]=%q want %q", got, tc.wantPath)
			}
			// Double-prefix guard: the team segment must appear at most once, so
			// a ctx workspace that already carries it is not joined again.
			if n := strings.Count(got, "teams"+string(filepath.Separator)+"TEAM-A"); n > 1 {
				t.Errorf("team segment repeated %d times in %q (double-prefix)", n, got)
			}
		})
	}
}

// TestTeamWorkspaceWriteReadRoundTrip proves the Option B fix at the
// path-resolution layer that actually matters: for the same relative path, the
// write-side relocate (handler.go) and the read-side ctx workspace resolution
// (read_file/list_files via tools.ToolWorkspaceFromCtx) must land on the SAME
// file. Before T0.1 the write went to {base}/teams/<T>/… while the read fell
// back to the global workspace — the 2026-10-11 WN split-brain (ADR-004).
func TestTeamWorkspaceWriteReadRoundTrip(t *testing.T) {
	base := t.TempDir()
	teamRoot := filepath.Join(base, "teams", "TEAM-A")
	if err := os.MkdirAll(teamRoot, 0o755); err != nil {
		t.Fatalf("mkdir team root: %v", err)
	}

	// Exactly what handleMCP attaches for a team-dispatched ACP session.
	ctx := tools.WithToolWorkspace(context.Background(), teamRoot)

	const rel = "2026-10-11/world-news/final_report.md"

	// Write side: the shim handler relocates the tool arg before dispatch.
	args := map[string]any{"path": rel}
	applyTeamRelocate(workspaceBase, "TEAM-A", "write_file", args, teamWorkspaceAnchor(ctx, "TEAM-A"))
	writePath, _ := args["path"].(string)
	if !filepath.IsAbs(writePath) {
		t.Fatalf("relocate produced a non-absolute write path: %q", writePath)
	}

	content := []byte("WN 2026-10-11 newsletter: final report")
	if err := os.MkdirAll(filepath.Dir(writePath), 0o755); err != nil {
		t.Fatalf("mkdir write dir: %v", err)
	}
	if err := os.WriteFile(writePath, content, 0o644); err != nil {
		t.Fatalf("write_file target: %v", err)
	}

	// Read side: read_file resolves the SAME relative arg against ctx workspace
	// (tools/filesystem.go resolvePath: filepath.Clean(filepath.Join(workspace, path))).
	readPath := filepath.Clean(filepath.Join(tools.ToolWorkspaceFromCtx(ctx), rel))

	if readPath != writePath {
		t.Errorf("write path %q != read path %q — split-brain not fixed", writePath, readPath)
	}
	got, err := os.ReadFile(readPath)
	if err != nil {
		t.Fatalf("read_file target unreadable (split-brain): %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("round-trip content mismatch: got %q want %q", got, content)
	}
}
