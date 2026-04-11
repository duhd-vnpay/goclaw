package unit

import (
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/agent"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

func TestBuildUserContextPrompt_Paired(t *testing.T) {
	profile := &store.UserProfile{
		ID:          uuid.New(),
		Email:       "hoang@vnpay.vn",
		DisplayName: "Hoang Du",
		TenantRole:  "admin",
		ProjectRole: "lead",
		Permissions: map[string]bool{
			"can_deploy":  true,
			"can_approve": true,
			"can_merge":   true,
			"can_delete":  false, // should NOT appear in output
		},
		Departments: []store.DepartmentMembership{
			{DepartmentName: "Engineering", Role: "lead", Title: "Backend Lead"},
			{DepartmentName: "DevOps", Role: "member", Title: ""},
		},
		Expertise:        []string{"golang", "k8s", "terraform"},
		Timezone:         "Asia/Ho_Chi_Minh",
		Availability:     "available",
		PreferredChannel: "telegram",
	}

	result := agent.BuildUserContextPrompt(profile, "telegram", "479273176")

	// Must start with section header
	if !strings.HasPrefix(result, "## Current User\n") {
		t.Errorf("expected '## Current User' header, got: %s", result)
	}

	// Required fields
	for _, want := range []string{
		"- Name: Hoang Du",
		"- Email: hoang@vnpay.vn",
		"- Tenant role: admin",
		"- Project role: lead",
		"Engineering (Backend Lead)",
		"DevOps",
		"golang, k8s, terraform",
		"can_approve",
		"can_deploy",
		"can_merge",
		"Asia/Ho_Chi_Minh",
		"available",
	} {
		if !strings.Contains(result, want) {
			t.Errorf("missing %q in output:\n%s", want, result)
		}
	}

	// can_delete=false should NOT appear
	if strings.Contains(result, "can_delete") {
		t.Errorf("can_delete (false) should not appear in output:\n%s", result)
	}

	// Should NOT contain "Anonymous"
	if strings.Contains(result, "Anonymous") {
		t.Errorf("paired user should not show Anonymous:\n%s", result)
	}
}

func TestBuildUserContextPrompt_Anonymous(t *testing.T) {
	result := agent.BuildUserContextPrompt(nil, "telegram", "479273176")

	if !strings.Contains(result, "## Current User") {
		t.Error("expected '## Current User' header for anonymous")
	}
	if !strings.Contains(result, "Anonymous") {
		t.Error("expected 'Anonymous' for nil profile")
	}
	if !strings.Contains(result, "telegram") {
		t.Error("expected channel type in anonymous output")
	}
	if !strings.Contains(result, "479273176") {
		t.Error("expected sender ID in anonymous output")
	}
	if !strings.Contains(result, "read-only") {
		t.Error("expected read-only notice for anonymous users")
	}
}

func TestBuildUserContextPrompt_EmptyAnonymous(t *testing.T) {
	// No channel, no sender — should return empty string
	result := agent.BuildUserContextPrompt(nil, "", "")
	if result != "" {
		t.Errorf("expected empty string for nil profile with no channel/sender, got: %s", result)
	}
}

func TestBuildUserContextPrompt_MinimalProfile(t *testing.T) {
	// Minimal profile: only email, no departments, no permissions, no expertise
	profile := &store.UserProfile{
		ID:    uuid.New(),
		Email: "test@example.com",
	}

	result := agent.BuildUserContextPrompt(profile, "telegram", "12345")

	if !strings.Contains(result, "## Current User") {
		t.Error("expected header")
	}
	// Name falls back to email when DisplayName is empty
	if !strings.Contains(result, "- Name: test@example.com") {
		t.Errorf("expected name fallback to email, got:\n%s", result)
	}
	// Should not contain Departments, Expertise, Permissions lines
	if strings.Contains(result, "- Departments:") {
		t.Error("should not have Departments line with empty list")
	}
	if strings.Contains(result, "- Expertise:") {
		t.Error("should not have Expertise line with empty list")
	}
	if strings.Contains(result, "- Permissions:") {
		t.Error("should not have Permissions line with empty map")
	}
}

func TestBuildUserContextPrompt_PermissionsDeterministic(t *testing.T) {
	// Verify permissions are sorted for deterministic LLM cache
	profile := &store.UserProfile{
		ID:    uuid.New(),
		Email: "test@example.com",
		Permissions: map[string]bool{
			"can_merge":   true,
			"can_approve": true,
			"can_deploy":  true,
		},
	}

	// Run multiple times to verify deterministic ordering
	first := agent.BuildUserContextPrompt(profile, "telegram", "12345")
	for i := 0; i < 10; i++ {
		result := agent.BuildUserContextPrompt(profile, "telegram", "12345")
		if result != first {
			t.Errorf("non-deterministic output on iteration %d:\nfirst: %s\ngot:   %s", i, first, result)
		}
	}

	// Verify sorted order
	if !strings.Contains(first, "can_approve, can_deploy, can_merge") {
		t.Errorf("permissions should be sorted, got:\n%s", first)
	}
}

func TestBuildUserContextPrompt_DepartmentTitleVsRole(t *testing.T) {
	// When title is set, show title. When title is empty and role is not "member", show role.
	profile := &store.UserProfile{
		ID:    uuid.New(),
		Email: "test@example.com",
		Departments: []store.DepartmentMembership{
			{DepartmentName: "Engineering", Role: "lead", Title: "Backend Lead"},
			{DepartmentName: "Security", Role: "lead", Title: ""},
			{DepartmentName: "Marketing", Role: "member", Title: ""},
		},
	}

	result := agent.BuildUserContextPrompt(profile, "telegram", "12345")

	if !strings.Contains(result, "Engineering (Backend Lead)") {
		t.Errorf("expected title for Engineering, got:\n%s", result)
	}
	if !strings.Contains(result, "Security (lead)") {
		t.Errorf("expected role for Security, got:\n%s", result)
	}
	// Marketing should appear without parenthetical
	if strings.Contains(result, "Marketing (member)") {
		t.Errorf("member role should not show in parens:\n%s", result)
	}
	if !strings.Contains(result, "Marketing") {
		t.Errorf("Marketing department should appear:\n%s", result)
	}
}
