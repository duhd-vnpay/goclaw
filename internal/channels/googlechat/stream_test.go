package googlechat

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// requestRecord captures an HTTP request for test assertions.
type requestRecord struct {
	Method string
	URL    string
	Body   map[string]any
}

// streamTestEnv holds test infrastructure for stream tests.
type streamTestEnv struct {
	ch  *Channel
	srv *httptest.Server
	mu  sync.Mutex
	recs []requestRecord
}

// getRecords returns a snapshot of recorded requests (thread-safe).
func (e *streamTestEnv) getRecords() []requestRecord {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]requestRecord, len(e.recs))
	copy(out, e.recs)
	return out
}

// testStreamEnv creates a Channel with mock auth and HTTP server for stream tests.
func testStreamEnv(t *testing.T) *streamTestEnv {
	t.Helper()
	env := &streamTestEnv{}

	env.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		env.mu.Lock()
		env.recs = append(env.recs, requestRecord{
			Method: r.Method,
			URL:    r.URL.String(),
			Body:   body,
		})
		env.mu.Unlock()

		if r.Method == "POST" {
			json.NewEncoder(w).Encode(map[string]any{
				"name":   "spaces/test/messages/new-1",
				"thread": map[string]string{"name": "spaces/test/threads/t1"},
			})
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(env.srv.Close)

	env.ch = &Channel{
		auth: &ServiceAccountAuth{
			token:     "test-token",
			expiresAt: time.Now().Add(1 * time.Hour),
		},
		apiBase:           env.srv.URL,
		httpClient:        env.srv.Client(),
		dmStream:          true,
		groupStream:       false,
		longFormThreshold: longFormThresholdDefault,
	}

	return env
}

func TestChatStream_Dedup(t *testing.T) {
	env := testStreamEnv(t)
	cs := newChatStream(env.ch, "spaces/test/messages/m1")

	ctx := context.Background()

	// First update should send
	cs.update(ctx, "Hello")
	// Same text should be deduped
	cs.update(ctx, "Hello")
	// Wait for any flush timers
	time.Sleep(50 * time.Millisecond)

	recs := env.getRecords()
	patchCount := 0
	for _, r := range recs {
		if r.Method == "PATCH" {
			patchCount++
		}
	}
	if patchCount != 1 {
		t.Errorf("expected 1 PATCH (dedup), got %d", patchCount)
	}
}

func TestChatStream_Throttle(t *testing.T) {
	env := testStreamEnv(t)
	cs := newChatStream(env.ch, "spaces/test/messages/m1")
	ctx := context.Background()

	// First update sends immediately
	cs.update(ctx, "chunk1")
	// Second update within throttle window should be buffered
	cs.update(ctx, "chunk2")
	// Third update within throttle window should replace pending
	cs.update(ctx, "chunk3")

	// Only 1 PATCH should have been sent (chunk1)
	time.Sleep(50 * time.Millisecond) // let goroutines settle
	recs := env.getRecords()
	immediatePatch := 0
	for _, r := range recs {
		if r.Method == "PATCH" {
			immediatePatch++
		}
	}
	if immediatePatch != 1 {
		t.Errorf("expected 1 immediate PATCH, got %d", immediatePatch)
	}

	// Wait for flush timer to fire
	time.Sleep(defaultStreamThrottle + 200*time.Millisecond)

	recs = env.getRecords()
	totalPatch := 0
	for _, r := range recs {
		if r.Method == "PATCH" {
			totalPatch++
		}
	}
	// Should now have 2 PATCHes: immediate (chunk1) + timer flush (chunk3)
	if totalPatch != 2 {
		t.Errorf("expected 2 total PATCHes after timer, got %d", totalPatch)
	}
}

func TestChatStream_PendingFlush(t *testing.T) {
	env := testStreamEnv(t)
	cs := newChatStream(env.ch, "spaces/test/messages/m1")
	ctx := context.Background()

	// Send first to start throttle window
	cs.update(ctx, "first")
	// Buffer pending
	cs.update(ctx, "pending-text")
	// Stop should flush pending
	cs.stop(ctx)

	recs := env.getRecords()
	patchCount := 0
	var lastBody map[string]any
	for _, r := range recs {
		if r.Method == "PATCH" {
			patchCount++
			lastBody = r.Body
		}
	}
	if patchCount != 2 {
		t.Errorf("expected 2 PATCHes (initial + flush), got %d", patchCount)
	}
	if lastBody != nil {
		if text, ok := lastBody["text"].(string); ok {
			if !strings.Contains(text, "pending") {
				t.Errorf("final flush should contain pending text, got %q", text)
			}
		}
	}
}

func TestStreamEnabled_Config(t *testing.T) {
	ch := &Channel{dmStream: true, groupStream: false}
	if !ch.StreamEnabled(false) {
		t.Error("DM streaming should be enabled")
	}
	if ch.StreamEnabled(true) {
		t.Error("group streaming should be disabled")
	}

	ch2 := &Channel{dmStream: false, groupStream: true}
	if ch2.StreamEnabled(false) {
		t.Error("DM streaming should be disabled")
	}
	if !ch2.StreamEnabled(true) {
		t.Error("group streaming should be enabled")
	}
}

func TestOnStreamStart_ReusePlaceholder(t *testing.T) {
	env := testStreamEnv(t)
	ctx := context.Background()

	// Pre-store a placeholder
	env.ch.placeholders.Store("spaces/test", "spaces/test/messages/placeholder-1")

	err := env.ch.OnStreamStart(ctx, "spaces/test")
	if err != nil {
		t.Fatal(err)
	}

	// Should NOT have created a new message (no POST)
	recs := env.getRecords()
	for _, r := range recs {
		if r.Method == "POST" {
			t.Error("should reuse placeholder, not POST new message")
		}
	}

	// Stream should exist
	val, ok := env.ch.streams.Load("spaces/test")
	if !ok {
		t.Fatal("stream should be stored")
	}
	cs := val.(*chatStream)
	if cs.messageName != "spaces/test/messages/placeholder-1" {
		t.Errorf("stream should use placeholder message, got %q", cs.messageName)
	}

	// Placeholder should be consumed
	if _, ok := env.ch.placeholders.Load("spaces/test"); ok {
		t.Error("placeholder should be deleted after reuse")
	}
}

func TestOnStreamStart_CreateNew(t *testing.T) {
	env := testStreamEnv(t)
	ctx := context.Background()

	// No placeholder stored
	err := env.ch.OnStreamStart(ctx, "spaces/test")
	if err != nil {
		t.Fatal(err)
	}

	// Should have created a new message (POST)
	recs := env.getRecords()
	postCount := 0
	for _, r := range recs {
		if r.Method == "POST" {
			postCount++
			// Verify "⏳" text
			if text, ok := r.Body["text"].(string); ok {
				if text != "⏳" {
					t.Errorf("new stream message should have ⏳ text, got %q", text)
				}
			}
		}
	}
	if postCount != 1 {
		t.Errorf("expected 1 POST, got %d", postCount)
	}

	// Stream should exist with server-returned name
	val, ok := env.ch.streams.Load("spaces/test")
	if !ok {
		t.Fatal("stream should be stored")
	}
	cs := val.(*chatStream)
	if cs.messageName != "spaces/test/messages/new-1" {
		t.Errorf("stream should use server-returned name, got %q", cs.messageName)
	}
}

func TestOnStreamEnd_HandoffToPlaceholders(t *testing.T) {
	env := testStreamEnv(t)
	ctx := context.Background()

	// Simulate active stream
	cs := newChatStream(env.ch, "spaces/test/messages/stream-1")
	env.ch.streams.Store("spaces/test", cs)

	err := env.ch.OnStreamEnd(ctx, "spaces/test", "final text")
	if err != nil {
		t.Fatal(err)
	}

	// Stream should be removed
	if _, ok := env.ch.streams.Load("spaces/test"); ok {
		t.Error("stream should be deleted after OnStreamEnd")
	}

	// Message should be handed off to placeholders
	pName, ok := env.ch.placeholders.Load("spaces/test")
	if !ok {
		t.Fatal("placeholder should be stored for Send() pickup")
	}
	if pName.(string) != "spaces/test/messages/stream-1" {
		t.Errorf("placeholder should have stream message name, got %q", pName)
	}
}

func TestToolIteration_Reuse(t *testing.T) {
	env := testStreamEnv(t)
	ctx := context.Background()

	// Simulate: OnStreamStart (reuses placeholder) → chunks → OnStreamEnd("")
	env.ch.placeholders.Store("spaces/test", "spaces/test/messages/original")
	if err := env.ch.OnStreamStart(ctx, "spaces/test"); err != nil {
		t.Fatal(err)
	}

	// Stream some text
	env.ch.OnChunkEvent(ctx, "spaces/test", "partial response")

	// Tool call: OnStreamEnd with empty finalText
	if err := env.ch.OnStreamEnd(ctx, "spaces/test", ""); err != nil {
		t.Fatal(err)
	}

	// Message should be in placeholders
	pName, ok := env.ch.placeholders.Load("spaces/test")
	if !ok {
		t.Fatal("placeholder should exist after tool-phase OnStreamEnd")
	}

	// Next iteration: OnStreamStart should reuse from placeholders
	if err := env.ch.OnStreamStart(ctx, "spaces/test"); err != nil {
		t.Fatal(err)
	}

	val, ok := env.ch.streams.Load("spaces/test")
	if !ok {
		t.Fatal("stream should exist after restart")
	}
	cs2 := val.(*chatStream)
	if cs2.messageName != pName.(string) {
		t.Errorf("restarted stream should reuse message %q, got %q", pName, cs2.messageName)
	}
}
