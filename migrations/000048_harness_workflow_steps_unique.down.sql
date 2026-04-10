ALTER TABLE harness_workflow_steps DROP CONSTRAINT IF EXISTS uq_hws_run_step;
CREATE INDEX IF NOT EXISTS idx_hws_run ON harness_workflow_steps(run_id, step_id);
