package acp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"
)

// ShimHandle is the subset of *mcp_shim.Server the ACP layer needs to wire
// the in-process MCP shim into session lifecycle. Kept as an interface so
// the acp package does not import mcp_shim directly (mcp_shim already
// depends on internal/tools and internal/mcp; an acp → mcp_shim import
// risks cycling once mcp_shim grows back-references). The wiring layer
// (acp_provider.go, in a later task) injects the concrete *mcp_shim.Server.
//
// RegisterSession accepts any rather than mcp_shim.SessionEntry to keep
// this surface cycle-free; production wiring asserts the concrete type at
// the call site (see Task 7 in docs/superpowers/plans/2026-06-10-phase4-acp-mcp-bridge.md).
type ShimHandle interface {
	URL() string
	SessionURL(sid string) string
	RegisterSession(entry any)
	UnregisterSession(sid string)
}

// ACPProcess represents a running ACP agent subprocess.
// One process is shared across all sessions — each goclaw conversation
// creates its own ACP session (identified by session ID) on this process.
type ACPProcess struct {
	cmd  *exec.Cmd
	conn *Conn

	agentCaps  AgentCaps
	workDir    string
	lastActive time.Time
	inUse      atomic.Int32 // >0 means at least one prompt is active — reaper must skip
	mu         sync.Mutex
	ctx        context.Context
	cancel     context.CancelFunc
	exited     chan struct{} // closed when process exits

	// updateFns routes session/update notifications to the correct active prompt.
	updateFns map[string]func(SessionUpdate)
	updateMu  sync.Mutex
}

// AgentCaps returns the capability flags reported by the agent during Initialize.
func (p *ACPProcess) AgentCaps() AgentCaps {
	return p.agentCaps
}

// registerUpdateFn registers a callback for session/update notifications on sessionID.
func (p *ACPProcess) registerUpdateFn(sid string, fn func(SessionUpdate)) {
	p.updateMu.Lock()
	defer p.updateMu.Unlock()
	if p.updateFns == nil {
		p.updateFns = make(map[string]func(SessionUpdate))
	}
	p.updateFns[sid] = fn
}

// unregisterUpdateFn removes the callback for sessionID after a Prompt completes.
func (p *ACPProcess) unregisterUpdateFn(sid string) {
	p.updateMu.Lock()
	defer p.updateMu.Unlock()
	delete(p.updateFns, sid)
}

// dispatchUpdate routes a session/update notification to the registered callback.
// It also performs Gemini ACP protocol mapping: the "agent_message_chunk" update type
// carries content in Update.Content rather than the standard Message field; this is
// normalized here so all callers receive a consistent SessionUpdate.
func (p *ACPProcess) dispatchUpdate(update SessionUpdate) {
	// Gemini protocol mapping: agent_message_chunk → Message
	if update.Update.SessionUpdate == "agent_message_chunk" && len(update.Update.Content) > 0 {
		if update.Message == nil {
			update.Message = &MessageUpdate{Role: "assistant"}
		}
		// Content may arrive as a single object {"type":"text","text":"..."} or an array
		var single struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}
		if err := json.Unmarshal(update.Update.Content, &single); err == nil && single.Type != "" {
			update.Message.Content = append(update.Message.Content, ContentBlock{
				Type: single.Type,
				Text: single.Text,
			})
		} else {
			var arr []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			}
			if err := json.Unmarshal(update.Update.Content, &arr); err == nil {
				for _, c := range arr {
					update.Message.Content = append(update.Message.Content, ContentBlock{
						Type: c.Type,
						Text: c.Text,
					})
				}
			}
		}
	}

	p.updateMu.Lock()
	fn, ok := p.updateFns[update.SessionID]
	p.updateMu.Unlock()
	if !ok {
		slog.Debug("acp: session/update with no registered callback", "sid", update.SessionID)
		return
	}
	if fn != nil {
		fn(update)
	}
}

