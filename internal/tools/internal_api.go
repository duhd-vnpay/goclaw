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

// allowedRoute defines a permitted method + path-prefix pair.
type allowedRoute struct {
	Method string `json:"method"`
	Prefix string `json:"prefix"`
}

// defaultAllowedRoutes is the fallback allowlist used when no settings are
// stored in the database yet (e.g. first boot before seed runs).
var defaultAllowedRoutes = []allowedRoute{
	{Method: "GET", Prefix: "/v1/projects/by-chat"},
	{Method: "POST", Prefix: "/v1/projects"},
	{Method: "PUT", Prefix: "/v1/projects/"},
}

// InternalAPITool lets agents call a restricted subset of the GoClaw HTTP API
// (project management only), bypassing SSRF protection and auto-injecting the
// gateway Bearer token.
//
// The allowed-route list is loaded from the builtin_tools.settings column for
// the "internal_api" row, making it configurable via the Builtin Tools UI
// without a code change or redeploy. It falls back to defaultAllowedRoutes
// when no settings are available.
type InternalAPITool struct {
	baseURL        string
	token          string
	client         *http.Client
	// settingsGetter is wired after store initialisation via SetSettingsGetter.
	// Signature: func(ctx, toolName) → (raw JSON, error)
	settingsGetter func(ctx context.Context, name string) (json.RawMessage, error)
}

// NewInternalAPITool creates the tool. Call SetSettingsGetter once the store
// is available to enable DB-backed allowlist management.
func NewInternalAPITool(port int, token string) *InternalAPITool {
	return &InternalAPITool{
		baseURL: fmt.Sprintf("http://localhost:%d", port),
		token:   token,
		client:  &http.Client{Timeout: 15 * time.Second},
	}
}

// SetSettingsGetter wires the builtin-tool settings loader.
// Typically called once in gateway.go after pgStores is initialised:
//
//	iat.SetSettingsGetter(pgStores.BuiltinTools.GetSettings)
func (t *InternalAPITool) SetSettingsGetter(fn func(ctx context.Context, name string) (json.RawMessage, error)) {
	t.settingsGetter = fn
}

func (t *InternalAPITool) Name() string { return "internal_api" }

func (t *InternalAPITool) Description() string {
	return "Call GoClaw's project management API. Auth is handled automatically. " +
		"Allowed: GET /v1/projects/by-chat (resolve project from channel/chat_id), " +
		"POST /v1/projects (create project), " +
		"PUT /v1/projects/{id}/mcp/{server} (set MCP overrides). " +
		"Allowed routes are configurable via Admin → Builtin Tools → internal_api."
}

func (t *InternalAPITool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"method": map[string]any{
				"type":        "string",
				"description": `HTTP method: "GET", "POST", or "PUT".`,
				"enum":        []string{"GET", "POST", "PUT"},
			},
			"path": map[string]any{
				"type": "string",
				"description": `API path. Allowed paths:
- GET  /v1/projects/by-chat?channel_type=telegram&chat_id=-100123
- POST /v1/projects
- PUT  /v1/projects/{id}/mcp/{serverName}`,
			},
			"body": map[string]any{
				"type":        "object",
				"description": "JSON request body (for POST/PUT). Omit for GET.",
			},
		},
		"required": []string{"method", "path"},
	}
}

// loadAllowedRoutes returns the current allowlist from DB settings, or falls
// back to defaultAllowedRoutes when the getter is not wired or the row is absent.
func (t *InternalAPITool) loadAllowedRoutes(ctx context.Context) []allowedRoute {
	if t.settingsGetter == nil {
		return defaultAllowedRoutes
	}
	raw, err := t.settingsGetter(ctx, "internal_api")
	if err != nil || len(raw) == 0 {
		return defaultAllowedRoutes
	}

	var settings struct {
		AllowedRoutes []allowedRoute `json:"allowed_routes"`
	}
	if err := json.Unmarshal(raw, &settings); err != nil || len(settings.AllowedRoutes) == 0 {
		return defaultAllowedRoutes
	}
	return settings.AllowedRoutes
}

// checkAllowlist returns a non-empty error string when method+path is denied.
func checkAllowlist(method, path string, routes []allowedRoute) string {
	// Strip query string for prefix matching
	pathOnly := path
	if i := strings.IndexByte(path, '?'); i >= 0 {
		pathOnly = path[:i]
	}
	for _, r := range routes {
		if r.Method == method && strings.HasPrefix(pathOnly, r.Prefix) {
			return ""
		}
	}
	return fmt.Sprintf(
		"internal_api: %s %s is not in the allowlist — "+
			"configure allowed routes via Admin → Builtin Tools → internal_api",
		method, pathOnly,
	)
}

func (t *InternalAPITool) Execute(ctx context.Context, args map[string]any) *Result {
	method, _ := args["method"].(string)
	path, _ := args["path"].(string)

	if method == "" || path == "" {
		return ErrorResult("method and path are required")
	}

	method = strings.ToUpper(method)

	// Ensure path starts with /
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	// Enforce allowlist — loaded from DB settings (or default)
	routes := t.loadAllowedRoutes(ctx)
	if errMsg := checkAllowlist(method, path, routes); errMsg != "" {
		slog.Warn("internal_api: denied by allowlist", "method", method, "path", path)
		return ErrorResult(errMsg)
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
