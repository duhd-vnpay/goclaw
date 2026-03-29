package mcp

import (
	"context"
	"testing"
	"time"
)

func TestConnectAndDiscoverRetriesOnStdioInitFailure(t *testing.T) {
	// Use "cat" as a fake MCP server — it starts successfully (createClient works)
	// but doesn't speak JSON-RPC, so Initialize fails. The retry loop should
	// attempt multiple times with backoff before giving up.

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	start := time.Now()
	_, _, err := connectAndDiscover(ctx, "test-retry", "stdio",
		"cat", nil, nil, "", nil, 2)

	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error — cat doesn't speak MCP")
	}

	// With retries, should take at least 2s (first retry backoff).
	// Context times out at 5s. Retry backoff: attempt 1 = 2s, attempt 2 = 4s.
	// So we expect it to fail after ~2s (second attempt fails, third would need 4s
	// but context cancels first).
	if elapsed < time.Second {
		t.Errorf("elapsed %v — expected at least 1s proving retries happened", elapsed)
	}
	t.Logf("elapsed: %v (retries confirmed)", elapsed)
}

func TestConnectAndDiscoverNoRetryForSSE(t *testing.T) {
	// SSE/HTTP transports should fail immediately without retrying.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	start := time.Now()
	_, _, err := connectAndDiscover(ctx, "test-no-retry", "sse",
		"", nil, nil, "http://127.0.0.1:1", nil, 10)

	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error for bad SSE URL")
	}

	// SSE should fail fast (no retries) — well under 2 seconds.
	if elapsed > 2*time.Second {
		t.Errorf("elapsed %v — SSE should fail fast without retries", elapsed)
	}
}
