package mcp_shim

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

// fakeGrantsStore is the test double for the real GrantsStore the resolver
// queries. Tests build a list and the fake just returns it.
type fakeGrantsStore struct {
	grants []GrantedTool
}

func (f *fakeGrantsStore) ListGrantedTools(ctx context.Context, agentID, tenantID string) ([]GrantedTool, error) {
	return f.grants, nil
}

func TestResolver_FiltersHardBlacklist(t *testing.T) {
	store := &fakeGrantsStore{
		grants: []GrantedTool{
			{Name: "mcp_ops__litellm_psql_query", Kind: HandlerKindMCPManager, MCPServer: "ops", MCPName: "litellm_psql_query", Schema: json.RawMessage(`{}`), Timeout: 30 * time.Second},
			{Name: "delegate", Kind: HandlerKindBuiltin, BuiltinName: "delegate", Schema: json.RawMessage(`{}`)},
			{Name: "cron_run", Kind: HandlerKindBuiltin, BuiltinName: "cron_run", Schema: json.RawMessage(`{}`)},
			{Name: "subagent_dispatch", Kind: HandlerKindBuiltin, BuiltinName: "subagent_dispatch", Schema: json.RawMessage(`{}`)},
			{Name: "write_file", Kind: HandlerKindBuiltin, BuiltinName: "write_file", Schema: json.RawMessage(`{}`)},
		},
	}
	r := NewResolver(store)
	got, err := r.ResolveToolSlice(context.Background(), "agent-1", "tenant-1")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	want := map[string]bool{"mcp_ops__litellm_psql_query": true, "write_file": true}
	if len(got) != len(want) {
		t.Fatalf("expected %d tools, got %d: %v", len(want), len(got), names(got))
	}
	for _, td := range got {
		if !want[td.Name] {
			t.Errorf("unexpected tool in slice: %s", td.Name)
		}
	}
}

func TestResolver_BlacklistGlobMatch(t *testing.T) {
	cases := []struct {
		name    string
		blocked bool
	}{
		{"subagent_run", true}, {"subagent_status", true},
		{"cron_create", true}, {"cron_update", true},
		{"team_task_create", true},
		{"agent_create", true}, {"agent_update", true},
		{"shell_exec_write", true},
		{"shell_exec_read", false}, // explicitly allowed
		{"mcp_tool_search", true},
		{"mcp_ops__litellm_psql_query", false},
	}
	r := NewResolver(nil) // we only test the predicate
	for _, c := range cases {
		if got := r.isBlacklisted(c.name); got != c.blocked {
			t.Errorf("isBlacklisted(%q) = %v, want %v", c.name, got, c.blocked)
		}
	}
}

func TestResolver_EmptyGrants(t *testing.T) {
	r := NewResolver(&fakeGrantsStore{})
	got, err := r.ResolveToolSlice(context.Background(), "a", "t")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty slice, got %v", names(got))
	}
}

func names(ts []ToolDescriptor) []string {
	out := make([]string, len(ts))
	for i, t := range ts {
		out[i] = t.Name
	}
	return out
}
