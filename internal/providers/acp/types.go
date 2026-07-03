package acp

import "encoding/json"

// --- Client -> Agent Requests ---

type InitializeRequest struct {
	ProtocolVersion int        `json:"protocolVersion"`
	ClientInfo      ClientInfo `json:"clientInfo"`
	Capabilities    ClientCaps `json:"clientCapabilities"`
}

type ClientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type ClientCaps struct {
	Fs       *FsCaps       `json:"fs,omitempty"`
	Terminal *TerminalCaps `json:"terminal,omitempty"`
}

type FsCaps struct {
	ReadTextFile  bool `json:"readTextFile"`
	WriteTextFile bool `json:"writeTextFile"`
}

type TerminalCaps struct {
	Enabled bool `json:"enabled"`
}

type InitializeResponse struct {
	AgentInfo    AgentInfo `json:"agentInfo"`
	Capabilities AgentCaps `json:"agentCapabilities"`
}

type AgentInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type AgentCaps struct {
	LoadSession         bool         `json:"loadSession"`
	PromptCapabilities  *PromptCaps  `json:"promptCapabilities,omitempty"`
	SessionCapabilities *SessionCaps `json:"sessionCapabilities,omitempty"`
	MCPCapabilities     *MCPCaps     `json:"mcpCapabilities,omitempty"`
}

type PromptCaps struct {
	Audio           bool `json:"audio"`
	Image           bool `json:"image"`
	EmbeddedContext bool `json:"embeddedContext"`
}

type SessionCaps struct{}

type MCPCaps struct {
	HTTP bool `json:"http"`
	SSE  bool `json:"sse"`
}

// --- Session Methods ---

// McpServerHTTP is the HTTP-transport McpServer variant the ACP wrapper
// validates against schema.json/$defs/McpServerHttp. Required fields per
// schema: type (const "http"), name, url, headers. The `headers` array must
// be present (can be empty) — omitting it triggers `-32602 Invalid params`.
//
// Sent inside NewSessionRequest.McpServers / LoadSessionRequest.McpServers
// when shim is wired (Phase 4). For Phase 3 / no-shim path the slice stays
// nil/empty which marshals to `[]` — also valid.
type McpServerHTTP struct {
	Type    string       `json:"type"` // const "http"
	Name    string       `json:"name"`
	URL     string       `json:"url"`
	Headers []HTTPHeader `json:"headers"`
}

// HTTPHeader matches schema.json/$defs/HttpHeader. Name + value both required.
type HTTPHeader struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type NewSessionRequest struct {
	Cwd        string `json:"cwd"`
	// McpServers carries the McpServer discriminated union from the schema.
	// We marshal one McpServerHTTP per shim URL today; any future stdio/sse
	// variants get the same []any treatment.
	McpServers []any `json:"mcpServers"`
}

type NewSessionResponse struct {
	SessionID string `json:"sessionId"`
}

type LoadSessionRequest struct {
	SessionID  string `json:"sessionId"`
	Cwd        string `json:"cwd,omitempty"`
	McpServers []any  `json:"mcpServers"`
}

type LoadSessionResponse struct {
	SessionID string `json:"sessionId"`
}

type PromptRequest struct {
	SessionID string         `json:"sessionId"`
	Prompt    []ContentBlock `json:"prompt"`
}

type PromptResponse struct {
	StopReason string `json:"stopReason,omitempty"`
}

type CancelNotification struct {
	SessionID string `json:"sessionId"`
}

// --- Content Blocks ---

type ContentBlock struct {
	Type     string `json:"type"` // "text", "image", "audio"
	Text     string `json:"text,omitempty"`
	Data     string `json:"data,omitempty"`
	MimeType string `json:"mimeType,omitempty"`
}

// --- Agent -> Client Notifications ---

