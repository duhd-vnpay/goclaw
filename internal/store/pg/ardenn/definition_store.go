package ardenn

import (
	"context"
	"encoding/json"
	"fmt"

	engine "github.com/nextlevelbuilder/goclaw/internal/ardenn"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

type PGDefinitionStore struct {
	db *sqlx.DB
}

func NewPGDefinitionStore(db *sqlx.DB) *PGDefinitionStore {
	return &PGDefinitionStore{db: db}
}

type Domain struct {
	ID           uuid.UUID       `db:"id"`
	TenantID     uuid.UUID       `db:"tenant_id"`
	Slug         string          `db:"slug"`
	Name         string          `db:"name"`
	Description  *string         `db:"description"`
	DepartmentID *uuid.UUID      `db:"department_id"`
	DefaultTier  string          `db:"default_tier"`
	Settings     json.RawMessage `db:"settings"`
}

type Workflow struct {
	ID            uuid.UUID       `db:"id"`
	TenantID      uuid.UUID       `db:"tenant_id"`
	DomainID      uuid.UUID       `db:"domain_id"`
	Slug          string          `db:"slug"`
	Name          string          `db:"name"`
	Description   *string         `db:"description"`
	Version       int             `db:"version"`
	Tier          string          `db:"tier"`
	TriggerConfig json.RawMessage `db:"trigger_config"`
	Variables     json.RawMessage `db:"variables"`
	Settings      json.RawMessage `db:"settings"`
	Visibility    string          `db:"visibility"`
	Status        string          `db:"status"`
	CreatedBy     *uuid.UUID      `db:"created_by"`
}

type Step struct {
	ID             uuid.UUID       `db:"id"`
	WorkflowID     uuid.UUID       `db:"workflow_id"`
	Slug           string          `db:"slug"`
	Name           string          `db:"name"`
	Description    *string         `db:"description"`
	Position       int             `db:"position"`
	AgentKey       *string         `db:"agent_key"`
	TaskTemplate   *string         `db:"task_template"`
	DependsOn      []uuid.UUID     `db:"depends_on"`
	Condition      *string         `db:"condition"`
	Timeout        string          `db:"timeout"`
	Constraints    json.RawMessage `db:"constraints"`
	Continuity     json.RawMessage `db:"continuity"`
	Evaluation     json.RawMessage `db:"evaluation"`
	Gate           json.RawMessage `db:"gate"`
	DispatchTo     *string         `db:"dispatch_to"`
	DispatchTarget *string         `db:"dispatch_target"`
}

func (s *PGDefinitionStore) GetDomain(ctx context.Context, tenantID uuid.UUID, slug string) (*Domain, error) {
	var d Domain
	err := s.db.GetContext(ctx, &d,
		`SELECT * FROM ardenn_domains WHERE tenant_id = $1 AND slug = $2`, tenantID, slug)
	if err != nil {
		return nil, fmt.Errorf("get domain: %w", err)
	}
	return &d, nil
}

func (s *PGDefinitionStore) ListDomains(ctx context.Context, tenantID uuid.UUID) ([]Domain, error) {
	var domains []Domain
	err := s.db.SelectContext(ctx, &domains,
		`SELECT * FROM ardenn_domains WHERE tenant_id = $1 ORDER BY name`, tenantID)
	return domains, err
}

func (s *PGDefinitionStore) GetPublishedWorkflow(ctx context.Context, tenantID uuid.UUID, slug string) (*Workflow, error) {
	var w Workflow
	err := s.db.GetContext(ctx, &w,
		`SELECT * FROM ardenn_workflows
		 WHERE tenant_id = $1 AND slug = $2 AND status = 'published'
		 ORDER BY version DESC LIMIT 1`, tenantID, slug)
	if err != nil {
		return nil, fmt.Errorf("get workflow: %w", err)
	}
	return &w, nil
}

func (s *PGDefinitionStore) GetSteps(ctx context.Context, workflowID uuid.UUID) ([]Step, error) {
	var steps []Step
	err := s.db.SelectContext(ctx, &steps,
		`SELECT * FROM ardenn_steps WHERE workflow_id = $1 ORDER BY position`, workflowID)
	return steps, err
}

func ToStepDefs(steps []Step) map[uuid.UUID]*engine.StepDef {
	defs := make(map[uuid.UUID]*engine.StepDef, len(steps))
	for _, s := range steps {
		def := &engine.StepDef{
			ID:         s.ID,
			WorkflowID: s.WorkflowID,
			Slug:       s.Slug,
			Name:       s.Name,
			Position:   s.Position,
			DependsOn:  s.DependsOn,
			Timeout:    s.Timeout,
		}
		if s.AgentKey != nil {
			def.AgentKey = *s.AgentKey
		}
		if s.TaskTemplate != nil {
			def.TaskTemplate = *s.TaskTemplate
		}
		if s.Description != nil {
			def.Description = *s.Description
		}
		if s.Condition != nil {
			def.Condition = *s.Condition
		}
		if s.DispatchTo != nil {
			def.DispatchTo = *s.DispatchTo
		}
		if s.DispatchTarget != nil {
			def.DispatchTarget = *s.DispatchTarget
		}
		if len(s.Gate) > 2 {
			var gc engine.GateConfig
			if json.Unmarshal(s.Gate, &gc) == nil && gc.Type != "" {
				def.Gate = &gc
			}
		}
		if len(s.Evaluation) > 2 {
			var ec engine.EvalConfig
			if json.Unmarshal(s.Evaluation, &ec) == nil {
				def.Evaluation = &ec
			}
		}
		defs[s.ID] = def
	}
	return defs
}
