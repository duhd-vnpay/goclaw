package ardenn

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
)

// ProjectResourceLoader loads resources for a project.
// Implemented by ProjectResourceStore.GetByProject.
type ProjectResourceLoader interface {
	LoadProjectResources(ctx context.Context, projectID uuid.UUID) ([]ProjectResourceData, error)
}

// ProjectResourceData is the minimal resource data needed for variable injection.
type ProjectResourceData struct {
	ResourceType string          `json:"resource_type"`
	ResourceKey  string          `json:"resource_key"`
	Config       json.RawMessage `json:"config"`
}

// ResolveProjectVariables loads project resources and flattens them into
// a nested map for template resolution:
//
//	project.resources.<resource_type>.<config_key> = <config_value>
//	project.resources.<resource_type>.__key__ = <resource_key>
//
// For example, a GitLab MCP server resource with config
// {"server_name":"gitlab-mcp","GITLAB_PROJECT_PATH":"group/repo"}
// becomes:
//
//	project.resources.mcp_server.server_name = "gitlab-mcp"
//	project.resources.mcp_server.GITLAB_PROJECT_PATH = "group/repo"
func ResolveProjectVariables(ctx context.Context, loader ProjectResourceLoader, projectID uuid.UUID) (map[string]any, error) {
	resources, err := loader.LoadProjectResources(ctx, projectID)
	if err != nil {
		return nil, err
	}

	// Build nested map: project -> resources -> <type> -> <key> -> <value>
	resourcesByType := map[string]any{}

	for _, r := range resources {
		var configMap map[string]any
		if err := json.Unmarshal(r.Config, &configMap); err != nil {
			// Skip resources with unparseable config
			continue
		}

		// If multiple resources of the same type, use resource_key as sub-key
		typeMap, ok := resourcesByType[r.ResourceType].(map[string]any)
		if !ok {
			typeMap = map[string]any{}
			resourcesByType[r.ResourceType] = typeMap
		}

		// Flatten config values under the resource key
		keyMap := map[string]any{}
		for k, v := range configMap {
			keyMap[k] = v
		}
		keyMap["__key__"] = r.ResourceKey
		typeMap[r.ResourceKey] = keyMap
	}

	return map[string]any{
		"project": map[string]any{
			"id":        projectID.String(),
			"resources": resourcesByType,
		},
	}, nil
}
