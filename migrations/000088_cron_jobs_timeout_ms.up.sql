-- Add per-job timeout override to cron_jobs.
-- NULL means "use config default" (cfg.Cron.JobTimeoutDuration()).
-- Use cases:
--   1. Long-running orchestrator agents (e.g. newsroom-conductor delegating via team_tasks)
--      should fail fast instead of waiting for the global 30m + 3 retries = 2h budget.
--   2. Quick heartbeat jobs can override with smaller timeouts if needed.
ALTER TABLE cron_jobs ADD COLUMN timeout_ms INTEGER DEFAULT NULL;
