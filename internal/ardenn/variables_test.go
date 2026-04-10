package ardenn

import "testing"

func TestResolveTemplate(t *testing.T) {
	vars := map[string]any{
		"feature_name": "installment payment",
		"project": map[string]any{
			"name": "XPOS",
			"slug": "xpos",
		},
	}

	tests := []struct {
		template string
		want     string
	}{
		{"Review {{.feature_name}}", "Review installment payment"},
		{"No vars here", "No vars here"},
		{"", ""},
	}

	for _, tt := range tests {
		got := ResolveTemplate(tt.template, vars)
		if got != tt.want {
			t.Errorf("ResolveTemplate(%q) = %q, want %q", tt.template, got, tt.want)
		}
	}
}

func TestMergeVariables(t *testing.T) {
	defaults := map[string]any{"a": "1", "b": "2"}
	overrides := map[string]any{"b": "3", "c": "4"}

	merged := MergeVariables(defaults, overrides)

	if merged["a"] != "1" {
		t.Errorf("a = %v, want 1", merged["a"])
	}
	if merged["b"] != "3" {
		t.Errorf("b = %v, want 3 (override)", merged["b"])
	}
	if merged["c"] != "4" {
		t.Errorf("c = %v, want 4", merged["c"])
	}
}
