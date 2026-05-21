-- 000063: Rollback project channel binding

DROP INDEX IF EXISTS idx_projects_chat_lookup;
DROP INDEX IF EXISTS idx_projects_channel_binding;

ALTER TABLE projects
    DROP COLUMN IF EXISTS slug,
    DROP COLUMN IF EXISTS chat_id,
    DROP COLUMN IF EXISTS channel_type;
