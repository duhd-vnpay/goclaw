ALTER TABLE agents
    ADD COLUMN IF NOT EXISTS acp_tools jsonb NOT NULL DEFAULT '[]'::jsonb;
COMMENT ON COLUMN agents.acp_tools IS 'Phase 4: per-agent ACP shim builtin tool allowlist (subset of mcp.BridgeToolNames). Empty array = no builtin tools exposed.';
