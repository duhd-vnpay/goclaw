CREATE TABLE IF NOT EXISTS harness_evaluations (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL,
    agent_id        UUID NOT NULL,
    session_id      UUID NOT NULL,
    pipeline_id     UUID,
    checkpoint      TEXT,
    round           INT NOT NULL,
    track           TEXT NOT NULL,
    sensor_name     TEXT NOT NULL,
    pass            BOOLEAN NOT NULL,
    score           FLOAT,
    dimension_scores JSONB,
    issues          JSONB DEFAULT '[]',
    feedback        TEXT,
    duration_ms     INT NOT NULL,
    escalated       BOOLEAN DEFAULT FALSE,
    final_result    TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_he_pipeline ON harness_evaluations(pipeline_id, checkpoint, round);
CREATE INDEX IF NOT EXISTS idx_he_agent ON harness_evaluations(tenant_id, agent_id, created_at DESC);

CREATE OR REPLACE VIEW harness_eval_metrics AS
SELECT
    agent_id, sensor_name, track,
    COUNT(*) as total_evals,
    AVG(CASE WHEN pass THEN 1.0 ELSE 0.0 END) as pass_rate,
    AVG(score) as avg_score,
    AVG(round) as avg_rounds_to_pass,
    AVG(duration_ms) as avg_duration_ms
FROM harness_evaluations
GROUP BY agent_id, sensor_name, track;
