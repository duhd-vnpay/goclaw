-- 000064: Memory isolation per project (project-as-a-channel)

-- Add project_id to memory_documents for project-scoped memory.
ALTER TABLE memory_documents
    ADD COLUMN IF NOT EXISTS project_id UUID REFERENCES projects(id) ON DELETE SET NULL;

-- Add project_id to memory_chunks (denormalized for fast scoped search).
ALTER TABLE memory_chunks
    ADD COLUMN IF NOT EXISTS project_id UUID REFERENCES projects(id) ON DELETE SET NULL;

-- Index for project-scoped queries.
CREATE INDEX IF NOT EXISTS idx_memory_documents_project
    ON memory_documents(project_id) WHERE project_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_memory_chunks_project
    ON memory_chunks(project_id) WHERE project_id IS NOT NULL;
