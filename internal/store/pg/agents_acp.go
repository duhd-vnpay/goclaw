package pg

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
)

// GetAgentACPTools returns the agents.acp_tools JSONB array for the given
// agent ID. This is the per-agent builtin tool allowlist that the ACP shim
// resolver intersects with mcp.BridgeToolNames before exposing tools to
// claude-agent-acp sessions (Phase 4 / migration 000092).
//
// Returns an empty slice (nil error) when the column is empty/null or the
// agent has no acp_tools configured. Returns sql.ErrNoRows wrapped only when
// the agent row itself does not exist. Cross-tenant: this query is
// intentionally unscoped (no tenant_id filter) so the shim grants store can
// resolve from the goclaw process identity at registration time. The shim
// resolver applies BridgeToolNames intersect downstream — exposing tools is
// gated by the registry presence + allowlist, not by reading this row.
func (s *PGAgentStore) GetAgentACPTools(ctx context.Context, agentID uuid.UUID) ([]string, error) {
	var raw []byte
	err := s.db.QueryRowContext(ctx,
		`SELECT acp_tools FROM agents WHERE id = $1 AND deleted_at IS NULL`,
		agentID,
	).Scan(&raw)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, sql.ErrNoRows
		}
		return nil, err
	}
	if len(raw) == 0 {
		return nil, nil
	}
	var out []string
	if err := json.Unmarshal(raw, &out); err != nil {
		// Malformed JSON in DB — treat as empty rather than erroring so a
		// bad seed row doesn't break ACP session setup.
		return nil, nil
	}
	return out, nil
}
