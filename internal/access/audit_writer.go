package access

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// FileAccessEvent represents a single file access audit record.
type FileAccessEvent struct {
	ActorID      string
	ActorType    string
	SessionKey   string
	Action       string // "read", "write", "delete", "deny"
	Resource     string
	ResourceType string // "media", "workspace", "team", "channel"
	Source       string // "http", "tool", "channel_download"
	IPAddress    string
}

// AuditWriterConfig configures the dual-write audit writer.
type AuditWriterConfig struct {
	BufferSize    int
	FlushInterval time.Duration
	MaxBuffer     int
	DBWriter      func(ctx context.Context, batch []FileAccessEvent) error
}

// AuditWriter performs dual-write: slog (sync) + PostgreSQL (async batch).
type AuditWriter struct {
	cfg    AuditWriterConfig
	buffer []FileAccessEvent
	mu     sync.Mutex
	done   chan struct{}
	wg     sync.WaitGroup
}

// NewAuditWriter creates and starts the audit writer background flusher.
func NewAuditWriter(cfg AuditWriterConfig) *AuditWriter {
	if cfg.BufferSize <= 0 {
		cfg.BufferSize = 100
	}
	if cfg.FlushInterval <= 0 {
		cfg.FlushInterval = 5 * time.Second
	}
	if cfg.MaxBuffer <= 0 {
		cfg.MaxBuffer = 1000
	}
	w := &AuditWriter{
		cfg:    cfg,
		buffer: make([]FileAccessEvent, 0, cfg.BufferSize),
		done:   make(chan struct{}),
	}
	w.wg.Add(1)
	go w.flusher()
	return w
}

// Log records a file access event. Always writes to slog synchronously.
// Buffers for async PostgreSQL batch insert.
func (w *AuditWriter) Log(_ context.Context, event FileAccessEvent) {
	// 1. slog — always, synchronous
	slog.Info("file.access",
		"actor_id", event.ActorID,
		"actor_type", event.ActorType,
		"action", event.Action,
		"resource", event.Resource,
		"resource_type", event.ResourceType,
		"source", event.Source,
		"ip_address", event.IPAddress,
	)

	// 2. Buffer for PostgreSQL batch
	w.mu.Lock()
	defer w.mu.Unlock()

	if len(w.buffer) >= w.cfg.MaxBuffer {
		slog.Warn("audit: buffer full, dropping oldest", "max_buffer", w.cfg.MaxBuffer)
		w.buffer = w.buffer[1:]
	}
	w.buffer = append(w.buffer, event)

	if len(w.buffer) >= w.cfg.BufferSize {
		w.flushLocked()
	}
}

func (w *AuditWriter) flusher() {
	defer w.wg.Done()
	ticker := time.NewTicker(w.cfg.FlushInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			w.mu.Lock()
			w.flushLocked()
			w.mu.Unlock()
		case <-w.done:
			w.mu.Lock()
			w.flushLocked()
			w.mu.Unlock()
			return
		}
	}
}

func (w *AuditWriter) flushLocked() {
	if len(w.buffer) == 0 {
		return
	}
	batch := make([]FileAccessEvent, len(w.buffer))
	copy(batch, w.buffer)
	w.buffer = w.buffer[:0]

	if w.cfg.DBWriter != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := w.cfg.DBWriter(ctx, batch); err != nil {
			slog.Error("audit: db write failed", "error", err, "count", len(batch))
		}
	}
}

// Close flushes remaining events and stops the background flusher.
func (w *AuditWriter) Close() {
	close(w.done)
	w.wg.Wait()
}
