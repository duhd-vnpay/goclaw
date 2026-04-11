package ardenn

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

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
	CreatedAt    time.Time       `db:"created_at"`
	UpdatedAt    time.Time       `db:"updated_at"`
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
	PublishedAt   *time.Time      `db:"published_at"`
	CreatedAt     time.Time       `db:"created_at"`
	UpdatedAt     time.Time       `db:"updated_at"`
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

// --- CRUD methods for gateway RPC ---

type CreateDomainParams struct {
	Slug         string
	Name         string
	Description  string
	DepartmentID *uuid.UUID
	DefaultTier  string
}

func (s *PGDefinitionStore) CreateDomain(ctx context.Context, tenantID uuid.UUID, p CreateDomainParams) (*Domain, error) {
	var d Domain
	err := s.db.GetContext(ctx, &d,
		`INSERT INTO ardenn_domains (tenant_id, slug, name, description, department_id, default_tier)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING *`,
		tenantID, p.Slug, p.Name, p.Description, p.DepartmentID, p.DefaultTier)
	if err != nil {
		return nil, fmt.Errorf("create domain: %w", err)
	}
	return &d, nil
}

type UpdateDomainParams struct {
	Name        *string
	Description *string
	DefaultTier *string
}

func (s *PGDefinitionStore) UpdateDomain(ctx context.Context, tenantID uuid.UUID, id uuid.UUID, p UpdateDomainParams) (*Domain, error) {
	var d Domain
	err := s.db.GetContext(ctx, &d,
		`UPDATE ardenn_domains SET
		   name = COALESCE($3, name),
		   description = COALESCE($4, description),
		   default_tier = COALESCE($5, default_tier),
		   updated_at = NOW()
		 WHERE id = $1 AND tenant_id = $2
		 RETURNING *`,
		id, tenantID, p.Name, p.Description, p.DefaultTier)
	if err != nil {
		return nil, fmt.Errorf("update domain: %w", err)
	}
	return &d, nil
}

