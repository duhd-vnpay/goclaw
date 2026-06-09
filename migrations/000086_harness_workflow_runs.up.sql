CREATE TABLE IF NOT EXISTS harness_workflow_runs (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id    UUID NOT NULL,
    workflow_id  TEXT NOT NULL,
    workflow_ver TEXT NOT NULL,
    status       TEXT NOT NULL DEFAULT 'pending',
    variables    JSONB DEFAULT '{}',
    input        JSONB DEFAULT '{}',
    output       JSONB,
    triggered_by TEXT,
    started_at   TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS harness_workflow_steps (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id       UUID NOT NULL REFERENCES harness_workflow_runs(id) ON DELETE CASCADE,
    step_id      TEXT NOT NULL,
    agent_id     UUID,
    session_id   UUID,
    status       TEXT NOT NULL DEFAULT 'pending',
    attempt      INT DEFAULT 1,
    result       JSONB,
    eval_result  JSONB,
    gate_result  JSONB,
    error        TEXT,
    artifact_id  UUID,
    started_at   TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_hwr_tenant ON harness_workflow_runs(tenant_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_hws_run ON harness_workflow_steps(run_id, step_id);
