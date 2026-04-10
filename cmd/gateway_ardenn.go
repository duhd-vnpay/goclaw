//go:build !sqlite && !sqliteonly

package cmd

import (
	"log/slog"

	"github.com/nextlevelbuilder/goclaw/internal/ardenn"
	"github.com/nextlevelbuilder/goclaw/internal/ardenn/hands"
	"github.com/nextlevelbuilder/goclaw/internal/bus"
	"github.com/nextlevelbuilder/goclaw/internal/store"
	pgardenn "github.com/nextlevelbuilder/goclaw/internal/store/pg/ardenn"
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
