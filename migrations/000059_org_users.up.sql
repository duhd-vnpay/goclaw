-- migrations/000059_org_users.up.sql
-- Org identity: thin cache of Keycloak users.
-- id = keycloak user UUID (not auto-generated).

CREATE TABLE IF NOT EXISTS org_users (
    id              UUID PRIMARY KEY,
    tenant_id       UUID NOT NULL REFERENCES tenants(id),
    email           VARCHAR(255) NOT NULL,
    display_name    VARCHAR(255),
    avatar_url      TEXT,
    auth_provider   VARCHAR(50),
    profile         JSONB NOT NULL DEFAULT '{}',
    status          VARCHAR(20) NOT NULL DEFAULT 'active',
    last_login_at   TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_org_users_tenant_email UNIQUE (tenant_id, email)
);

-- Index for tenant-scoped queries
CREATE INDEX IF NOT EXISTS idx_org_users_tenant_id ON org_users(tenant_id);
-- Index for email lookup (cross-tenant admin)
CREATE INDEX IF NOT EXISTS idx_org_users_email ON org_users(email);

-- Link tenant_users to org_users (nullable — existing rows have no keycloak mapping yet)
ALTER TABLE tenant_users ADD COLUMN IF NOT EXISTS keycloak_id UUID REFERENCES org_users(id);
CREATE INDEX IF NOT EXISTS idx_tenant_users_keycloak_id ON tenant_users(keycloak_id) WHERE keycloak_id IS NOT NULL;
