-- migrations/000050_project_resources.up.sql

-- Generic resource binding for projects
CREATE TABLE IF NOT EXISTS project_resources (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id      UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    tenant_id       UUID NOT NULL REFERENCES tenants(id),
    resource_type   VARCHAR(50) NOT NULL,  -- 'mcp_server', 'git_repo', 'ci_pipeline', 'jira_project', 'confluence_space'
    resource_key    VARCHAR(255) NOT NULL, -- unique key within project+type (e.g. server name, repo slug)
    config          JSONB NOT NULL DEFAULT '{}',
    credentials_ref VARCHAR(255),          -- reference to config_secrets for sensitive values
    enabled         BOOLEAN NOT NULL DEFAULT true,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(project_id, resource_type, resource_key)
);

CREATE INDEX idx_project_resources_project ON project_resources(project_id);
CREATE INDEX idx_project_resources_tenant ON project_resources(tenant_id);
CREATE INDEX idx_project_resources_type ON project_resources(resource_type);

-- Domain resource templates: when creating a project in domain X, auto-suggest these resources
CREATE TABLE IF NOT EXISTS ardenn_domain_resource_templates (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    domain_id       UUID NOT NULL REFERENCES ardenn_domains(id) ON DELETE CASCADE,
    tenant_id       UUID NOT NULL REFERENCES tenants(id),
    resource_type   VARCHAR(50) NOT NULL,
    resource_key    VARCHAR(255) NOT NULL,
    display_name    VARCHAR(255) NOT NULL,
    description     TEXT,
    default_config  JSONB NOT NULL DEFAULT '{}',
    required        BOOLEAN NOT NULL DEFAULT false,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(domain_id, resource_type, resource_key)
);

CREATE INDEX idx_domain_resource_templates_domain ON ardenn_domain_resource_templates(domain_id);

-- Migrate existing project_mcp_overrides data into project_resources
INSERT INTO project_resources (project_id, tenant_id, resource_type, resource_key, config)
SELECT
    pmo.project_id,
    p.tenant_id,
    'mcp_server',
    pmo.server_name,
    jsonb_build_object(
        'server_name', pmo.server_name,
        'env_overrides', pmo.env_overrides
    )
FROM project_mcp_overrides pmo
JOIN projects p ON p.id = pmo.project_id
ON CONFLICT (project_id, resource_type, resource_key) DO NOTHING;

-- Create the backward compat view (moved from 000049 to here where the table exists)
CREATE OR REPLACE VIEW project_mcp_overrides_compat AS
SELECT
    pr.project_id,
    pr.config->>'server_name' AS server_name,
    pr.config->'env_overrides' AS env_overrides
FROM project_resources pr
WHERE pr.resource_type = 'mcp_server'
  AND pr.config->>'server_name' IS NOT NULL;