// ProcessPool manages a pool of ACP agent subprocesses.
// Typically a single shared process is used (poolKey = binary identifier),
// and multiple ACP sessions are multiplexed over it.
type ProcessPool struct {
	processes   sync.Map // poolKey → *ACPProcess
	spawnMu     sync.Map // poolKey → *sync.Mutex — prevents concurrent spawn
	agentBinary string
	agentArgs   []string
	workDir     string
	idleTTL     time.Duration
	mu          sync.RWMutex // protects toolHandler + shim
	toolHandler RequestHandler
	// shim is the in-process MCP shim that ACP sub-sessions advertise to
	// claude-agent-acp via NewSessionRequest.McpServers. Nil → no shim
	// wiring (Phase 3 behavior); session/new still works, just without
	// per-session MCP tool exposure. Set via SetShim before any
	// NewSessionWithShim / LoadSessionWithShim call.
	shim      ShimHandle
	done      chan struct{}
	closeOnce sync.Once
}

// NewProcessPool creates a pool that spawns ACP agents as subprocesses.
func NewProcessPool(binary string, args []string, workDir string, idleTTL time.Duration) *ProcessPool {
	pp := &ProcessPool{
		agentBinary: binary,
		agentArgs:   args,
		workDir:     workDir,
		idleTTL:     idleTTL,
		done:        make(chan struct{}),
	}
	go pp.reapLoop()
	return pp
}

// SetToolHandler sets the agent→client request handler (tool bridge).
// Must be called before any GetOrSpawn calls.
func (pp *ProcessPool) SetToolHandler(h RequestHandler) {
	pp.mu.Lock()
	defer pp.mu.Unlock()
	pp.toolHandler = h
}

// getToolHandler returns the current tool handler (thread-safe).
func (pp *ProcessPool) getToolHandler() RequestHandler {
	pp.mu.RLock()
	defer pp.mu.RUnlock()
	return pp.toolHandler
}

// SetShim wires the in-process MCP shim into the pool. Safe to call before
// any GetOrSpawn or NewSessionWithShim. A nil handle disables shim wiring
// (subsequent NewSessionWithShim falls back to Phase 3 empty-mcpServers
// behavior).
func (pp *ProcessPool) SetShim(h ShimHandle) {
	pp.mu.Lock()
	defer pp.mu.Unlock()
	pp.shim = h
}

// getShim returns the current shim handle (thread-safe). May return nil.
func (pp *ProcessPool) getShim() ShimHandle {
	pp.mu.RLock()
	defer pp.mu.RUnlock()
	return pp.shim
}

// NewSessionWithShim creates an ACP session, passing the pool's wired shim
// handle through to the session/new RPC. The optional register callback is
// invoked with the resolved session ID so the caller can register the
// SessionEntry in the shim under the correct sid (Task 7 will populate
// McpServers using shim.SessionURL). For now this is a thin wrapper that
// preserves Phase 3 behavior — McpServers stays empty — and lays the
// wiring surface for the populate step.
//
// Callers that don't need shim wiring should keep using proc.NewSession.
func (pp *ProcessPool) NewSessionWithShim(ctx context.Context, proc *ACPProcess, register func(sid string)) (string, error) {
	return proc.newSessionImpl(ctx, pp.getShim(), register)
}

// LoadSessionWithShim is the shim-aware counterpart to ACPProcess.LoadSession,
// mirroring NewSessionWithShim for the session/load path. Used after a
// process respawn to re-attach an existing session ID with shim wiring
// preserved. McpServers population is deferred to Task 7 — for now this
// preserves Phase 3 behavior (empty McpServers) regardless of shim presence.
func (pp *ProcessPool) LoadSessionWithShim(ctx context.Context, proc *ACPProcess, sessionID string, register func(sid string)) (string, error) {
	return proc.loadSessionImpl(ctx, sessionID, pp.getShim(), register)
}

// GetOrSpawn returns an existing process for the pool key or spawns a new one.
// Uses per-key mutex to prevent concurrent spawn for the same key.
func (pp *ProcessPool) GetOrSpawn(ctx context.Context, poolKey string) (*ACPProcess, error) {
	actual, _ := pp.spawnMu.LoadOrStore(poolKey, &sync.Mutex{})
	mu := actual.(*sync.Mutex)
	mu.Lock()
	defer mu.Unlock()

	if val, ok := pp.processes.Load(poolKey); ok {
		proc := val.(*ACPProcess)
		select {
		case <-proc.exited:
			pp.processes.Delete(poolKey)
			slog.Info("acp: respawning crashed process", "pool_key", poolKey)
		default:
			return proc, nil
		}
	}
	return pp.spawn(ctx, poolKey)
}

