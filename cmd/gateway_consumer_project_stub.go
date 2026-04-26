//go:build sqlite || sqliteonly

package cmd

import (
	"context"

	"github.com/nextlevelbuilder/goclaw/internal/bus"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// resolveProjectOverrides is a no-op in sqliteonly builds: project-as-a-channel
// requires the PG ProjectStore, which is not wired into the SQLite/desktop edition.
func resolveProjectOverrides(
	ctx context.Context,
	_ bus.InboundMessage,
	_ store.ProjectStore,
	_ string,
) (context.Context, *store.ProjectData) {
	return ctx, nil
}

// checkProjectAgentAccess is a no-op in sqliteonly builds. Without projects there
// are no project-level agent allow/deny lists to evaluate.
func checkProjectAgentAccess(_ *store.ProjectData, _ string) string {
	return ""
}
