package ardenn

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// ProjectResource represents a resource bound to a project.
type ProjectResource struct {
	ID             uuid.UUID       `db:"id"              json:"id"`
	ProjectID      uuid.UUID       `db:"project_id"      json:"project_id"`
	TenantID       uuid.UUID       `db:"tenant_id"       json:"tenant_id"`
	ResourceType   string          `db:"resource_type"    json:"resource_type"`
	ResourceKey    string          `db:"resource_key"     json:"resource_key"`
	Config         json.RawMessage `db:"config"           json:"config"`
	CredentialsRef *string         `db:"credentials_ref"  json:"credentials_ref,omitempty"`
	Enabled        bool            `db:"enabled"          json:"enabled"`
	CreatedAt      time.Time       `db:"created_at"       json:"created_at"`
	UpdatedAt      time.Time       `db:"updated_at"       json:"updated_at"`
}

// DomainResourceTemplate represents a default resource for a domain.
type DomainResourceTemplate struct {
	ID            uuid.UUID       `db:"id"              json:"id"`
	DomainID      uuid.UUID       `db:"domain_id"       json:"domain_id"`
	TenantID      uuid.UUID       `db:"tenant_id"       json:"tenant_id"`
	ResourceType  string          `db:"resource_type"    json:"resource_type"`
	ResourceKey   string          `db:"resource_key"     json:"resource_key"`
	DisplayName   string          `db:"display_name"     json:"display_name"`
	Description   *string         `db:"description"      json:"description,omitempty"`
	DefaultConfig json.RawMessage `db:"default_config"   json:"default_config"`
	Required      bool            `db:"required"         json:"required"`
	CreatedAt     time.Time       `db:"created_at"       json:"created_at"`
}

// ProjectResourceStore manages project resources and domain templates.
type ProjectResourceStore struct {
	db *sqlx.DB
}

// NewProjectResourceStore creates a store backed by the given DB.
func NewProjectResourceStore(db *sqlx.DB) *ProjectResourceStore {
	return &ProjectResourceStore{db: db}
}

// GetByProject returns all enabled resources for a project.
func (s *ProjectResourceStore) GetByProject(ctx context.Context, projectID uuid.UUID) ([]ProjectResource, error) {
	var resources []ProjectResource
	err := s.db.SelectContext(ctx, &resources,
		`SELECT id, project_id, tenant_id, resource_type, resource_key, config,
		        credentials_ref, enabled, created_at, updated_at
		 FROM project_resources
		 WHERE project_id = $1 AND enabled = true
		 ORDER BY resource_type, resource_key`,
		projectID,
	)
	if err != nil {
		return nil, fmt.Errorf("get project resources: %w", err)
	}
	return resources, nil
}

// GetByProjectAndType returns resources of a specific type for a project.
func (s *ProjectResourceStore) GetByProjectAndType(ctx context.Context, projectID uuid.UUID, resourceType string) ([]ProjectResource, error) {
	var resources []ProjectResource
	err := s.db.SelectContext(ctx, &resources,
		`SELECT id, project_id, tenant_id, resource_type, resource_key, config,
		        credentials_ref, enabled, created_at, updated_at
		 FROM project_resources
		 WHERE project_id = $1 AND resource_type = $2 AND enabled = true
		 ORDER BY resource_key`,
		projectID, resourceType,
	)
	if err != nil {
		return nil, fmt.Errorf("get project resources by type: %w", err)
	}
	return resources, nil
}

// CreateResource inserts a new project resource (upsert on conflict).
func (s *ProjectResourceStore) CreateResource(ctx context.Context, r ProjectResource) (*ProjectResource, error) {
	if r.ID == uuid.Nil {
		r.ID = uuid.New()
	}
	err := s.db.QueryRowxContext(ctx,
		`INSERT INTO project_resources (id, project_id, tenant_id, resource_type, resource_key, config, credentials_ref, enabled)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 ON CONFLICT (project_id, resource_type, resource_key)
		 DO UPDATE SET config = EXCLUDED.config,
		               credentials_ref = EXCLUDED.credentials_ref,
		               enabled = EXCLUDED.enabled,
		               updated_at = NOW()
		 RETURNING id, project_id, tenant_id, resource_type, resource_key, config,
		           credentials_ref, enabled, created_at, updated_at`,
		r.ID, r.ProjectID, r.TenantID, r.ResourceType, r.ResourceKey,
		r.Config, r.CredentialsRef, r.Enabled,
	).StructScan(&r)
	if err != nil {
		return nil, fmt.Errorf("create project resource: %w", err)
	}
	return &r, nil
}

