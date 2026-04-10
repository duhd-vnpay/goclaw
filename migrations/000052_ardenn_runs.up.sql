CREATE TABLE IF NOT EXISTS ardenn_runs (
    id              UUID PRIMARY KEY,
    tenant_id       UUID NOT NULL REFERENCES tenants(id),
    workflow_id     UUID NOT NULL REFERENCES ardenn_workflows(id),
    project_id      UUID,
    triggered_by    UUID,
    variables       JSONB NOT NULL DEFAULT '{}',
    tier            VARCHAR(20) NOT NULL,
    status          VARCHAR(20) NOT NULL DEFAULT 'pending',
    last_sequence   BIGINT NOT NULL DEFAULT 0,
    started_at      TIMESTAMPTZ,
    completed_at    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_ardenn_runs_tenant_status ON ardenn_runs(tenant_id, status);
CREATE INDEX idx_ardenn_runs_workflow ON ardenn_runs(workflow_id);

CREATE TABLE IF NOT EXISTS ardenn_step_runs (
    id              UUID PRIMARY KEY,
    run_id          UUID NOT NULL REFERENCES ardenn_runs(id) ON DELETE CASCADE,
    step_id         UUID NOT NULL REFERENCES ardenn_steps(id),
    status          VARCHAR(20) NOT NULL DEFAULT 'pending',
    assigned_user   UUID,
    assigned_agent  UUID,
    hand_type       VARCHAR(20),
    result          TEXT,
    dispatch_count  INT NOT NULL DEFAULT 0,
    eval_round      INT NOT NULL DEFAULT 0,
    eval_score      FLOAT,
    eval_passed     BOOLEAN,
    gate_status     VARCHAR(20),
    gate_decided_by UUID,
    gate_decided_at TIMESTAMPTZ,
    metadata        JSONB NOT NULL DEFAULT '{}',
    last_sequence   BIGINT NOT NULL DEFAULT 0,
    started_at      TIMESTAMPTZ,
    completed_at    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(run_id, step_id)
);

CREATE INDEX idx_ardenn_step_runs_run ON ardenn_step_runs(run_id);
CREATE INDEX idx_ardenn_step_runs_status ON ardenn_step_runs(status) WHERE status IN ('running', 'waiting_gate', 'blocked');

CREATE TABLE IF NOT EXISTS ardenn_artifacts (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    step_run_id     UUID NOT NULL REFERENCES ardenn_step_runs(id),
    tenant_id       UUID NOT NULL REFERENCES tenants(id),
    agent_id        UUID,
    user_id         UUID,
    artifact_type   VARCHAR(50) NOT NULL,
    content         JSONB NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_ardenn_artifacts_step_run ON ardenn_artifacts(step_run_id);
