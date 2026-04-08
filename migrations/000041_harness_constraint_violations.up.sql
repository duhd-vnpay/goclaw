CREATE TABLE IF NOT EXISTS harness_constraint_violations (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL,
    agent_id    UUID NOT NULL,
    session_id  UUID,
    guard_name  TEXT NOT NULL,
    phase       TEXT NOT NULL,
    kind        TEXT NOT NULL,
    action      TEXT NOT NULL,
    feedback    TEXT,
    context     JSONB,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_hcv_agent_time
    ON harness_constraint_violations(tenant_id, agent_id, created_at);
CREATE INDEX IF NOT EXISTS idx_hcv_guard
    ON harness_constraint_violations(guard_name, action, created_at);
