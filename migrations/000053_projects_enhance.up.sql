-- migrations/000049_projects_enhance.up.sql

-- Add columns to projects table
ALTER TABLE projects
    ADD COLUMN IF NOT EXISTS tenant_id UUID REFERENCES tenants(id),
    ADD COLUMN IF NOT EXISTS domain_id UUID REFERENCES ardenn_domains(id),
    ADD COLUMN IF NOT EXISTS created_by_user UUID,
    ADD COLUMN IF NOT EXISTS settings JSONB NOT NULL DEFAULT '{}';

-- Backfill existing projects with master tenant
UPDATE projects
SET tenant_id = '0193a5b0-7000-7000-8000-000000000001'::uuid
WHERE tenant_id IS NULL;

-- Make tenant_id NOT NULL after backfill
ALTER TABLE projects ALTER COLUMN tenant_id SET NOT NULL;

-- Indexes for new columns
CREATE INDEX IF NOT EXISTS idx_projects_tenant ON projects(tenant_id);
CREATE INDEX IF NOT EXISTS idx_projects_domain ON projects(domain_id);
CREATE INDEX IF NOT EXISTS idx_projects_tenant_domain ON projects(tenant_id, domain_id);

-- NOTE: Backward compat view (project_mcp_overrides_compat) is created in
-- migration 000050 after the project_resources table exists.
