package harness

import (
	"database/sql"

	"github.com/nextlevelbuilder/goclaw/internal/harness/constraints"
	"github.com/nextlevelbuilder/goclaw/internal/harness/continuity"
	"github.com/nextlevelbuilder/goclaw/internal/harness/orchestrator"
)

// Manager wires all harness layers together.
type Manager struct {
	config     Config
	guards     *constraints.GuardRegistry
	deps       *constraints.DependencyEngine
	violations *constraints.ViolationStore
	artifacts  *continuity.ArtifactStore
	strategy   *continuity.StrategyResolver
	toolHandler *continuity.ToolHandler
	workflows  *orchestrator.WorkflowLoader
	events     *orchestrator.EventBus
	gate       *orchestrator.GateKeeper
	state      *orchestrator.StateManager
}

// NewManager creates a fully wired harness manager.
func NewManager(cfg Config, db *sql.DB) *Manager {
	artifactStore := continuity.NewArtifactStore(db)
	strategyResolver := continuity.NewStrategyResolver(
		cfg.Continuity.Strategy,
		continuity.AdaptiveStrategyConfig{
			ContextUsagePct: cfg.Continuity.Adaptive.ContextUsagePct,
			MessageCount:    cfg.Continuity.Adaptive.MessageCount,
			TaskBoundary:    cfg.Continuity.Adaptive.TaskBoundary,
		},
	)
	events := orchestrator.NewEventBus()

	m := &Manager{
		config:      cfg,
		guards:      constraints.NewGuardRegistry(),
		violations:  constraints.NewViolationStore(db),
		artifacts:   artifactStore,
		strategy:    strategyResolver,
		toolHandler: continuity.NewToolHandler(artifactStore, strategyResolver),
		events:      events,
		gate:        orchestrator.NewGateKeeper(events),
		state:       orchestrator.NewStateManager(),
	}

	if cfg.DependencyLayers.Enforcement != "off" {
		m.deps = constraints.NewDependencyEngine(cfg.DependencyLayers)
	}

	if cfg.Orchestrator.Enabled && cfg.Orchestrator.WorkflowDir != "" {
		m.workflows = orchestrator.NewWorkflowLoader(cfg.Orchestrator.WorkflowDir)
	}

	return m
}

// Enabled returns true if the harness layer is active.
func (m *Manager) Enabled() bool { return m.config.Enabled }

// Guards returns the L1 guard registry.
func (m *Manager) Guards() *constraints.GuardRegistry { return m.guards }

// Dependencies returns the L1 dependency engine (may be nil if disabled).
func (m *Manager) Dependencies() *constraints.DependencyEngine { return m.deps }

// Artifacts returns the L2 artifact store.
func (m *Manager) Artifacts() *continuity.ArtifactStore { return m.artifacts }

// Strategy returns the L2 context strategy resolver.
func (m *Manager) Strategy() *continuity.StrategyResolver { return m.strategy }

// ToolHandler returns the L2 continuity tool handler.
func (m *Manager) ToolHandler() *continuity.ToolHandler { return m.toolHandler }

// Events returns the L4 event bus.
func (m *Manager) Events() *orchestrator.EventBus { return m.events }

// Gate returns the L4 gate keeper.
func (m *Manager) Gate() *orchestrator.GateKeeper { return m.gate }

// Workflows returns the L4 workflow loader (may be nil if disabled).
func (m *Manager) Workflows() *orchestrator.WorkflowLoader { return m.workflows }

// StateManager returns the L4 workflow state manager.
func (m *Manager) StateManager() *orchestrator.StateManager { return m.state }

// Config returns the harness configuration.
func (m *Manager) Config() Config { return m.config }
