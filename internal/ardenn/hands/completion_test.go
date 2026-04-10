package hands

import (
	"testing"
	"time"

	engine "github.com/nextlevelbuilder/goclaw/internal/ardenn"

	"github.com/google/uuid"
)

func TestCompletionRegistry_RegisterAndComplete(t *testing.T) {
	reg := NewCompletionRegistry()
	stepRunID := uuid.New()

	ch := reg.Register(stepRunID)

	go func() {
		time.Sleep(10 * time.Millisecond)
		reg.Complete(stepRunID, engine.HandResult{Output: "done"})
	}()

	select {
	case result := <-ch:
		if result.Output != "done" {
			t.Errorf("got output %q, want done", result.Output)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for completion")
	}
}

func TestCompletionRegistry_CompleteUnregistered(t *testing.T) {
	reg := NewCompletionRegistry()
	reg.Complete(uuid.New(), engine.HandResult{Output: "orphan"})
}

func TestCompletionRegistry_Deregister(t *testing.T) {
	reg := NewCompletionRegistry()
	stepRunID := uuid.New()

	reg.Register(stepRunID)
	reg.Deregister(stepRunID)

	reg.Complete(stepRunID, engine.HandResult{Output: "late"})
}
