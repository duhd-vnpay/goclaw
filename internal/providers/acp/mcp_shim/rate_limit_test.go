package mcp_shim

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

// TestServer_RateLimitBlocks101stCall verifies the per-session token-bucket
// (Layer 4): the first 100 tools/call requests succeed; the 101st returns an
// MCP tool error containing "rate_limit_exceeded".
func TestServer_RateLimitBlocks101stCall(t *testing.T) {
	reg := newTestRegistry(t, "noop")
	s := newTestServer(t, reg)

	s.RegisterSession(SessionEntry{
		SID:       "sid-rl",
		Allowlist: []string{"noop"},
	})

	for i := 1; i <= defaultRateCapacity; i++ {
		body := rpcRequest(t, i, "tools/call", map[string]any{
			"name":      "noop",
			"arguments": map[string]any{},
		})
		w := postMCP(t, s, "sid-rl", body)
		if w.Code != http.StatusOK {
			t.Fatalf("call %d: status=%d body=%s", i, w.Code, w.Body.String())
		}
		var cr callToolResult
		decodeResult(t, w.Body.Bytes(), &cr)
		if cr.IsError {
			t.Fatalf("call %d: unexpected IsError=true content=%+v", i, cr.Content)
		}
	}

	// 101st call must be rate-limited.
	body := rpcRequest(t, defaultRateCapacity+1, "tools/call", map[string]any{
		"name":      "noop",
		"arguments": map[string]any{},
	})
	w := postMCP(t, s, "sid-rl", body)
	if w.Code != http.StatusOK {
		t.Fatalf("call %d: status=%d body=%s", defaultRateCapacity+1, w.Code, w.Body.String())
	}
	var cr callToolResult
	decodeResult(t, w.Body.Bytes(), &cr)
	if !cr.IsError {
		t.Fatalf("expected IsError=true for 101st call, got: %+v", cr)
	}
	if len(cr.Content) == 0 || !strings.Contains(cr.Content[0].Text, "rate_limit_exceeded") {
		t.Errorf("expected rate_limit_exceeded message, got: %+v", cr.Content)
	}
}

// TestRateBucket_RefillsAfterInterval exercises the bucket's lazy-refill
// directly: after exhausting the bucket, rewinding lastRefill past the
// refill interval restores full capacity on the next take.
func TestRateBucket_RefillsAfterInterval(t *testing.T) {
	b := newRateBucket(3, 5*time.Minute)

	now := time.Now()
	for i := 0; i < 3; i++ {
		if !b.take(now) {
			t.Fatalf("take %d should succeed", i+1)
		}
	}
	if b.take(now) {
		t.Fatalf("4th take should fail (bucket empty)")
	}

	// Rewind lastRefill past refillEvery — next take should refill and pass.
	b.mu.Lock()
	b.lastRefill = now.Add(-6 * time.Minute)
	b.mu.Unlock()

	if !b.take(now) {
		t.Fatalf("take after refill window should succeed")
	}
	// Should have capacity-1 tokens left after the refill+take.
	b.mu.Lock()
	left := b.tokens
	b.mu.Unlock()
	if left != 2 {
		t.Errorf("expected 2 tokens left after refill+take, got %d", left)
	}
}

// TestRateBucket_NilSafe ensures a nil bucket is permissive (used by the
// handler's defensive fallback when SessionEntry has no bucket attached).
func TestRateBucket_NilSafe(t *testing.T) {
	var b *rateBucket
	if !b.take(time.Now()) {
		t.Errorf("nil bucket should permit take")
	}
}
