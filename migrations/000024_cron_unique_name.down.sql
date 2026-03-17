-- Rollback: remove the unique constraint
DROP INDEX IF EXISTS idx_cron_jobs_agent_name;
