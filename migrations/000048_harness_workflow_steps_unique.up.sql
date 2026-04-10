-- Add unique constraint on (run_id, step_id) so ON CONFLICT clause works correctly.
-- Drop the non-unique index first to avoid duplicate index overhead.
DROP INDEX IF EXISTS idx_hws_run;
ALTER TABLE harness_workflow_steps ADD CONSTRAINT uq_hws_run_step UNIQUE (run_id, step_id);
