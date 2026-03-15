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

// testStreamEnv creates a Channel with mock auth and HTTP server for stream tests.
func testStreamEnv(t *testing.T) (*Channel, *httptest.Server, *[]requestRecord) {
	t.Helper()
	var mu sync.Mutex
	var records []requestRecord

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		mu.Lock()
		records = append(records, requestRecord{
			Method: r.Method,
			URL:    r.URL.String(),
			Body:   body,
		})
		mu.Unlock()

		if r.Method == "POST" {
			json.NewEncoder(w).Encode(map[string]any{
				"name":   "spaces/test/messages/new-1",
				"thread": map[string]string{"name": "spaces/test/threads/t1"},
			})
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	ch := &Channel{
		auth: &ServiceAccountAuth{
			token:     "test-token",
			expiresAt: time.Now().Add(1 * time.Hour),
		},
		apiBase:           srv.URL,
		httpClient:        srv.Client(),
		dmStream:          true,
		groupStream:       false,
		longFormThreshold: longFormThresholdDefault,
	}

	return ch, srv, &records
}

func getRecords(records *[]requestRecord) []requestRecord {
	return *records
}

func TestChatStream_Dedup(t *testing.T) {
	ch, _, records := testStreamEnv(t)
	cs := newChatStream(ch, "spaces/test/messages/m1")

	ctx := context.Background()

	// First update should send
	cs.update(ctx, "Hello")
	// Same text should be deduped
	cs.update(ctx, "Hello")
	// Wait for any flush timers
	time.Sleep(50 * time.Millisecond)

	recs := getRecords(records)
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
	ch, _, records := testStreamEnv(t)
	cs := newChatStream(ch, "spaces/test/messages/m1")
	ctx := context.Background()

	// First update sends immediately
	cs.update(ctx, "chunk1")
	// Second update within throttle window should be buffered
	cs.update(ctx, "chunk2")
	// Third update within throttle window should replace pending
	cs.update(ctx, "chunk3")

	// Only 1 PATCH should have been sent (chunk1)
	time.Sleep(50 * time.Millisecond) // let goroutines settle
	recs := getRecords(records)
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

	recs = getRecords(records)
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
	ch, _, records := testStreamEnv(t)
	cs := newChatStream(ch, "spaces/test/messages/m1")
	ctx := context.Background()

	// Send first to start throttle window
	cs.update(ctx, "first")
	// Buffer pending
	cs.update(ctx, "pending-text")
	// Stop should flush pending
	cs.stop(ctx)

	recs := getRecords(records)
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
