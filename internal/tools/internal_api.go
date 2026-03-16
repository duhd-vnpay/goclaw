package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// InternalAPITool lets agents call the GoClaw HTTP API directly,
// bypassing SSRF protection and auto-injecting the gateway Bearer token.
type InternalAPITool struct {
	baseURL string // e.g. "http://localhost:18790"
	token   string // gateway token for Authorization header
	client  *http.Client
}

// NewInternalAPITool creates a tool for agents to call GoClaw's own REST API.
func NewInternalAPITool(port int, token string) *InternalAPITool {
	return &InternalAPITool{
		baseURL: fmt.Sprintf("http://localhost:%d", port),
		token:   token,
		client:  &http.Client{Timeout: 15 * time.Second},
	}
}

func (t *InternalAPITool) Name() string { return "internal_api" }

func (t *InternalAPITool) Description() string {
	return "Call GoClaw's internal REST API. Auth is handled automatically. Use for project management (/v1/projects), MCP overrides, and other gateway endpoints."
}

func (t *InternalAPITool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"method": map[string]any{
				"type":        "string",
				"description": `HTTP method: "GET", "POST", "PUT", "DELETE".`,
				"enum":        []string{"GET", "POST", "PUT", "DELETE"},
			},
			"path": map[string]any{
				"type":        "string",
				"description": `API path, e.g. "/v1/projects" or "/v1/projects/{id}/mcp/gitlab". Query params included.`,
			},
			"body": map[string]any{
				"type":        "object",
				"description": "JSON request body (for POST/PUT). Omit for GET/DELETE.",
			},
		},
		"required": []string{"method", "path"},
	}
}

func (t *InternalAPITool) Execute(ctx context.Context, args map[string]any) *Result {
	method, _ := args["method"].(string)
	path, _ := args["path"].(string)

	if method == "" || path == "" {
		return ErrorResult("method and path are required")
	}

	method = strings.ToUpper(method)
	if method != "GET" && method != "POST" && method != "PUT" && method != "DELETE" {
		return ErrorResult("method must be GET, POST, PUT, or DELETE")
	}

	// Ensure path starts with /
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	url := t.baseURL + path

	var bodyReader io.Reader
	if body, ok := args["body"]; ok && body != nil && (method == "POST" || method == "PUT") {
		b, err := json.Marshal(body)
		if err != nil {
			return ErrorResult(fmt.Sprintf("failed to marshal body: %v", err))
		}
		bodyReader = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to create request: %v", err))
	}

	if t.token != "" {
		req.Header.Set("Authorization", "Bearer "+t.token)
	}
	if bodyReader != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	slog.Debug("internal_api", "method", method, "path", path)

	resp, err := t.client.Do(req)
	if err != nil {
		return ErrorResult(fmt.Sprintf("request failed: %v", err))
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024)) // 64KB limit
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to read response: %v", err))
	}

	result := fmt.Sprintf("HTTP %d\n%s", resp.StatusCode, string(respBody))

	if resp.StatusCode >= 400 {
		return ErrorResult(result)
	}

	return NewResult(result)
}
