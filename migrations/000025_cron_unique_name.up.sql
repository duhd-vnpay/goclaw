-- 000021: Add unique constraint on cron_jobs(agent_id, name)
-- Prevents duplicate jobs when seed is re-applied.

-- First, deduplicate existing rows: keep only the newest row per (agent_id, name)
DELETE FROM cron_run_logs WHERE job_id IN (
    SELECT id FROM cron_jobs
    WHERE id NOT IN (
        SELECT DISTINCT ON (agent_id, name) id
        FROM cron_jobs
        ORDER BY agent_id, name, created_at DESC
    )
);

DELETE FROM cron_jobs
WHERE id NOT IN (
    SELECT DISTINCT ON (agent_id, name) id
    FROM cron_jobs
    ORDER BY agent_id, name, created_at DESC
);

-- Now add the unique constraint
CREATE UNIQUE INDEX IF NOT EXISTS idx_cron_jobs_agent_name
    ON cron_jobs(agent_id, name);
