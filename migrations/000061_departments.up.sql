-- 000061: Departments + Department Members (Identity Phase 3)

CREATE TABLE departments (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL REFERENCES tenants(id),
    name            VARCHAR(255) NOT NULL,
    slug            VARCHAR(100) NOT NULL,
    parent_id       UUID REFERENCES departments(id),
    head_user_id    UUID REFERENCES org_users(id),
    description     TEXT,
    metadata        JSONB DEFAULT '{}',
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    updated_at      TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(tenant_id, slug)
);

CREATE INDEX idx_departments_tenant ON departments(tenant_id);
CREATE INDEX idx_departments_parent ON departments(parent_id) WHERE parent_id IS NOT NULL;

CREATE TABLE department_members (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    department_id   UUID NOT NULL REFERENCES departments(id) ON DELETE CASCADE,
    user_id         UUID NOT NULL REFERENCES org_users(id) ON DELETE CASCADE,
    role            VARCHAR(50) NOT NULL,
    title           VARCHAR(255),
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(department_id, user_id)
);

CREATE INDEX idx_department_members_user ON department_members(user_id);
CREATE INDEX idx_department_members_dept ON department_members(department_id);
