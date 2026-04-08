CREATE TABLE IF NOT EXISTS harness_handoff_artifacts (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL,
    agent_id        UUID NOT NULL,
    user_id         TEXT NOT NULL,
    session_id      UUID NOT NULL,
    pipeline_id     UUID,
    sequence        INT NOT NULL,
    objective       TEXT NOT NULL,
    progress        JSONB NOT NULL,
    decisions       JSONB NOT NULL DEFAULT '[]',
    artifacts       JSONB NOT NULL DEFAULT '[]',
    open_questions  JSONB NOT NULL DEFAULT '[]',
    git_branch      TEXT,
    git_commit      TEXT,
    strategy        TEXT NOT NULL,
    context_usage_pct INT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_hha_lookup
    ON harness_handoff_artifacts(tenant_id, agent_id, user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_hha_pipeline
    ON harness_handoff_artifacts(pipeline_id, sequence);
