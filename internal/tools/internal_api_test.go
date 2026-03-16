package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestInternalAPI_GET(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if r.Method != "GET" {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/v1/projects" {
			t.Errorf("expected /v1/projects, got %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]any{"projects": []any{}})
	}))
	defer srv.Close()

	tool := &InternalAPITool{
		baseURL: srv.URL,
		token:   "test-token",
		client:  srv.Client(),
	}

	result := tool.Execute(context.Background(), map[string]any{
		"method": "GET",
		"path":   "/v1/projects",
	})

	if result.IsError {
		t.Fatalf("unexpected error: %s", result.ForLLM)
	}
	if result.ForLLM == "" {
		t.Fatal("expected non-empty result")
	}
}

func TestInternalAPI_POST(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Error("expected Content-Type: application/json")
		}

		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["name"] != "XPOS" {
			t.Errorf("expected name=XPOS, got %v", body["name"])
		}

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"id": "123"})
	}))
	defer srv.Close()

	tool := &InternalAPITool{
		baseURL: srv.URL,
		token:   "test-token",
		client:  srv.Client(),
	}

	result := tool.Execute(context.Background(), map[string]any{
		"method": "POST",
		"path":   "/v1/projects",
		"body":   map[string]any{"name": "XPOS", "slug": "xpos"},
	})

	if result.IsError {
		t.Fatalf("unexpected error: %s", result.ForLLM)
	}
}

func TestInternalAPI_ErrorResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "not found"})
	}))
	defer srv.Close()

	tool := &InternalAPITool{
		baseURL: srv.URL,
		token:   "",
		client:  srv.Client(),
	}

	result := tool.Execute(context.Background(), map[string]any{
		"method": "GET",
		"path":   "/v1/projects/nonexistent",
	})

	if !result.IsError {
		t.Fatal("expected error result for 404")
	}
}

func TestInternalAPI_MissingParams(t *testing.T) {
	tool := NewInternalAPITool(18790, "token")

	result := tool.Execute(context.Background(), map[string]any{})
	if !result.IsError {
		t.Fatal("expected error for missing params")
	}

	result = tool.Execute(context.Background(), map[string]any{"method": "GET"})
	if !result.IsError {
		t.Fatal("expected error for missing path")
	}

	result = tool.Execute(context.Background(), map[string]any{
		"method": "PATCH",
		"path":   "/v1/projects",
	})
	if !result.IsError {
		t.Fatal("expected error for invalid method")
	}
}

func TestInternalAPI_NoToken(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	tool := &InternalAPITool{
		baseURL: srv.URL,
		token:   "",
		client:  srv.Client(),
	}

	tool.Execute(context.Background(), map[string]any{
		"method": "GET",
		"path":   "/v1/projects",
	})

	if gotAuth != "" {
		t.Errorf("expected no Authorization header when token is empty, got %q", gotAuth)
	}
}