type SessionUpdate struct {
	SessionID  string `json:"sessionId"`
	StopReason string `json:"stopReason,omitempty"`
	
	Kind     string          `json:"kind,omitempty"`
	Message  *MessageUpdate  `json:"message,omitempty"`
	ToolCall *ToolCallUpdate `json:"toolCall,omitempty"`

	Update struct {
		SessionUpdate string `json:"sessionUpdate"`
		MessageID     string `json:"messageId,omitempty"`

		Content json.RawMessage `json:"content,omitempty"`

		Entries []struct {
			Content  string `json:"content"`
			Priority string `json:"priority"`
			Status   string `json:"status"`
		} `json:"entries,omitempty"`

		ToolCallID string `json:"toolCallId,omitempty"`
		Title      string `json:"title,omitempty"`
		Kind       string `json:"kind,omitempty"`
		Status     string `json:"status,omitempty"`
	} `json:"update"`
}

type MessageUpdate struct {
	Role      string         `json:"role"`
	Content   []ContentBlock `json:"content"`
	MessageID string         `json:"messageId,omitempty"`
}

type ToolCallUpdate struct {
	ID      string         `json:"id"`
	Name    string         `json:"name"`
	Status  string         `json:"status"`
	Content []ContentBlock `json:"content,omitempty"`
}

// --- Agent -> Client Requests ---

type ReadTextFileRequest struct {
	Path string `json:"path"`
}

type ReadTextFileResponse struct {
	Content string `json:"content"`
}

type WriteTextFileRequest struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type WriteTextFileResponse struct{}

type CreateTerminalRequest struct {
	Command string   `json:"command"`
	Args    []string `json:"args,omitempty"`
	Cwd     string   `json:"cwd,omitempty"`
}

type CreateTerminalResponse struct {
	TerminalID string `json:"terminalId"`
}

type TerminalOutputRequest struct {
	TerminalID string `json:"terminalId"`
}

type TerminalOutputResponse struct {
	Output     string `json:"output"`
	ExitStatus *int   `json:"exitStatus,omitempty"`
}

type ReleaseTerminalRequest struct {
	TerminalID string `json:"terminalId"`
}

type ReleaseTerminalResponse struct{}

type WaitForTerminalExitRequest struct {
	TerminalID string `json:"terminalId"`
}

type WaitForTerminalExitResponse struct {
	ExitStatus int `json:"exitStatus"`
}

type KillTerminalRequest struct {
	TerminalID string `json:"terminalId"`
}

type KillTerminalResponse struct{}

// RequestPermissionRequest matches schema.json/$defs/RequestPermissionRequest
// (x-method "session/request_permission", x-side "client"). The wrapper sends
// this before every tool call; required fields per schema: sessionId, toolCall,
// options. ToolCall is left as raw JSON because we only need to introspect
// `kind` for the approve-reads mode; everything else is opaque pass-through.
type RequestPermissionRequest struct {
	SessionID string             `json:"sessionId"`
	ToolCall  PermissionToolCall `json:"toolCall"`
	Options   []PermissionOption `json:"options"`
}

// PermissionToolCall is the subset of ToolCallUpdate we read for permission
// decisions. Other fields (locations, content, status) are ignored.
type PermissionToolCall struct {
	ToolCallID string `json:"toolCallId"`
	Title      string `json:"title,omitempty"`
	Kind       string `json:"kind,omitempty"`
}

// PermissionOption matches schema.json/$defs/PermissionOption. Kind is one of
// allow_once|allow_always|reject_once|reject_always per PermissionOptionKind.
type PermissionOption struct {
	OptionID string `json:"optionId"`
	Name     string `json:"name"`
	Kind     string `json:"kind"`
}

// RequestPermissionResponse matches schema.json/$defs/RequestPermissionResponse.
// outcome is a discriminated union: {"outcome":"cancelled"} or
// {"outcome":"selected", "optionId":"..."}. We always return "selected" — the
// wrapper never sees cancelled outcomes today (Phase 4 cron has no user to
// cancel mid-flight).
type RequestPermissionResponse struct {
	Outcome  string `json:"outcome"`
	OptionID string `json:"optionId,omitempty"`
}
