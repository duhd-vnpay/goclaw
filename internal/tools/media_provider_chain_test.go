package tools

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/providers"
)

// --- mock provider ---

type mockProvider struct {
	name         string
	providerType string // returned by ProviderType() for typedProvider interface
}

func (m *mockProvider) Name() string { return m.name }
func (m *mockProvider) DefaultModel() string {
	return ""
}
func (m *mockProvider) Chat(_ context.Context, _ providers.ChatRequest) (*providers.ChatResponse, error) {
	return nil, errors.New("mock: not implemented")
}
func (m *mockProvider) ChatStream(_ context.Context, _ providers.ChatRequest, _ func(providers.StreamChunk)) (*providers.ChatResponse, error) {
	return nil, errors.New("mock: not implemented")
}
func (m *mockProvider) ProviderType() string { return m.providerType }

func newTestRegistry(provs ...*mockProvider) *providers.Registry {
	r := providers.NewRegistry(nil)
	for _, p := range provs {
		r.Register(p)
	}
	return r
}

var testPriority = []string{"openai_compat", "gemini", "anthropic", "dashscope"}
var testModels = map[string]string{
	"openai_compat": "gemini-3.1-flash-image",
	"gemini":        "gemini-2.5-flash",
	"anthropic":     "",
	"dashscope":     "qwen3-vl",
}

// --- buildDefaultChain ---

func TestBuildDefaultChain_EmptyRegistry(t *testing.T) {
	r := newTestRegistry()
	chain := buildDefaultChain(context.Background(), testPriority, testModels, r)
	if len(chain) != 0 {
		t.Errorf("expected empty chain, got %d entries", len(chain))
	}
}

func TestBuildDefaultChain_MatchesByType_NotByName(t *testing.T) {
	// Provider named "aistudio-google" with type "gemini_native" → resolved to "gemini"
	r := newTestRegistry(&mockProvider{name: "aistudio-google", providerType: "gemini_native"})
	chain := buildDefaultChain(context.Background(), testPriority, testModels, r)
	if len(chain) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(chain))
	}
	if chain[0].Provider != "aistudio-google" {
		t.Errorf("expected provider aistudio-google, got %s", chain[0].Provider)
	}
	if chain[0].Model != "gemini-2.5-flash" {
		t.Errorf("expected model gemini-2.5-flash, got %s", chain[0].Model)
	}
}

func TestBuildDefaultChain_SkipsUnknownTypes(t *testing.T) {
	// minimax_native is not in testModels — should be excluded
	r := newTestRegistry(
		&mockProvider{name: "minimax", providerType: "minimax_native"},
		&mockProvider{name: "cli-proxy-api", providerType: "openai_compat"},
	)
	chain := buildDefaultChain(context.Background(), testPriority, testModels, r)
	if len(chain) != 1 {
		t.Fatalf("expected 1 entry (only cli-proxy-api), got %d", len(chain))
	}
	if chain[0].Provider != "cli-proxy-api" {
		t.Errorf("expected cli-proxy-api, got %s", chain[0].Provider)
	}
}

func TestBuildDefaultChain_RespectsTypeOrder(t *testing.T) {
	// priority: openai_compat > gemini
	r := newTestRegistry(
		&mockProvider{name: "aistudio-google", providerType: "gemini_native"},
		&mockProvider{name: "cli-proxy-api", providerType: "openai_compat"},
	)
	chain := buildDefaultChain(context.Background(), testPriority, testModels, r)
	if len(chain) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(chain))
	}
	if chain[0].Provider != "cli-proxy-api" {
		t.Errorf("first entry should be cli-proxy-api (openai_compat), got %s", chain[0].Provider)
	}
	if chain[1].Provider != "aistudio-google" {
		t.Errorf("second entry should be aistudio-google (gemini), got %s", chain[1].Provider)
	}
}

func TestBuildDefaultChain_AppliesDefaults(t *testing.T) {
	r := newTestRegistry(&mockProvider{name: "my-proxy", providerType: "openai_compat"})
	chain := buildDefaultChain(context.Background(), testPriority, testModels, r)
	if len(chain) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(chain))
	}
	if chain[0].Timeout != 120 {
		t.Errorf("expected default timeout 120, got %d", chain[0].Timeout)
	}
	if chain[0].MaxRetries != 2 {
		t.Errorf("expected default max_retries 2, got %d", chain[0].MaxRetries)
	}
}

