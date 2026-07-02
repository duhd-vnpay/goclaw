package acp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"regexp"
	"sync"
	"sync/atomic"
)

// debugScrubPatterns redacts credential-shaped substrings from ACP JSON-RPC
// wire dumps before they hit Debug logs. Security 2026-07-02 (audit P2#5):
// these lines can carry tool-result payloads (e.g. env dumps, API responses)
// containing live secrets. Kept minimal and local rather than importing
// internal/tools.ScrubCredentials — that package imports internal/providers,
// and internal/providers imports internal/providers/acp, so pulling it in
// here would create an import cycle.
var debugScrubPatterns = []*regexp.Regexp{
	regexp.MustCompile(`sk-[a-zA-Z0-9-]{20,}`),
	regexp.MustCompile(`ghp_[a-zA-Z0-9]{36}`),
	regexp.MustCompile(`AKIA[A-Z0-9]{16}`),
	regexp.MustCompile(`(?i)(api[_-]?key|token|secret|password|bearer|authorization)\s*[:=]\s*["']?\S{8,}["']?`),
	regexp.MustCompile(`(?i)(postgres|postgresql|mysql|mongodb|redis|amqp)://[^\s"']+`),
}

func scrubDebugLine(s string) string {
	for _, pat := range debugScrubPatterns {
		s = pat.ReplaceAllString(s, "[REDACTED]")
	}
	return s
}

// jsonrpcMessage is the wire format for JSON-RPC 2.0 messages.
type jsonrpcMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int64          `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonrpcError   `json:"error,omitempty"`
}

type jsonrpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *jsonrpcError) Error() string {
	return fmt.Sprintf("jsonrpc error %d: %s", e.Code, e.Message)
}

// RequestHandler handles agent→client requests (fs, terminal, permission).
// Context is derived from the connection lifetime and cancelled when the read loop exits.
type RequestHandler func(ctx context.Context, method string, params json.RawMessage) (any, error)

// NotifyHandler handles agent→client notifications (session/update).
type NotifyHandler func(method string, params json.RawMessage)

// Conn manages bidirectional JSON-RPC 2.0 over stdio pipes.
type Conn struct {
	writer  io.Writer
	reader  io.Reader
	nextID  atomic.Int64
	pending sync.Map // id → chan *jsonrpcMessage
	handler RequestHandler
	notify  NotifyHandler
	done    chan struct{}
	mu      sync.Mutex // protects writes
}

// NewConn creates a JSON-RPC connection over the given reader/writer pair.
func NewConn(writer io.Writer, reader io.Reader, handler RequestHandler, notify NotifyHandler) *Conn {
	return &Conn{
		writer:  writer,
		reader:  reader,
		handler: handler,
		notify:  notify,
		done:    make(chan struct{}),
	}
}

// Start begins the read loop goroutine that dispatches incoming messages.
func (c *Conn) Start() {
	go c.readLoop()
}

// readLoop reads newline-delimited JSON messages from stdout and dispatches them.
func (c *Conn) readLoop() {
	scanner := bufio.NewScanner(c.reader)
	scanner.Buffer(make([]byte, 0, 256*1024), 10*1024*1024) // 256KB initial, 10MB max

	for scanner.Scan() {
		line := scanner.Bytes()
		slog.Debug("acp.jsonrpc: < READ", "line", scrubDebugLine(string(line)))
		if len(line) == 0 {
			continue
		}

		var msg jsonrpcMessage
		if err := json.Unmarshal(line, &msg); err != nil {
			slog.Debug("acp.jsonrpc: skip malformed line", "error", err)
			continue
		}

		switch {
		case msg.ID != nil && msg.Method == "":
			// Response to our request — dispatch to pending caller
			if ch, ok := c.pending.LoadAndDelete(*msg.ID); ok {
				ch.(chan *jsonrpcMessage) <- &msg
			}

		case msg.ID != nil && msg.Method != "":
			// Agent→client request — handle and respond
			go c.handleRequest(&msg)

		case msg.ID == nil && msg.Method != "":
			// Notification — dispatch to handler
			if c.notify != nil {
				c.notify(msg.Method, msg.Params)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		slog.Debug("acp.jsonrpc: read loop ended", "error", err)
	}
	close(c.done)
}

// handleRequest processes an agent→client request and sends back the response.
// Uses a context derived from the connection lifetime so long-running handlers
// (e.g. waitForExit) are cancelled when the connection closes.
func (c *Conn) handleRequest(msg *jsonrpcMessage) {
	var resp jsonrpcMessage
	resp.JSONRPC = "2.0"
	resp.ID = msg.ID

	// Create a context that cancels when the connection's read loop exits.
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		select {
		case <-c.done:
			cancel()
		case <-ctx.Done():
		}
	}()
	defer cancel()

	if c.handler == nil {
		resp.Error = &jsonrpcError{Code: -32601, Message: "no handler registered"}
	} else {
		result, err := c.handler(ctx, msg.Method, msg.Params)
		if err != nil {
			resp.Error = &jsonrpcError{Code: -32000, Message: err.Error()}
		} else {
			data, marshalErr := json.Marshal(result)
			if marshalErr != nil {
				resp.Error = &jsonrpcError{Code: -32603, Message: "internal: " + marshalErr.Error()}
			} else {
				resp.Result = data
			}
		}
	}

	c.writeMessage(&resp)
}

// Call sends a JSON-RPC request and blocks until the response arrives.
func (c *Conn) Call(ctx context.Context, method string, params any, result any) error {
	id := c.nextID.Add(1)
	ch := make(chan *jsonrpcMessage, 1)
	c.pending.Store(id, ch)
	defer c.pending.Delete(id)

	paramsData, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("marshal params: %w", err)
	}

	msg := &jsonrpcMessage{
		JSONRPC: "2.0",
		ID:      &id,
		Method:  method,
		Params:  paramsData,
	}
	if err := c.writeMessage(msg); err != nil {
		return fmt.Errorf("write request: %w", err)
	}

	select {
	case resp := <-ch:
		if resp.Error != nil {
			return resp.Error
		}
		if result != nil && len(resp.Result) > 0 {
			return json.Unmarshal(resp.Result, result)
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-c.done:
		return fmt.Errorf("connection closed")
	}
}

// Notify sends a JSON-RPC notification (no response expected).
func (c *Conn) Notify(method string, params any) error {
	paramsData, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("marshal params: %w", err)
	}
	msg := &jsonrpcMessage{
		JSONRPC: "2.0",
		Method:  method,
		Params:  paramsData,
	}
	return c.writeMessage(msg)
}

// writeMessage marshals and writes a JSON-RPC message followed by a newline.
func (c *Conn) writeMessage(msg *jsonrpcMessage) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	data = append(data, '\n')

	c.mu.Lock()
	defer c.mu.Unlock()
	slog.Debug("acp.jsonrpc: > WRITE", "data", scrubDebugLine(string(data)))
	_, err = c.writer.Write(data)
	return err
}

// Done returns a channel that is closed when the read loop exits.
func (c *Conn) Done() <-chan struct{} {
	return c.done
}
