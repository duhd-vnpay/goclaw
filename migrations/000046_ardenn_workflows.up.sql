CREATE TABLE IF NOT EXISTS ardenn_workflows (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL REFERENCES tenants(id),
    domain_id       UUID NOT NULL REFERENCES ardenn_domains(id),
    slug            VARCHAR(100) NOT NULL,
    name            VARCHAR(255) NOT NULL,
    description     TEXT,
    version         INT NOT NULL DEFAULT 1,
    tier            VARCHAR(20) NOT NULL,
    trigger_config  JSONB NOT NULL DEFAULT '{}',
    variables       JSONB NOT NULL DEFAULT '{}',
    settings        JSONB NOT NULL DEFAULT '{}',
    visibility      VARCHAR(20) NOT NULL DEFAULT 'domain',
    status          VARCHAR(20) NOT NULL DEFAULT 'draft',
    created_by      UUID,
    published_at    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(tenant_id, slug, version)
);

CREATE INDEX idx_ardenn_workflows_domain ON ardenn_workflows(domain_id);
CREATE INDEX idx_ardenn_workflows_tenant_status ON ardenn_workflows(tenant_id, status);

CREATE TABLE IF NOT EXISTS ardenn_steps (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workflow_id     UUID NOT NULL REFERENCES ardenn_workflows(id) ON DELETE CASCADE,
    slug            VARCHAR(100) NOT NULL,
    name            VARCHAR(255) NOT NULL,
    description     TEXT,
    position        INT NOT NULL,
    agent_key       VARCHAR(255),
    task_template   TEXT,
    depends_on      UUID[],
    condition       TEXT,
    timeout         INTERVAL NOT NULL DEFAULT '30 minutes',
    constraints     JSONB NOT NULL DEFAULT '{}',
    continuity      JSONB NOT NULL DEFAULT '{}',
    evaluation      JSONB NOT NULL DEFAULT '{}',
    gate            JSONB NOT NULL DEFAULT '{}',
    dispatch_to     VARCHAR(50),
    dispatch_target VARCHAR(255),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(workflow_id, slug)
);

CREATE INDEX idx_ardenn_steps_workflow ON ardenn_steps(workflow_id, position);
