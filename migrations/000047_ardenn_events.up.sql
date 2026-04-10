CREATE TABLE IF NOT EXISTS ardenn_events (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL REFERENCES tenants(id),
    run_id          UUID NOT NULL,
    step_id         UUID,
    sequence        BIGSERIAL,
    event_type      VARCHAR(100) NOT NULL,
    actor_type      VARCHAR(20) NOT NULL,
    actor_id        UUID,
    payload         JSONB NOT NULL DEFAULT '{}',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_ardenn_events_run_seq ON ardenn_events(run_id, sequence);
CREATE INDEX idx_ardenn_events_type ON ardenn_events(tenant_id, event_type, created_at DESC);
CREATE INDEX idx_ardenn_events_created ON ardenn_events(created_at);