// spawn creates a new ACP subprocess and performs the ACP initialize handshake.
// Session creation (session/new) is NOT done here — the provider handles that
// per-conversation via NewSession or LoadSession.
func (pp *ProcessPool) spawn(ctx context.Context, poolKey string) (*ACPProcess, error) {
	procCtx, cancel := context.WithCancel(context.Background())

	cmd := exec.CommandContext(procCtx, pp.agentBinary, pp.agentArgs...)
	cmd.Dir = pp.workDir
	cmd.Env = acpSubprocessEnv(os.Environ())
	cmd.SysProcAttr = sysProcAttr()

	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		slog.Error("acp: spawn failed", "pool_key", poolKey, "stage", "stdin_pipe", "error", err)
		cancel()
		return nil, fmt.Errorf("acp: stdin pipe: %w", err)
	}
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		slog.Error("acp: spawn failed", "pool_key", poolKey, "stage", "stdout_pipe", "error", err)
		cancel()
		return nil, fmt.Errorf("acp: stdout pipe: %w", err)
	}
	cmd.Stderr = &limitedWriter{max: 4096}

	slog.Info("acp: starting subprocess", "pool_key", poolKey, "binary", pp.agentBinary, "args", pp.agentArgs)
	if err := cmd.Start(); err != nil {
		slog.Error("acp: spawn failed", "pool_key", poolKey, "stage", "start", "binary", pp.agentBinary, "error", err)
		cancel()
		return nil, fmt.Errorf("acp: start %s: %w", pp.agentBinary, err)
	}

	proc := &ACPProcess{
		cmd:        cmd,
		lastActive: time.Now(),
		ctx:        procCtx,
		cancel:     cancel,
		exited:     make(chan struct{}),
		workDir:    pp.workDir,
	}

	// Notification handler: log all notifications and dispatch session/update to callers
	notifyHandler := func(method string, params json.RawMessage) {
		slog.Info("acp: notification received", "method", method)
		slog.Debug("acp: notification params", "method", method, "params", string(params))
		if method == "session/update" {
			var update SessionUpdate
			if err := json.Unmarshal(params, &update); err != nil {
				slog.Warn("acp: session/update parse failed", "error", err)
				return
			}
			proc.dispatchUpdate(update)
		}
	}

	proc.conn = NewConn(stdinPipe, stdoutPipe, pp.getToolHandler(), notifyHandler)
	proc.conn.Start()

	stderrWriter := cmd.Stderr.(*limitedWriter)
	go func() {
		_ = cmd.Wait()
		if s := stderrWriter.String(); s != "" {
			slog.Debug("acp: process stderr", "pool_key", poolKey, "stderr", s)
		}
		close(proc.exited)
	}()

	slog.Info("acp: performing handshake (initialize)", "pool_key", poolKey)
	if err := proc.Initialize(ctx); err != nil {
		slog.Error("acp: spawn failed", "pool_key", poolKey, "stage", "handshake", "error", err)
		cancel()
		return nil, err
	}

	pp.processes.Store(poolKey, proc)
	slog.Info("acp: process spawned", "pool_key", poolKey, "binary", pp.agentBinary)
	return proc, nil
}

// reapLoop periodically checks for idle processes and kills them.
func (pp *ProcessPool) reapLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			pp.processes.Range(func(key, value any) bool {
				proc := value.(*ACPProcess)
				if proc.inUse.Load() > 0 {
					return true
				}
				proc.mu.Lock()
				idle := time.Since(proc.lastActive) > pp.idleTTL
				proc.mu.Unlock()
				if idle {
					slog.Info("acp: reaping idle process", "pool_key", key)
					proc.cancel()
					pp.processes.Delete(key)
				}
				return true
			})
		case <-pp.done:
			return
		}
	}
}

// processCloseTimeout is the per-process max wait during ProcessPool.Close.
// Exposed as a package var so tests can shorten it.
var processCloseTimeout = 5 * time.Second

// Close shuts down all processes gracefully.
func (pp *ProcessPool) Close() error {
	pp.closeOnce.Do(func() {
		close(pp.done)
		pp.processes.Range(func(key, value any) bool {
			proc := value.(*ACPProcess)
			proc.cancel()
			select {
			case <-proc.exited:
			case <-time.After(processCloseTimeout):
				slog.Warn("acp: process did not exit in time", "pool_key", key)
			}
			pp.processes.Delete(key)
			return true
		})
	})
	return nil
}
