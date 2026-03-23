CREATE TABLE IF NOT EXISTS file_access_log (
    id            BIGSERIAL PRIMARY KEY,
    actor_id      TEXT NOT NULL,
    actor_type    TEXT NOT NULL,
    session_key   TEXT,
    action        TEXT NOT NULL,
    resource      TEXT NOT NULL,
    resource_type TEXT NOT NULL,
    source        TEXT NOT NULL,
    ip_address    TEXT,
    metadata      JSONB DEFAULT '{}'::jsonb,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_file_access_log_actor
    ON file_access_log (actor_id, created_at);
CREATE INDEX IF NOT EXISTS idx_file_access_log_resource
    ON file_access_log (resource, created_at);
CREATE INDEX IF NOT EXISTS idx_file_access_log_action_deny
    ON file_access_log (action, created_at)
    WHERE action = 'deny';