// --- ResolveMediaProviderChain ---

func TestResolveMediaProviderChain_PerAgentOverride(t *testing.T) {
	r := newTestRegistry()
	chain := ResolveMediaProviderChain(context.Background(), "read_image",
		"my-provider", "my-model", testPriority, testModels, r)
	if len(chain) != 1 {
		t.Fatalf("expected 1 entry from per-agent override, got %d", len(chain))
	}
	if chain[0].Provider != "my-provider" || chain[0].Model != "my-model" {
		t.Errorf("unexpected entry: %+v", chain[0])
	}
}

func TestResolveMediaProviderChain_SettingsChain_FiltersByRegistry(t *testing.T) {
	// Settings has "openrouter" but registry only has "cli-proxy-api"
	r := newTestRegistry(&mockProvider{name: "cli-proxy-api", providerType: "openai_compat"})

	settings := mustMarshalChain([]MediaProviderEntry{
		{Provider: "openrouter", Model: "google/gemini-2.5-flash-image", Enabled: true},
		{Provider: "cli-proxy-api", Model: "gemini-3.1-flash-image", Enabled: true},
	})
	ctx := WithBuiltinToolSettings(context.Background(), BuiltinToolSettings{"read_image": settings})

	chain := ResolveMediaProviderChain(ctx, "read_image", "", "", testPriority, testModels, r)
	if len(chain) != 1 {
		t.Fatalf("expected 1 entry (openrouter filtered out), got %d", len(chain))
	}
	if chain[0].Provider != "cli-proxy-api" {
		t.Errorf("expected cli-proxy-api, got %s", chain[0].Provider)
	}
}

func TestResolveMediaProviderChain_SettingsChain_AllUnavailable_FallsToDefault(t *testing.T) {
	// Settings has "openrouter" only, but registry has "aistudio-google"
	r := newTestRegistry(&mockProvider{name: "aistudio-google", providerType: "gemini_native"})

	settings := mustMarshalChain([]MediaProviderEntry{
		{Provider: "openrouter", Model: "google/gemini-2.5-flash-image", Enabled: true},
	})
	ctx := WithBuiltinToolSettings(context.Background(), BuiltinToolSettings{"read_image": settings})

	chain := ResolveMediaProviderChain(ctx, "read_image", "", "", testPriority, testModels, r)
	// Should fall through to buildDefaultChain → aistudio-google
	if len(chain) != 1 {
		t.Fatalf("expected 1 entry from default chain, got %d", len(chain))
	}
	if chain[0].Provider != "aistudio-google" {
		t.Errorf("expected aistudio-google from default chain, got %s", chain[0].Provider)
	}
}

func TestResolveMediaProviderChain_NoSettings_UsesDefault(t *testing.T) {
	r := newTestRegistry(&mockProvider{name: "cli-proxy-api", providerType: "openai_compat"})
	chain := ResolveMediaProviderChain(context.Background(), "read_image", "", "", testPriority, testModels, r)
	if len(chain) != 1 || chain[0].Provider != "cli-proxy-api" {
		t.Errorf("expected cli-proxy-api from default chain, got %+v", chain)
	}
}

func TestResolveMediaProviderChain_SettingsDisabledEntries_Skipped(t *testing.T) {
	r := newTestRegistry(&mockProvider{name: "cli-proxy-api", providerType: "openai_compat"})

	settings := mustMarshalChain([]MediaProviderEntry{
		{Provider: "cli-proxy-api", Model: "gemini-3.1-flash-image", Enabled: false}, // disabled
	})
	ctx := WithBuiltinToolSettings(context.Background(), BuiltinToolSettings{"read_image": settings})

	chain := ResolveMediaProviderChain(ctx, "read_image", "", "", testPriority, testModels, r)
	// disabled entry → parseChainSettings returns empty → falls to default
	if len(chain) != 1 {
		t.Fatalf("expected 1 entry from default chain, got %d", len(chain))
	}
	if chain[0].Provider != "cli-proxy-api" {
		t.Errorf("expected cli-proxy-api, got %s", chain[0].Provider)
	}
}

// --- helpers ---

func mustMarshalChain(entries []MediaProviderEntry) []byte {
	b, err := json.Marshal(map[string]any{"providers": entries})
	if err != nil {
		panic(err)
	}
	return b
}
