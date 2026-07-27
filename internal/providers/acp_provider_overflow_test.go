package providers

import (
	"errors"
	"testing"
)

// TestPurgeIfContextOverflow verifies the goclaw→ACP session mapping is
// dropped only on context-overflow errors, so the next resolveSession call
// creates a fresh ACP session instead of retrying against an agent-side
// transcript that can never fit the model context again.
func TestPurgeIfContextOverflow(t *testing.T) {
	cases := []struct {
		name      string
		err       error
		wantPurge bool
	}{
		{"prompt too long (anthropic via acp)", errors.New("acp session/prompt: jsonrpc error -32603: Internal error: Prompt is too long"), true},
		{"context length exceeded", errors.New("context length exceeded"), true},
		{"nil error", nil, false},
		{"unrelated error", errors.New("acp session/prompt: context deadline exceeded"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := &ACPProvider{}
			const key = "agent:x:team:t1:t1"
			p.acpSessions.Store(key, &acpSessionEntry{id: "sid-1"})

			p.purgeIfContextOverflow(key, "sid-1", tc.err)

			_, stillThere := p.acpSessions.Load(key)
			if tc.wantPurge && stillThere {
				t.Errorf("expected session %q purged, but entry still present", key)
			}
			if !tc.wantPurge && !stillThere {
				t.Errorf("expected session %q kept, but entry was purged", key)
			}
		})
	}
}
