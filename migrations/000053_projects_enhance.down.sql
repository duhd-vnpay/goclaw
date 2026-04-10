-- migrations/000049_projects_enhance.down.sql

DROP INDEX IF EXISTS idx_projects_tenant_domain;
DROP INDEX IF EXISTS idx_projects_domain;
DROP INDEX IF EXISTS idx_projects_tenant;

ALTER TABLE projects
    DROP COLUMN IF EXISTS settings,
    DROP COLUMN IF EXISTS created_by_user,
    DROP COLUMN IF EXISTS domain_id,
    DROP COLUMN IF EXISTS tenant_id;
