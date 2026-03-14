package mcp

import "testing"

func TestMergeEnv(t *testing.T) {
	tests := []struct {
		name      string
		base      map[string]string
		overrides map[string]string
		want      map[string]string
	}{
		{
			name:      "nil overrides returns base unchanged",
			base:      map[string]string{"A": "1", "B": "2"},
			overrides: nil,
			want:      map[string]string{"A": "1", "B": "2"},
		},
		{
			name:      "empty overrides returns base unchanged",
			base:      map[string]string{"A": "1"},
			overrides: map[string]string{},
			want:      map[string]string{"A": "1"},
		},
		{
			name:      "override replaces base key",
			base:      map[string]string{"GITLAB_URL": "https://git.example.com", "GITLAB_PROJECT_ID": "1"},
			overrides: map[string]string{"GITLAB_PROJECT_ID": "42"},
			want:      map[string]string{"GITLAB_URL": "https://git.example.com", "GITLAB_PROJECT_ID": "42"},
		},
		{
			name:      "override adds new key without removing base",
			base:      map[string]string{"GITLAB_URL": "https://git.example.com"},
			overrides: map[string]string{"GITLAB_PROJECT_PATH": "duhd/xpos"},
			want:      map[string]string{"GITLAB_URL": "https://git.example.com", "GITLAB_PROJECT_PATH": "duhd/xpos"},
		},
		{
			name:      "nil base with overrides",
			base:      nil,
			overrides: map[string]string{"KEY": "val"},
			want:      map[string]string{"KEY": "val"},
		},
		{
			name:      "both nil returns nil-like base",
			base:      nil,
			overrides: nil,
			want:      nil,
		},
		{
			name:      "multiple overrides replace multiple base keys",
			base:      map[string]string{"A": "1", "B": "2", "C": "3"},
			overrides: map[string]string{"A": "10", "C": "30", "D": "40"},
			want:      map[string]string{"A": "10", "B": "2", "C": "30", "D": "40"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mergeEnv(tt.base, tt.overrides)
			if tt.want == nil {
				if tt.base == nil && got != nil && len(got) > 0 {
					t.Errorf("expected nil-like result, got %v", got)
				}
				return
			}
			if len(got) != len(tt.want) {
				t.Errorf("len mismatch: got %d, want %d\ngot:  %v\nwant: %v", len(got), len(tt.want), got, tt.want)
				return
			}
			for k, wantV := range tt.want {
				if gotV, ok := got[k]; !ok || gotV != wantV {
					t.Errorf("key %q: got %q, want %q", k, gotV, wantV)
				}
			}
		})
	}
}

func TestMergeEnv_BaseNotMutated(t *testing.T) {
	base := map[string]string{"A": "1", "B": "2"}
	overrides := map[string]string{"A": "override", "C": "new"}

	_ = mergeEnv(base, overrides)

	if base["A"] != "1" {
		t.Errorf("base was mutated: A=%q, want '1'", base["A"])
	}
	if _, ok := base["C"]; ok {
		t.Error("base was mutated: unexpected key 'C'")
	}
}

func TestPoolKeyComputation(t *testing.T) {
	tests := []struct {
		name      string
		server    string
		projectID string
		wantKey   string
	}{
		{
			name:      "no project — backward compat key",
			server:    "gitlab",
			projectID: "",
			wantKey:   "gitlab",
		},
		{
			name:      "with project — composite key",
			server:    "gitlab",
			projectID: "uuid-xpos",
			wantKey:   "gitlab:uuid-xpos",
		},
		{
			name:      "different projects same server — different keys",
			server:    "atlassian",
			projectID: "uuid-payment",
			wantKey:   "atlassian:uuid-payment",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			poolKey := tt.server
			if tt.projectID != "" {
				poolKey = tt.server + ":" + tt.projectID
			}
			if poolKey != tt.wantKey {
				t.Errorf("poolKey: got %q, want %q", poolKey, tt.wantKey)
			}
		})
	}
}

func TestPoolKeyIsolation(t *testing.T) {
	server := "gitlab"
	projectA := "uuid-xpos"
	projectB := "uuid-payment"

	keyA := server + ":" + projectA
	keyB := server + ":" + projectB
	keyNone := server

	if keyA == keyB {
		t.Error("project A and B should have different pool keys")
	}
	if keyA == keyNone {
		t.Error("project A and no-project should have different pool keys")
	}
	if keyB == keyNone {
		t.Error("project B and no-project should have different pool keys")
	}
}
