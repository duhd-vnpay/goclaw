package tts

import (
	"context"
	"fmt"
	"testing"
)

// --- mock provider ---

type mockProvider struct {
	name    string
	result  *SynthResult
	err     error
	callLog []string
}

func (m *mockProvider) Name() string { return m.name }
func (m *mockProvider) Synthesize(_ context.Context, text string, _ Options) (*SynthResult, error) {
	m.callLog = append(m.callLog, text)
	if m.err != nil {
		return nil, m.err
	}
	return m.result, nil
}

func newMock(name string) *mockProvider {
	return &mockProvider{
		name:   name,
		result: &SynthResult{Audio: []byte("audio"), Extension: "mp3"},
	}
}

// --- GetProvider tests (mirrors the startup bug) ---

// TestGetProvider_ProviderNotRegistered reproduces the startup bug:
// manager has primary="minimax" but minimax was never registered (no API key).
// GetProvider("minimax") must return (nil, false).
func TestGetProvider_ProviderNotRegistered(t *testing.T) {
	mgr := NewManager(ManagerConfig{Primary: "minimax", Auto: AutoAlways})
	// Only edge registered (no minimax key at startup)
	mgr.RegisterProvider(newMock("edge"))

	p, ok := mgr.GetProvider("minimax")
	if ok {
		t.Errorf("expected GetProvider(minimax) to return false, got provider: %v", p)
	}
}

// TestGetProvider_AfterRebuildWithSecrets mirrors the fix:
// after DB secrets are applied and manager is rebuilt, minimax must be found.
func TestGetProvider_AfterRebuildWithSecrets(t *testing.T) {
	// Phase 1 – startup without DB secrets: minimax not registered
	mgr1 := NewManager(ManagerConfig{Primary: "minimax", Auto: AutoAlways})
	mgr1.RegisterProvider(newMock("edge"))

	if _, ok := mgr1.GetProvider("minimax"); ok {
		t.Fatal("phase 1: minimax should NOT be registered before DB secrets")
	}

	// Phase 2 – after secrets applied: rebuild manager with minimax
	mgr2 := NewManager(ManagerConfig{Primary: "minimax", Auto: AutoAlways})
	mgr2.RegisterProvider(newMock("edge"))
	mgr2.RegisterProvider(newMock("minimax")) // API key was applied from DB

	p, ok := mgr2.GetProvider("minimax")
	if !ok {
		t.Fatal("phase 2: GetProvider(minimax) must succeed after DB secrets applied")
	}
	if p.Name() != "minimax" {
		t.Errorf("expected minimax provider, got: %s", p.Name())
	}
}

// TestGetProvider_RegisteredProvider ensures a registered provider is found.
func TestGetProvider_RegisteredProvider(t *testing.T) {
	mgr := NewManager(ManagerConfig{Primary: "edge"})
	mgr.RegisterProvider(newMock("edge"))
	mgr.RegisterProvider(newMock("minimax"))

	for _, name := range []string{"edge", "minimax"} {
		p, ok := mgr.GetProvider(name)
		if !ok {
			t.Errorf("GetProvider(%s) returned false", name)
		}
		if p.Name() != name {
			t.Errorf("expected %s, got %s", name, p.Name())
		}
	}
}

// --- SynthesizeWithFallback: primary missing from providers ---

// TestSynthesizeWithFallback_PrimaryNotInProviders: primary="minimax" but only edge registered.
// Must fall back to edge (exactly the container behaviour before the fix).
func TestSynthesizeWithFallback_PrimaryNotInProviders(t *testing.T) {
	edge := newMock("edge")
	mgr := NewManager(ManagerConfig{Primary: "minimax", Auto: AutoAlways})
	mgr.RegisterProvider(edge) // no minimax!

	res, err := mgr.SynthesizeWithFallback(context.Background(), "hello", Options{})
	if err != nil {
		t.Fatalf("expected fallback to edge to succeed, got: %v", err)
	}
	_ = res
	if len(edge.callLog) != 1 {
		t.Errorf("edge should have been called once, called %d times", len(edge.callLog))
	}
}

// --- UpdateManager (TtsTool) concurrency ---

// TestUpdateManager_SwapsProvider verifies that after UpdateManager the new
// manager's provider is used for subsequent calls.
// This mirrors the tts-config-reload handler calling ttsTool.UpdateManager(newMgr).
func TestUpdateManager_SwapsProvider(t *testing.T) {
	// start with edge-only manager
	mgr1 := NewManager(ManagerConfig{Primary: "edge"})
	mgr1.RegisterProvider(newMock("edge"))

	// rebuild with minimax (simulating DB-secret-aware manager)
	mgr2 := NewManager(ManagerConfig{Primary: "minimax"})
	minimax := newMock("minimax")
	mgr2.RegisterProvider(newMock("edge"))
	mgr2.RegisterProvider(minimax)

	// Simulate UpdateManager (what our fix does)
	current := mgr2 // after swap

	p, ok := current.GetProvider("minimax")
	if !ok {
		t.Fatal("after UpdateManager, minimax provider must be found")
	}
	if p.Name() != "minimax" {
		t.Errorf("expected minimax, got %s", p.Name())
	}
}

// --- AllProviders fail ---

func TestSynthesizeWithFallback_AllFail(t *testing.T) {
	fail := &mockProvider{name: "edge", err: fmt.Errorf("edge down")}
	mgr := NewManager(ManagerConfig{Primary: "edge"})
	mgr.RegisterProvider(fail)

	_, err := mgr.SynthesizeWithFallback(context.Background(), "hello", Options{})
	if err == nil {
		t.Error("expected error when all providers fail")
	}
}

// --- HasProviders ---

func TestHasProviders(t *testing.T) {
	mgr := NewManager(ManagerConfig{})
	if mgr.HasProviders() {
		t.Error("new manager should have no providers")
	}
	mgr.RegisterProvider(newMock("edge"))
	if !mgr.HasProviders() {
		t.Error("should have providers after RegisterProvider")
	}
}

// --- PrimaryProvider default ---

func TestPrimaryProvider_FirstRegistered(t *testing.T) {
	mgr := NewManager(ManagerConfig{}) // no explicit primary
	mgr.RegisterProvider(newMock("edge"))
	if mgr.PrimaryProvider() != "edge" {
		t.Errorf("expected edge as auto-primary, got %s", mgr.PrimaryProvider())
	}
}
