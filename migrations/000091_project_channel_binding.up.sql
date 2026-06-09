-- 000063: Project-as-a-Channel — bind projects to channel type + chat ID

-- Add channel binding columns to projects table.
-- channel_type: factory name (e.g. 'telegram', 'zalo_oa', 'discord')
-- chat_id: the group/DM chat ID where this project lives
ALTER TABLE projects
    ADD COLUMN IF NOT EXISTS channel_type VARCHAR(50),
    ADD COLUMN IF NOT EXISTS chat_id VARCHAR(255),
    ADD COLUMN IF NOT EXISTS slug VARCHAR(255);

-- Unique constraint: one project per channel_type + chat_id
-- Uses partial index to allow NULL chat_id (projects not yet bound).
CREATE UNIQUE INDEX IF NOT EXISTS idx_projects_channel_binding
    ON projects(tenant_id, channel_type, chat_id)
    WHERE channel_type IS NOT NULL AND chat_id IS NOT NULL;

-- Index for fast lookup by channel_type + chat_id (the hot path).
CREATE INDEX IF NOT EXISTS idx_projects_chat_lookup
    ON projects(channel_type, chat_id)
    WHERE channel_type IS NOT NULL AND chat_id IS NOT NULL;
