-- 000064: Rollback memory project scope

DROP INDEX IF EXISTS idx_memory_chunks_project;
DROP INDEX IF EXISTS idx_memory_documents_project;

ALTER TABLE memory_chunks DROP COLUMN IF EXISTS project_id;
ALTER TABLE memory_documents DROP COLUMN IF EXISTS project_id;
