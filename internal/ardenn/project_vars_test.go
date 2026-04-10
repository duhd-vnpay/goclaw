package ardenn

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
)

type mockResourceLoader struct {
	resources []ProjectResourceData
}

func (m *mockResourceLoader) LoadProjectResources(_ context.Context, _ uuid.UUID) ([]ProjectResourceData, error) {
	return m.resources, nil
}

func TestResolveProjectVariables_MCPServer(t *testing.T) {
	loader := &mockResourceLoader{
		resources: []ProjectResourceData{
			{
				ResourceType: "mcp_server",
				ResourceKey:  "gitlab-mcp",
				Config:       json.RawMessage(`{"server_name":"gitlab-mcp","GITLAB_PROJECT_PATH":"group/repo"}`),
			},
		},
	}

	projectID := uuid.New()
	vars, err := ResolveProjectVariables(context.Background(), loader, projectID)
	if err != nil {
		t.Fatal(err)
	}

	// Navigate: project.resources.mcp_server.gitlab-mcp.GITLAB_PROJECT_PATH
	project, ok := vars["project"].(map[string]any)
	if !ok {
		t.Fatal("missing project")
	}
	resources, ok := project["resources"].(map[string]any)
	if !ok {
		t.Fatal("missing resources")
	}
	mcpType, ok := resources["mcp_server"].(map[string]any)
	if !ok {
		t.Fatal("missing mcp_server type")
	}
	gitlab, ok := mcpType["gitlab-mcp"].(map[string]any)
	if !ok {
		t.Fatal("missing gitlab-mcp resource")
	}
	path, ok := gitlab["GITLAB_PROJECT_PATH"].(string)
	if !ok || path != "group/repo" {
		t.Errorf("path = %q, want group/repo", path)
	}
	key, ok := gitlab["__key__"].(string)
	if !ok || key != "gitlab-mcp" {
		t.Errorf("__key__ = %q, want gitlab-mcp", key)
	}
}

func TestResolveProjectVariables_Empty(t *testing.T) {
	loader := &mockResourceLoader{resources: nil}

	vars, err := ResolveProjectVariables(context.Background(), loader, uuid.New())
	if err != nil {
		t.Fatal(err)
	}

	project, ok := vars["project"].(map[string]any)
	if !ok {
		t.Fatal("missing project")
	}
	resources, ok := project["resources"].(map[string]any)
	if !ok {
		t.Fatal("missing resources")
	}
	if len(resources) != 0 {
		t.Errorf("expected empty resources, got %d", len(resources))
	}
}

func TestResolveProjectVariables_MultipleTypes(t *testing.T) {
	loader := &mockResourceLoader{
		resources: []ProjectResourceData{
			{ResourceType: "mcp_server", ResourceKey: "gitlab-mcp", Config: json.RawMessage(`{"server_name":"gitlab-mcp"}`)},
			{ResourceType: "git_repo", ResourceKey: "main", Config: json.RawMessage(`{"url":"https://git.vnpay.vn/group/repo.git"}`)},
			{ResourceType: "mcp_server", ResourceKey: "jira-mcp", Config: json.RawMessage(`{"server_name":"jira-mcp"}`)},
		},
	}

	vars, err := ResolveProjectVariables(context.Background(), loader, uuid.New())
	if err != nil {
		t.Fatal(err)
	}

	project := vars["project"].(map[string]any)
	resources := project["resources"].(map[string]any)

	// Should have 2 resource types
	if len(resources) != 2 {
		t.Errorf("resource types = %d, want 2", len(resources))
	}

	// mcp_server should have 2 resources
	mcpType := resources["mcp_server"].(map[string]any)
	if len(mcpType) != 2 {
		t.Errorf("mcp_server resources = %d, want 2", len(mcpType))
	}
}

func TestResolveProjectVariables_InvalidConfig(t *testing.T) {
	loader := &mockResourceLoader{
		resources: []ProjectResourceData{
			{ResourceType: "bad", ResourceKey: "broken", Config: json.RawMessage(`not json`)},
			{ResourceType: "good", ResourceKey: "ok", Config: json.RawMessage(`{"key":"val"}`)},
		},
	}

	vars, err := ResolveProjectVariables(context.Background(), loader, uuid.New())
	if err != nil {
		t.Fatal(err)
	}

	project := vars["project"].(map[string]any)
	resources := project["resources"].(map[string]any)

	// bad resource should be skipped, good should remain
	if _, ok := resources["bad"]; ok {
		t.Error("bad resource should be skipped")
	}
	if _, ok := resources["good"]; !ok {
		t.Error("good resource should be present")
	}
}