// DeleteResource removes a project resource by ID.
func (s *ProjectResourceStore) DeleteResource(ctx context.Context, resourceID uuid.UUID) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM project_resources WHERE id = $1`,
		resourceID,
	)
	if err != nil {
		return fmt.Errorf("delete project resource: %w", err)
	}
	return nil
}

// DisableResource soft-disables a resource (sets enabled=false).
func (s *ProjectResourceStore) DisableResource(ctx context.Context, resourceID uuid.UUID) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE project_resources SET enabled = false, updated_at = NOW() WHERE id = $1`,
		resourceID,
	)
	if err != nil {
		return fmt.Errorf("disable project resource: %w", err)
	}
	return nil
}

// GetDomainTemplates returns all resource templates for a domain.
func (s *ProjectResourceStore) GetDomainTemplates(ctx context.Context, domainID uuid.UUID) ([]DomainResourceTemplate, error) {
	var templates []DomainResourceTemplate
	err := s.db.SelectContext(ctx, &templates,
		`SELECT id, domain_id, tenant_id, resource_type, resource_key, display_name,
		        description, default_config, required, created_at
		 FROM ardenn_domain_resource_templates
		 WHERE domain_id = $1
		 ORDER BY required DESC, resource_type, resource_key`,
		domainID,
	)
	if err != nil {
		return nil, fmt.Errorf("get domain templates: %w", err)
	}
	return templates, nil
}

// CreateDomainTemplate inserts a domain resource template.
func (s *ProjectResourceStore) CreateDomainTemplate(ctx context.Context, t DomainResourceTemplate) (*DomainResourceTemplate, error) {
	if t.ID == uuid.Nil {
		t.ID = uuid.New()
	}
	err := s.db.QueryRowxContext(ctx,
		`INSERT INTO ardenn_domain_resource_templates
		     (id, domain_id, tenant_id, resource_type, resource_key, display_name, description, default_config, required)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		 ON CONFLICT (domain_id, resource_type, resource_key) DO UPDATE SET
		     display_name = EXCLUDED.display_name,
		     description = EXCLUDED.description,
		     default_config = EXCLUDED.default_config,
		     required = EXCLUDED.required
		 RETURNING id, domain_id, tenant_id, resource_type, resource_key, display_name,
		           description, default_config, required, created_at`,
		t.ID, t.DomainID, t.TenantID, t.ResourceType, t.ResourceKey,
		t.DisplayName, t.Description, t.DefaultConfig, t.Required,
	).StructScan(&t)
	if err != nil {
		return nil, fmt.Errorf("create domain template: %w", err)
	}
	return &t, nil
}

// ApplyDomainTemplates creates project resources from domain templates for a project.
// Only creates resources that don't already exist for the project.
func (s *ProjectResourceStore) ApplyDomainTemplates(ctx context.Context, projectID, domainID, tenantID uuid.UUID) ([]ProjectResource, error) {
	templates, err := s.GetDomainTemplates(ctx, domainID)
	if err != nil {
		return nil, err
	}

	var created []ProjectResource
	for _, tmpl := range templates {
		r := ProjectResource{
			ProjectID:    projectID,
			TenantID:     tenantID,
			ResourceType: tmpl.ResourceType,
			ResourceKey:  tmpl.ResourceKey,
			Config:       tmpl.DefaultConfig,
			Enabled:      true,
		}
		res, err := s.CreateResource(ctx, r)
		if err != nil {
			return nil, fmt.Errorf("apply template %s/%s: %w", tmpl.ResourceType, tmpl.ResourceKey, err)
		}
		created = append(created, *res)
	}
	return created, nil
}
