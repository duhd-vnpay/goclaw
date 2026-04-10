package agent

import (
	"context"

	"github.com/google/uuid"
	"github.com/nextlevelbuilder/goclaw/internal/agent/extension"
)

// Extension is re-exported from the extension package to avoid import cycles.
// External packages should import github.com/nextlevelbuilder/goclaw/internal/agent/extension
type Extension = extension.Extension

// Re-export RunRequest and RunResult for backward compatibility
// (they are now defined in the extension package to avoid cycles).
type extensionRunRequest = extension.RunRequest
type extensionRunResult = extension.RunResult

// extensionRegistry holds registered extensions
type extensionRegistry struct {
	extensions []Extension
}

var globalExtensions = &extensionRegistry{}

// RegisterExtension adds an extension to the global registry.
// Call this from init() or startup code in extension packages.
func RegisterExtension(ext Extension) {
	globalExtensions.extensions = append(globalExtensions.extensions, ext)
}

// GetExtensions returns all registered extensions
func GetExtensions() []Extension {
	return globalExtensions.extensions
}

// buildExtensionSystemPrompt aggregates system prompt contributions from all extensions.
// Returns a slice of prompt sections (one per extension) to be appended to the system prompt.
func buildExtensionSystemPrompt(ctx context.Context, tenantID, agentID uuid.UUID, userID string) []string {
	var result []string
	for _, ext := range globalExtensions.extensions {
		if ext.Enabled() {
			if ctxAddition := ext.OnBuildSystemPrompt(ctx, tenantID, agentID, userID); ctxAddition != "" {
				result = append(result, ctxAddition)
			}
		}
	}
	return result
}
