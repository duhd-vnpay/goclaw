//go:build !sqlite && !sqliteonly

package cmd

import (
	"context"
	"log/slog"

	"github.com/google/uuid"
	"github.com/nextlevelbuilder/goclaw/internal/ardenn"
	"github.com/nextlevelbuilder/goclaw/internal/ardenn/hands"
	"github.com/nextlevelbuilder/goclaw/internal/bus"
	"github.com/nextlevelbuilder/goclaw/internal/gateway"
	"github.com/nextlevelbuilder/goclaw/internal/gateway/methods"
	"github.com/nextlevelbuilder/goclaw/internal/store"
	pg "github.com/nextlevelbuilder/goclaw/internal/store/pg"
	pgardenn "github.com/nextlevelbuilder/goclaw/internal/store/pg/ardenn"
	"github.com/nextlevelbuilder/goclaw/internal/tools"
)

// Package-level Ardenn instances — set during gateway startup, read by ConsumerDeps.
var (
	pkgArdennEngine     *ardenn.Engine
	pkgArdennCompletion *hands.CompletionRegistry
)

// ardennProfileResolverAdapter bridges *pg.PGProfileResolver (returns *store.UserProfile)
// to ardenn.ProfileResolver (returns *ardenn.ResolvedUserProfile).
// Keeps internal/ardenn free of internal/store dependencies.
type ardennProfileResolverAdapter struct {
	inner *pg.PGProfileResolver
}

func (a *ardennProfileResolverAdapter) ResolveByUserID(ctx context.Context, userID uuid.UUID) (*ardenn.ResolvedUserProfile, error) {
	p, err := a.inner.ResolveByUserID(ctx, userID)
	if err != nil || p == nil {
		return nil, err
	}
	return &ardenn.ResolvedUserProfile{
		ID:          p.ID,
		Permissions: p.Permissions,
	}, nil
}

func (a *ardennProfileResolverAdapter) IncrementWorkload(ctx context.Context, userID uuid.UUID) error {
	return a.inner.IncrementWorkload(ctx, userID)
}

func (a *ardennProfileResolverAdapter) DecrementWorkload(ctx context.Context, userID uuid.UUID) error {
	return a.inner.DecrementWorkload(ctx, userID)
}

// initArdenn wires the Ardenn workflow engine from PG stores.
// resolver is optional (nil-safe): when provided, Engine.StartRun resolves
// the triggering user's permissions from org_users for guard evaluation.
// Returns nil, nil when Ardenn stores are not available (e.g., SQLite build).
func initArdenn(stores *store.Stores, msgBus *bus.MessageBus, resolver *pg.PGProfileResolver) (*ardenn.Engine, *hands.CompletionRegistry) {
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

	var opts ardenn.EngineOptions
	if resolver != nil {
		opts.ProfileResolver = &ardennProfileResolverAdapter{inner: resolver}
	}

	engine := ardenn.NewEngine(eventStore, registry, opts)

	slog.Info("ardenn: engine initialized",
		"event_store", "pg",
		"hands", []string{"agent"},
		"profile_resolver", resolver != nil,
	)

	// Startup recovery: wake any runs that were in-flight when the pod last restarted.
	// Runs without registered step defs (lost on restart) are re-loaded from DB.
	if defStore, ok := stores.ArdennDefinitions.(*pgardenn.PGDefinitionStore); ok {
		go recoverInFlightRuns(eventStore, defStore, engine)
	}

	return engine, completion
}

// recoverInFlightRuns scans for runs that didn't reach a terminal state before
// the last pod restart, reloads their step definitions, and calls Wake to resume.
// Called once at startup in a goroutine — errors are logged but not fatal.
func recoverInFlightRuns(
	eventStore *pgardenn.PGEventStore,
	defStore *pgardenn.PGDefinitionStore,
	engine *ardenn.Engine,
) {
	ctx := context.Background()
	runs, err := eventStore.GetInFlightRuns(ctx)
	if err != nil {
		slog.Warn("ardenn: startup recovery query failed", "error", err)
		return
	}
	if len(runs) == 0 {
		slog.Debug("ardenn: no in-flight runs to recover")
		return
	}

	slog.Info("ardenn: recovering in-flight runs", "count", len(runs))
	for _, r := range runs {
		steps, err := defStore.GetSteps(ctx, r.WorkflowID)
		if err != nil {
			slog.Warn("ardenn: recovery failed to load steps",
				"run_id", r.RunID, "workflow_id", r.WorkflowID, "error", err)
			continue
		}
		defs := pgardenn.ToStepDefs(steps)
		engine.RegisterStepDefs(r.RunID, defs)
		if err := engine.Wake(ctx, r.RunID); err != nil {
			slog.Warn("ardenn: recovery wake failed",
				"run_id", r.RunID, "error", err)
		} else {
			slog.Info("ardenn: recovered run", "run_id", r.RunID)
		}
	}
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
