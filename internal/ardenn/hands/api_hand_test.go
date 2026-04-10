package hands

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	engine "github.com/nextlevelbuilder/goclaw/internal/ardenn"
)

func TestAPIHand_Execute_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"status":"ok"}`)
	}))
	defer server.Close()

	hand := NewAPIHand(server.Client())

	req := engine.HandRequest{
		RunID:     uuid.New(),
		StepRunID: uuid.New(),
		Name:      server.URL,
		Input:     `{"key":"value"}`,
		Metadata:  map[string]any{},
	}

	result := hand.Execute(context.Background(), req)

	if result.Error != nil {
		t.Fatalf("expected no error, got %v", result.Error)
	}
	if result.Output != `{"status":"ok"}` {
		t.Errorf("expected output '{\"status\":\"ok\"}', got %q", result.Output)
	}
}

func TestAPIHand_Execute_RetryOn5xx(t *testing.T) {
	var count atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := count.Add(1)
		if n <= 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprint(w, "service unavailable")
			return
		}
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"retry":"success"}`)
	}))
	defer server.Close()

	hand := NewAPIHand(server.Client())

	req := engine.HandRequest{
		RunID:     uuid.New(),
		StepRunID: uuid.New(),
		Name:      server.URL,
		Input:     "{}",
		Metadata:  map[string]any{},
		Timeout:   10 * time.Second, // enough for retries
	}

	result := hand.Execute(context.Background(), req)

	if result.Error != nil {
		t.Fatalf("expected success after retries, got %v", result.Error)
	}
	if result.Output != `{"retry":"success"}` {
		t.Errorf("unexpected output: %q", result.Output)
	}
	if got := count.Load(); got != 3 {
		t.Errorf("expected 3 total requests, got %d", got)
	}
}

func TestAPIHand_Execute_MaxRetryExhausted(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "internal error")
	}))
	defer server.Close()

	hand := NewAPIHand(server.Client())

	req := engine.HandRequest{
		RunID:     uuid.New(),
		StepRunID: uuid.New(),
		Name:      server.URL,
		Input:     "{}",
		Metadata:  map[string]any{},
		Timeout:   15 * time.Second,
	}

	result := hand.Execute(context.Background(), req)

	if result.Error == nil {
		t.Fatal("expected error after max retries, got nil")
	}
	errMsg := result.Error.Error()
	if !contains(errMsg, "max retries") {
		t.Errorf("expected error containing 'max retries', got %q", errMsg)
	}
	if !contains(errMsg, "500") {
		t.Errorf("expected error containing '500', got %q", errMsg)
	}
}

func TestAPIHand_Execute_Timeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	hand := NewAPIHand(server.Client())

	req := engine.HandRequest{
		RunID:     uuid.New(),
		StepRunID: uuid.New(),
		Name:      server.URL,
		Input:     "{}",
		Metadata:  map[string]any{},
		Timeout:   50 * time.Millisecond,
	}

	result := hand.Execute(context.Background(), req)

	if result.Error == nil {
		t.Fatal("expected timeout error, got nil")
	}
	errMsg := result.Error.Error()
	if !contains(errMsg, "deadline") && !contains(errMsg, "timeout") && !contains(errMsg, "context") {
		t.Errorf("expected timeout-related error, got %q", errMsg)
	}
}

func TestAPIHand_Execute_CustomHeaders(t *testing.T) {
	var receivedAuth string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ok")
	}))
	defer server.Close()

	hand := NewAPIHand(server.Client())

	req := engine.HandRequest{
		RunID:     uuid.New(),
		StepRunID: uuid.New(),
		Name:      server.URL,
		Input:     "{}",
		Metadata: map[string]any{
			"headers": map[string]any{
				"Authorization": "Bearer secret-token",
			},
		},
	}

	result := hand.Execute(context.Background(), req)

	if result.Error != nil {
		t.Fatalf("expected no error, got %v", result.Error)
	}
	if receivedAuth != "Bearer secret-token" {
		t.Errorf("expected Authorization header 'Bearer secret-token', got %q", receivedAuth)
	}
}
