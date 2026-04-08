package harness

import (
	"database/sql"
	"github.com/nextlevelbuilder/goclaw/internal/harness/constraints"
)

type Manager struct {
	config     Config
	guards     *constraints.GuardRegistry
	deps       *constraints.DependencyEngine
	violations *constraints.ViolationStore
}

func NewManager(cfg Config, db *sql.DB) *Manager {
	m := &Manager{
		config:     cfg,
		guards:     constraints.NewGuardRegistry(),
		violations: constraints.NewViolationStore(db),
	}
	if cfg.DependencyLayers.Enforcement != "off" {
		m.deps = constraints.NewDependencyEngine(cfg.DependencyLayers)
	}
	return m
}

func (m *Manager) Enabled() bool                               { return m.config.Enabled }
func (m *Manager) Guards() *constraints.GuardRegistry           { return m.guards }
func (m *Manager) Dependencies() *constraints.DependencyEngine  { return m.deps }
func (m *Manager) Config() Config                              { return m.config }
