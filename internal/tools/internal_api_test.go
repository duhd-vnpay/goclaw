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
		if r.URL.Path != "/v1/projects/by-chat" {
			t.Errorf("expected /v1/projects/by-chat, got %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]any{"id": "abc", "slug": "xpos"})
	}))
	defer srv.Close()

	tool := &InternalAPITool{
		baseURL: srv.URL,
		token:   "test-token",
		client:  srv.Client(),
	}

	result := tool.Execute(context.Background(), map[string]any{
		"method": "GET",
		"path":   "/v1/projects/by-chat?channel_type=telegram&chat_id=-100123",
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

	// /v1/projects/by-chat is in the allowlist — this tests 404 handling
	result := tool.Execute(context.Background(), map[string]any{
		"method": "GET",
		"path":   "/v1/projects/by-chat?channel_type=telegram&chat_id=nonexistent",
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

	// Use an allowed path
	tool.Execute(context.Background(), map[string]any{
		"method": "GET",
		"path":   "/v1/projects/by-chat?channel_type=telegram&chat_id=-100",
	})

	if gotAuth != "" {
		t.Errorf("expected no Authorization header when token is empty, got %q", gotAuth)
	}
}

// TestInternalAPI_Allowlist verifies that the allowlist blocks denied paths
// and permits allowed paths, with and without a custom settings getter.
func TestInternalAPI_Allowlist(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	tool := &InternalAPITool{baseURL: srv.URL, token: "", client: srv.Client()}

	cases := []struct {
		method  string
		path    string
		wantErr bool
	}{
		// allowed by default
		{"GET", "/v1/projects/by-chat?channel_type=telegram&chat_id=-1", false},
		{"POST", "/v1/projects", false},
		{"PUT", "/v1/projects/abc-123/mcp/gitlab", false},
		// denied by default
		{"GET", "/v1/projects", true},
		{"DELETE", "/v1/projects/abc-123", true},
		{"GET", "/v1/agents", true},
		{"PUT", "/v1/agents/abc-123", true},
	}

	for _, c := range cases {
		res := tool.Execute(context.Background(), map[string]any{
			"method": c.method,
			"path":   c.path,
		})
		if c.wantErr && !res.IsError {
			t.Errorf("%s %s: expected denied, got allowed", c.method, c.path)
		}
		if !c.wantErr && res.IsError {
			t.Errorf("%s %s: expected allowed, got denied: %s", c.method, c.path, res.ForLLM)
		}
	}
}

// TestInternalAPI_SettingsGetter verifies that a custom settings getter
// overrides the default allowlist.
func TestInternalAPI_SettingsGetter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	tool := &InternalAPITool{baseURL: srv.URL, token: "", client: srv.Client()}

	// Custom allowlist: only GET /v1/agents
	tool.SetSettingsGetter(func(_ context.Context, _ string) (json.RawMessage, error) {
		return json.RawMessage(`{"allowed_routes":[{"method":"GET","prefix":"/v1/agents"}]}`), nil
	})

	// Now /v1/agents should be allowed, /v1/projects/by-chat should be denied
	resAllowed := tool.Execute(context.Background(), map[string]any{
		"method": "GET",
		"path":   "/v1/agents",
	})
	if resAllowed.IsError {
		t.Errorf("expected /v1/agents to be allowed by custom settings, got: %s", resAllowed.ForLLM)
	}

	resDenied := tool.Execute(context.Background(), map[string]any{
		"method": "GET",
		"path":   "/v1/projects/by-chat?channel_type=telegram&chat_id=-1",
	})
	if !resDenied.IsError {
		t.Error("expected /v1/projects/by-chat to be denied by custom settings")
	}
}
