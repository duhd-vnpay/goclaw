//go:build !sqlite && !sqliteonly

package cmd

import (
	"log/slog"

	"github.com/nextlevelbuilder/goclaw/internal/ardenn"
	"github.com/nextlevelbuilder/goclaw/internal/ardenn/hands"
	"github.com/nextlevelbuilder/goclaw/internal/bus"
	"github.com/nextlevelbuilder/goclaw/internal/gateway"
	"github.com/nextlevelbuilder/goclaw/internal/gateway/methods"
	"github.com/nextlevelbuilder/goclaw/internal/store"
	pgardenn "github.com/nextlevelbuilder/goclaw/internal/store/pg/ardenn"
	"github.com/nextlevelbuilder/goclaw/internal/tools"
)

// initArdenn wires the Ardenn workflow engine from PG stores.
// Returns nil, nil when Ardenn stores are not available (e.g., SQLite build).
func initArdenn(stores *store.Stores, msgBus *bus.MessageBus) (*ardenn.Engine, *hands.CompletionRegistry) {
	if stores.ArdennEvents == nil {
		slog.Debug("ardenn: skipped (no event store)")
		return nil, nil
	}

	// Type-assert the interface{} fields to concrete types.
	eventStore, ok := stores.ArdennEvents.(*pgardenn.PGEventStore)
	if !ok {
		slog.Warn("ardenn: ArdennEvents is not *pgardenn.PGEventStore, skipping")
		return nil, nil
	}

	completion := hands.NewCompletionRegistry()
	agentHand := hands.NewAgentHand(msgBus, completion)

	registry := ardenn.NewHandRegistry()
	registry.Register(agentHand)

	engine := ardenn.NewEngine(eventStore, registry)

	slog.Info("ardenn: engine initialized",
		"event_store", "pg",
		"hands", []string{"agent"},
	)

	return engine, completion
}

// registerArdennTool registers the ardenn_workflow tool in the tool registry.
func registerArdennTool(
	engine *ardenn.Engine,
	stores *store.Stores,
	toolsReg *tools.Registry,
) {
	if engine == nil {
		return
	}
	defStore, ok := stores.ArdennDefinitions.(*pgardenn.PGDefinitionStore)
	if !ok {
		slog.Warn("ardenn: cannot register tool — definition store unavailable")
		return
	}
	projStore, ok := stores.ArdennProjections.(*pgardenn.PGProjectionStore)
	if !ok {
		slog.Warn("ardenn: cannot register tool — projection store unavailable")
		return
	}
	toolsReg.Register(tools.NewArdennWorkflowTool(engine, defStore, projStore))
	slog.Info("ardenn_workflow tool registered")
}

// RegisterArdennMethods wires Ardenn RPC handlers into the gateway method router.
func RegisterArdennMethods(router *gateway.MethodRouter, eng *ardenn.Engine, stores *store.Stores) {
	if eng == nil {
		slog.Debug("ardenn: skipping RPC method registration (engine nil)")
		return
	}
	defStore, ok := stores.ArdennDefinitions.(*pgardenn.PGDefinitionStore)
	if !ok {
		slog.Warn("ardenn: cannot register RPC methods — definition store unavailable")
		return
	}
	projStore, ok := stores.ArdennProjections.(*pgardenn.PGProjectionStore)
	if !ok {
		slog.Warn("ardenn: cannot register RPC methods — projection store unavailable")
		return
	}
	methods.NewArdennMethods(defStore, projStore, eng).Register(router)
	slog.Info("ardenn: registered gateway RPC methods")
}
