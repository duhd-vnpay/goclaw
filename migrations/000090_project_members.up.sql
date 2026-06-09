-- 000062: Project Members + Permissions (Identity Phase 4)

CREATE TABLE project_members (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id      UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    user_id         UUID NOT NULL REFERENCES org_users(id) ON DELETE CASCADE,
    role            VARCHAR(50) NOT NULL,
    permissions     JSONB DEFAULT '{}',
    joined_at       TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(project_id, user_id)
);

CREATE INDEX idx_project_members_user ON project_members(user_id);
CREATE INDEX idx_project_members_project ON project_members(project_id);
