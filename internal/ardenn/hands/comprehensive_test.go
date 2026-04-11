package hands

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	engine "github.com/nextlevelbuilder/goclaw/internal/ardenn"
	"github.com/nextlevelbuilder/goclaw/internal/bus"
)

func TestAgentHand_Execute_NilMetadata(t *testing.T) {
	msgBus := bus.New()
	completion := NewCompletionRegistry()
	h := NewAgentHand(msgBus, completion)

	stepRunID := uuid.New()

	go func() {
		time.Sleep(50 * time.Millisecond)
		completion.Complete(stepRunID, engine.HandResult{Output: "ok"})
	}()

	// Metadata is nil — should not panic
	result := h.Execute(context.Background(), engine.HandRequest{
		RunID:     uuid.New(),
		StepRunID: stepRunID,
		Name:      "test-agent",
		Input:     "Do something",
		Metadata:  nil,
		Timeout:   5 * time.Second,
	})

	if result.Error != nil {
		t.Fatalf("unexpected error: %v", result.Error)
	}
	if result.Output != "ok" {
		t.Errorf("output = %q, want 'ok'", result.Output)
	}
}

func TestUserHand_Type(t *testing.T) {
	cr := NewCompletionRegistry()
	h := NewUserHand(cr)
	if h.Type() != engine.HandUser {
		t.Errorf("Type() = %v, want %v", h.Type(), engine.HandUser)
	}
}

func TestAPIHand_Type(t *testing.T) {
	h := NewAPIHand(nil)
	if h.Type() != engine.HandAPI {
		t.Errorf("Type() = %v, want %v", h.Type(), engine.HandAPI)
	}
}

func TestAPIHand_Execute_4xxNoRetry(t *testing.T) {
	var requestCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"error":"bad request"}`)
	}))
	defer server.Close()

	hand := NewAPIHand(server.Client())

	req := engine.HandRequest{
		RunID:     uuid.New(),
		StepRunID: uuid.New(),
		Name:      server.URL,
		Input:     `{"invalid": true}`,
		Metadata:  map[string]any{},
	}

	result := hand.Execute(context.Background(), req)

	if result.Error == nil {
		t.Fatal("expected error for 400 response")
	}
	errMsg := result.Error.Error()
	if !contains(errMsg, "400") {
		t.Errorf("expected error containing '400', got %q", errMsg)
	}
	// 4xx should NOT retry — only 1 request
	if requestCount != 1 {
		t.Errorf("expected exactly 1 request (no retry for 4xx), got %d", requestCount)
	}
}

func TestCompletionRegistry_ConcurrentAccess(t *testing.T) {
	reg := NewCompletionRegistry()

	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines * 2)

	ids := make([]uuid.UUID, goroutines)
	for i := range ids {
		ids[i] = uuid.New()
	}

	// Register concurrently
	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			reg.Register(ids[idx])
		}(i)
	}

	// Complete/Deregister concurrently
	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			if idx%2 == 0 {
				reg.Complete(ids[idx], engine.HandResult{Output: "done"})
			} else {
				reg.Deregister(ids[idx])
			}
		}(i)
	}

	// Should not panic or deadlock
	wg.Wait()
}
