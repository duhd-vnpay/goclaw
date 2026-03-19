package access

import (
	"context"
	"testing"
	"time"
)

func TestAuditWriter_LogAndFlush(t *testing.T) {
	var flushed []FileAccessEvent
	w := NewAuditWriter(AuditWriterConfig{
		BufferSize:    10,
		FlushInterval: 100 * time.Millisecond,
		MaxBuffer:     100,
		DBWriter: func(ctx context.Context, batch []FileAccessEvent) error {
			flushed = append(flushed, batch...)
			return nil
		},
	})
	defer w.Close()

	w.Log(context.Background(), FileAccessEvent{
		ActorID:  "user1",
		Action:   "read",
		Resource: "/media/test.jpg",
	})

	time.Sleep(200 * time.Millisecond)

	if len(flushed) != 1 {
		t.Errorf("flushed %d events, want 1", len(flushed))
	}
}

func TestAuditWriter_BatchFlush(t *testing.T) {
	var flushed []FileAccessEvent
	w := NewAuditWriter(AuditWriterConfig{
		BufferSize:    5,
		FlushInterval: 10 * time.Second,
		MaxBuffer:     100,
		DBWriter: func(ctx context.Context, batch []FileAccessEvent) error {
			flushed = append(flushed, batch...)
			return nil
		},
	})
	defer w.Close()

	for i := 0; i < 5; i++ {
		w.Log(context.Background(), FileAccessEvent{
			ActorID:  "user1",
			Action:   "read",
			Resource: "/media/test.jpg",
		})
	}

	time.Sleep(100 * time.Millisecond)

	if len(flushed) != 5 {
		t.Errorf("flushed %d events, want 5", len(flushed))
	}
}

func TestAuditWriter_DropOldestWhenFull(t *testing.T) {
	var flushed []FileAccessEvent
	w := NewAuditWriter(AuditWriterConfig{
		BufferSize:    100,
		FlushInterval: 100 * time.Millisecond,
		MaxBuffer:     3,
		DBWriter: func(ctx context.Context, batch []FileAccessEvent) error {
			flushed = append(flushed, batch...)
			return nil
		},
	})
	defer w.Close()

	for i := 0; i < 5; i++ {
		w.Log(context.Background(), FileAccessEvent{
			ActorID:  "user1",
			Action:   "read",
			Resource: "/media/test.jpg",
		})
	}

	time.Sleep(200 * time.Millisecond)

	// MaxBuffer=3, so oldest 2 should be dropped
	if len(flushed) != 3 {
		t.Errorf("flushed %d events, want 3 (max buffer)", len(flushed))
	}
}
