package acp

import "regexp"

// teamSessionRe matches the GoClaw team-dispatch session key pattern:
//
//	agent:<agent-key>:team:<teamUUID>:<taskID>
//
// Capture group 1 is the team UUID. An empty teamUUID segment yields no match.
var teamSessionRe = regexp.MustCompile(`^agent:[^:]+:team:([^:]+):[^:]+$`)

// ParseTeamIDFromSessionKey extracts the team UUID from a team-pattern session
// key. Returns "" if the key is not a team session (cron, direct, malformed,
// empty). Used by Phase 5.1 shim relocate to scope file writes under
// <GOCLAW_DATA_DIR>/teams/<teamID>/.
func ParseTeamIDFromSessionKey(sessionKey string) string {
	if sessionKey == "" {
		return ""
	}
	m := teamSessionRe.FindStringSubmatch(sessionKey)
	if len(m) != 2 {
		return ""
	}
	return m[1]
}