func (s *PGDefinitionStore) DeleteDomain(ctx context.Context, tenantID uuid.UUID, id uuid.UUID) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM ardenn_domains WHERE id = $1 AND tenant_id = $2`, id, tenantID)
	return err
}

type ListWorkflowsFilter struct {
	DomainID *uuid.UUID
	Status   *string
}

func (s *PGDefinitionStore) ListWorkflows(ctx context.Context, tenantID uuid.UUID, f ListWorkflowsFilter) ([]Workflow, error) {
	query := `SELECT * FROM ardenn_workflows WHERE tenant_id = $1`
	args := []any{tenantID}
	idx := 2

	if f.DomainID != nil {
		query += fmt.Sprintf(" AND domain_id = $%d", idx)
		args = append(args, *f.DomainID)
		idx++
	}
	if f.Status != nil {
		query += fmt.Sprintf(" AND status = $%d", idx)
		args = append(args, *f.Status)
		idx++
	}
	query += " ORDER BY updated_at DESC"

	var workflows []Workflow
	err := s.db.SelectContext(ctx, &workflows, query, args...)
	return workflows, err
}

func (s *PGDefinitionStore) GetWorkflowByID(ctx context.Context, id uuid.UUID) (*Workflow, error) {
	var w Workflow
	err := s.db.GetContext(ctx, &w, `SELECT * FROM ardenn_workflows WHERE id = $1`, id)
	if err != nil {
		return nil, fmt.Errorf("get workflow by id: %w", err)
	}
	return &w, nil
}

type CreateWorkflowParams struct {
	TenantID      uuid.UUID
	DomainID      uuid.UUID
	Slug          string
	Name          string
	Description   string
	Tier          string
	TriggerConfig json.RawMessage
	Variables     json.RawMessage
	Settings      json.RawMessage
	Visibility    string
	CreatedBy     string
}

func (s *PGDefinitionStore) CreateWorkflow(ctx context.Context, p CreateWorkflowParams) (*Workflow, error) {
	var createdBy *uuid.UUID
	if p.CreatedBy != "" {
		if uid, err := uuid.Parse(p.CreatedBy); err == nil {
			createdBy = &uid
		}
	}
	var w Workflow
	err := s.db.GetContext(ctx, &w,
		`INSERT INTO ardenn_workflows
		   (tenant_id, domain_id, slug, name, description, tier, trigger_config, variables, settings, visibility, created_by)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		 RETURNING *`,
		p.TenantID, p.DomainID, p.Slug, p.Name, p.Description, p.Tier,
		p.TriggerConfig, p.Variables, p.Settings, p.Visibility, createdBy)
	if err != nil {
		return nil, fmt.Errorf("create workflow: %w", err)
	}
	return &w, nil
}

type UpdateWorkflowParams struct {
	Name          *string
	Description   *string
	Tier          *string
	TriggerConfig *json.RawMessage
	Variables     *json.RawMessage
	Settings      *json.RawMessage
	Visibility    *string
}

func (s *PGDefinitionStore) UpdateWorkflow(ctx context.Context, id uuid.UUID, p UpdateWorkflowParams) (*Workflow, error) {
	var w Workflow
	err := s.db.GetContext(ctx, &w,
		`UPDATE ardenn_workflows SET
		   name = COALESCE($2, name),
		   description = COALESCE($3, description),
		   tier = COALESCE($4, tier),
		   trigger_config = COALESCE($5, trigger_config),
		   variables = COALESCE($6, variables),
		   settings = COALESCE($7, settings),
		   visibility = COALESCE($8, visibility),
		   updated_at = NOW()
		 WHERE id = $1
		 RETURNING *`,
		id, p.Name, p.Description, p.Tier, p.TriggerConfig, p.Variables, p.Settings, p.Visibility)
	if err != nil {
		return nil, fmt.Errorf("update workflow: %w", err)
	}
	return &w, nil
}

func (s *PGDefinitionStore) PublishWorkflow(ctx context.Context, id uuid.UUID) (*Workflow, error) {
	var w Workflow
	err := s.db.GetContext(ctx, &w,
		`UPDATE ardenn_workflows SET status = 'published', published_at = NOW(), updated_at = NOW()
		 WHERE id = $1 RETURNING *`, id)
	if err != nil {
		return nil, fmt.Errorf("publish workflow: %w", err)
	}
	return &w, nil
}

func (s *PGDefinitionStore) DeleteWorkflow(ctx context.Context, id uuid.UUID) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM ardenn_workflows WHERE id = $1`, id)
	return err
}

type CreateStepParams struct {
	WorkflowID     uuid.UUID
	Slug           string
	Name           string
	Description    string
	Position       int
	AgentKey       string
	TaskTemplate   string
	DependsOn      []uuid.UUID
	Condition      string
	Timeout        string
	DispatchTo     string
	DispatchTarget string
	Gate           json.RawMessage
	Constraints    json.RawMessage
	Continuity     json.RawMessage
	Evaluation     json.RawMessage
}

func (s *PGDefinitionStore) CreateStep(ctx context.Context, p CreateStepParams) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO ardenn_steps
		   (workflow_id, slug, name, description, position, agent_key, task_template,
		    depends_on, condition, timeout, dispatch_to, dispatch_target,
		    gate, constraints, continuity, evaluation)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10::interval, $11, $12, $13, $14, $15, $16)`,
		p.WorkflowID, p.Slug, p.Name, p.Description, p.Position,
		nilStr(p.AgentKey), nilStr(p.TaskTemplate),
		p.DependsOn, nilStr(p.Condition), p.Timeout,
		nilStr(p.DispatchTo), nilStr(p.DispatchTarget),
		p.Gate, p.Constraints, p.Continuity, p.Evaluation)
	return err
}

func (s *PGDefinitionStore) DeleteSteps(ctx context.Context, workflowID uuid.UUID) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM ardenn_steps WHERE workflow_id = $1`, workflowID)
	return err
}

func nilStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
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
