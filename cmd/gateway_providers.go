package cmd

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/bus"
	"github.com/nextlevelbuilder/goclaw/internal/config"
	"github.com/nextlevelbuilder/goclaw/internal/oauth"
	"github.com/nextlevelbuilder/goclaw/internal/providers"
	"github.com/nextlevelbuilder/goclaw/internal/providers/acp"
	"github.com/nextlevelbuilder/goclaw/internal/providers/acp/mcp_shim"
	"github.com/nextlevelbuilder/goclaw/internal/store"
	"github.com/nextlevelbuilder/goclaw/internal/tools"
)

// ACPDeps bundles the optional in-process MCP shim handle + resolver passed
// down to ACP provider construction (Phase 4). When zero-valued the ACP
// provider falls back to Phase 3 behavior — empty mcpServers, no shim-side
// session state. Plumbed through registerProviders and registerProvidersFromDB
// so the same struct shape works for config + DB providers.
type ACPDeps struct {
	Shim      acp.ShimHandle
	Resolver  *mcp_shim.Resolver
	CtxReader providers.ACPContextReader
	// CfgReader honors the acp.shim.enabled kill-switch (Task 9). Nil → flag
	// effectively absent → shim stays on (default-OPEN).
	CfgReader providers.ACPConfigReader
}

// toolsCtxReader bridges providers.ACPContextReader to the tools.Tool*FromCtx
// accessors. Lives in cmd/ so internal/providers stays free of the
// internal/tools import (which would create a cycle).
type toolsCtxReader struct{}

func (toolsCtxReader) ReadRouting(ctx context.Context) providers.ACPRoutingContext {
	return providers.ACPRoutingContext{
		AgentKey:   tools.ToolAgentKeyFromCtx(ctx),
		ChannelID:  tools.ToolChannelFromCtx(ctx),
		ChatID:     tools.ToolChatIDFromCtx(ctx),
		PeerKind:   tools.ToolPeerKindFromCtx(ctx),
		SessionKey: tools.ToolSessionKeyFromCtx(ctx),
	}
}

// systemConfigReader adapts store.SystemConfigStore to
// providers.ACPConfigReader, binding the tenant on ctx before delegating.
// Defined in cmd/ so internal/providers stays free of the internal/store
// import (store → providers cycle).
//
// tenantID is the tenant the ACPProvider is registered for. For a global
// ACP provider (config-file form, no per-tenant binding) the caller plugs
// in store.MasterTenantID so the system_configs Get hits the master row
// the parent-repo seed (122-acp-shim-feature-flag.sql) populates.
type systemConfigReader struct {
	src      store.SystemConfigStore
	tenantID uuid.UUID
}

func (r systemConfigReader) GetSystemConfig(ctx context.Context, key string) (string, error) {
	if r.src == nil {
		return "", nil
	}
	if store.TenantIDFromContext(ctx) == uuid.Nil {
		ctx = store.WithTenantID(ctx, r.tenantID)
	}
	return r.src.Get(ctx, key)
}

// mcpAccessAdapter bridges store.MCPServerStore.ListAccessible to the
// mcp_shim.MCPAccessLookup interface. Defined in cmd/ so the shim package
// stays free of the internal/store import (cycle: store → providers →
// providers/acp/mcp_shim → store).
type mcpAccessAdapter struct {
	src store.MCPServerStore
}

func (a mcpAccessAdapter) ListAccessibleForAgent(ctx context.Context, agentID uuid.UUID) ([]mcp_shim.MCPAccessInfoView, error) {
	infos, err := a.src.ListAccessible(ctx, agentID, "")
	if err != nil {
		return nil, err
	}
	out := make([]mcp_shim.MCPAccessInfoView, 0, len(infos))
	for _, i := range infos {
		out = append(out, mcp_shim.MCPAccessInfoView{
			ServerName: i.Server.Name,
			ToolAllow:  i.ToolAllow,
			ToolDeny:   i.ToolDeny,
		})
	}
	return out, nil
}

// loopbackAddr normalizes a gateway address for local connections.
// CLI processes on the same machine can't connect to 0.0.0.0 on some OSes.
func loopbackAddr(host string, port int) string {
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, strconv.Itoa(port))
}

func registerProviders(registry *providers.Registry, cfg *config.Config, modelReg providers.ModelRegistry, acpDeps ACPDeps) {
	if cfg.Providers.Anthropic.APIKey != "" {
		registry.Register(providers.NewAnthropicProvider(cfg.Providers.Anthropic.APIKey,
			providers.WithAnthropicBaseURL(cfg.Providers.Anthropic.APIBase),
			providers.WithAnthropicRegistry(modelReg)))
		slog.Info("registered provider", "name", "anthropic")
	}

	if cfg.Providers.OpenAI.APIKey != "" {
		registry.Register(providers.NewOpenAIProvider("openai", cfg.Providers.OpenAI.APIKey, cfg.Providers.OpenAI.APIBase, "gpt-4o").
			WithRegistry(modelReg))
		slog.Info("registered provider", "name", "openai")
	}

	if cfg.Providers.OpenRouter.APIKey != "" {
		orProv := providers.NewOpenAIProvider("openrouter", cfg.Providers.OpenRouter.APIKey, "https://openrouter.ai/api/v1", "anthropic/claude-sonnet-4-5-20250929")
		orProv.WithSiteInfo("https://goclaw.sh", "GoClaw")
		registry.Register(orProv)
		slog.Info("registered provider", "name", "openrouter")
	}

	if cfg.Providers.Groq.APIKey != "" {
		registry.Register(providers.NewOpenAIProvider("groq", cfg.Providers.Groq.APIKey, "https://api.groq.com/openai/v1", "llama-3.3-70b-versatile"))
		slog.Info("registered provider", "name", "groq")
	}

	if cfg.Providers.DeepSeek.APIKey != "" {
		registry.Register(providers.NewOpenAIProvider("deepseek", cfg.Providers.DeepSeek.APIKey, "https://api.deepseek.com/v1", "deepseek-chat"))
		slog.Info("registered provider", "name", "deepseek")
	}

	if cfg.Providers.Gemini.APIKey != "" {
		registry.Register(providers.NewOpenAIProvider("gemini", cfg.Providers.Gemini.APIKey, "https://generativelanguage.googleapis.com/v1beta/openai", "gemini-2.0-flash"))
		slog.Info("registered provider", "name", "gemini")
	}

	if cfg.Providers.Mistral.APIKey != "" {
		registry.Register(providers.NewOpenAIProvider("mistral", cfg.Providers.Mistral.APIKey, "https://api.mistral.ai/v1", "mistral-large-latest"))
		slog.Info("registered provider", "name", "mistral")
	}

	if cfg.Providers.XAI.APIKey != "" {
		registry.Register(providers.NewOpenAIProvider("xai", cfg.Providers.XAI.APIKey, "https://api.x.ai/v1", "grok-3-mini"))
		slog.Info("registered provider", "name", "xai")
	}

	if cfg.Providers.MiniMax.APIKey != "" {
		registry.Register(providers.NewOpenAIProvider("minimax", cfg.Providers.MiniMax.APIKey, "https://api.minimax.io/v1", "MiniMax-M2.5").
			WithChatPath("/text/chatcompletion_v2"))
		slog.Info("registered provider", "name", "minimax")
	}

	if cfg.Providers.Cohere.APIKey != "" {
		registry.Register(providers.NewOpenAIProvider("cohere", cfg.Providers.Cohere.APIKey, "https://api.cohere.ai/compatibility/v1", "command-a"))
		slog.Info("registered provider", "name", "cohere")
	}

	if cfg.Providers.Perplexity.APIKey != "" {
		registry.Register(providers.NewOpenAIProvider("perplexity", cfg.Providers.Perplexity.APIKey, "https://api.perplexity.ai", "sonar-pro"))
		slog.Info("registered provider", "name", "perplexity")
	}

	if cfg.Providers.DashScope.APIKey != "" {
		registry.Register(providers.NewDashScopeProvider("dashscope", cfg.Providers.DashScope.APIKey, cfg.Providers.DashScope.APIBase, "qwen3-max"))
		slog.Info("registered provider", "name", "dashscope")
	}

	if cfg.Providers.Bailian.APIKey != "" {
		base := cfg.Providers.Bailian.APIBase
		if base == "" {
			base = "https://coding-intl.dashscope.aliyuncs.com/v1"
		}
		registry.Register(providers.NewOpenAIProvider("bailian", cfg.Providers.Bailian.APIKey, base, "qwen3.5-plus"))
		slog.Info("registered provider", "name", "bailian")
	}

	if cfg.Providers.Zai.APIKey != "" {
		base := cfg.Providers.Zai.APIBase
		if base == "" {
			base = "https://api.z.ai/api/paas/v4"
		}
		registry.Register(providers.NewOpenAIProvider("zai", cfg.Providers.Zai.APIKey, base, "glm-5"))
		slog.Info("registered provider", "name", "zai")
	}

	if cfg.Providers.ZaiCoding.APIKey != "" {
		base := cfg.Providers.ZaiCoding.APIBase
		if base == "" {
			base = "https://api.z.ai/api/coding/paas/v4"
		}
		registry.Register(providers.NewOpenAIProvider("zai-coding", cfg.Providers.ZaiCoding.APIKey, base, "glm-5"))
		slog.Info("registered provider", "name", "zai-coding")
	}

	// Local / self-hosted Ollama — gated on Host, no API key required.
	// Ollama's OpenAI-compat endpoint accepts any non-empty Bearer value.
	if cfg.Providers.Ollama.Host != "" {
		host := cfg.Providers.Ollama.Host
		registry.Register(providers.NewOpenAIProvider("ollama", "ollama", host+"/v1", "llama3.3"))
		slog.Info("registered provider", "name", "ollama")
	}

	// Ollama Cloud — API key required (generate at ollama.com/settings/keys).
	if cfg.Providers.OllamaCloud.APIKey != "" {
		base := cfg.Providers.OllamaCloud.APIBase
		if base == "" {
			base = "https://ollama.com/v1"
		}
		registry.Register(providers.NewOpenAIProvider("ollama-cloud", cfg.Providers.OllamaCloud.APIKey, base, "llama3.3"))
		slog.Info("registered provider", "name", "ollama-cloud")
	}

	// Novita AI — OpenAI-compatible endpoint.
	if cfg.Providers.Novita.APIKey != "" {
		base := cfg.Providers.Novita.APIBase
		if base == "" {
			base = store.NovitaDefaultAPIBase
		}
		registry.Register(providers.NewOpenAIProvider("novita", cfg.Providers.Novita.APIKey, base, store.NovitaDefaultModel))
		slog.Info("registered provider", "name", "novita")
	}

	// BytePlus ModelArk — OpenAI-compatible (standard Bearer auth).
	if cfg.Providers.BytePlus.APIKey != "" {
		base := cfg.Providers.BytePlus.APIBase
		if base == "" {
			base = store.BytePlusDefaultAPIBase
		}
		prov := providers.NewOpenAIProvider("byteplus", cfg.Providers.BytePlus.APIKey, base, store.BytePlusDefaultModel)
		prov.WithProviderType(store.ProviderBytePlus)
		registry.Register(prov)
		slog.Info("registered provider", "name", "byteplus")
	}

	// BytePlus ModelArk Coding Plan — separate endpoint for developer tools quota.
	if cfg.Providers.BytePlusCoding.APIKey != "" {
		base := cfg.Providers.BytePlusCoding.APIBase
		if base == "" {
			base = store.BytePlusCodingDefaultAPIBase
		}
		prov := providers.NewOpenAIProvider("byteplus-coding", cfg.Providers.BytePlusCoding.APIKey, base, store.BytePlusDefaultModel)
		prov.WithProviderType(store.ProviderBytePlusCoding)
		registry.Register(prov)
		slog.Info("registered provider", "name", "byteplus-coding")
	}

	// Google Cloud Vertex AI — OAuth2 service account or Application Default Credentials.
	// Registers when project_id + region are set. Credential sources (priority order):
	// inline JSON (APIKey) → file path (CredentialsFile) → ADC.
	if cfg.Providers.Vertex.ProjectID != "" && cfg.Providers.Vertex.Region != "" {
		vcfg := providers.VertexConfig{
			Name:            "vertex",
			CredentialsJSON: cfg.Providers.Vertex.APIKey,
			CredentialsFile: cfg.Providers.Vertex.CredentialsFile,
			ProjectID:       cfg.Providers.Vertex.ProjectID,
			Region:          cfg.Providers.Vertex.Region,
			DefaultModel:    cfg.Providers.Vertex.Model,
		}
		prov, err := providers.NewVertexProviderWithTimeout(vcfg)
		if err != nil {
			slog.Warn("vertex: initialization failed", "error", err)
		} else {
			registry.Register(prov)
			slog.Info("registered provider", "name", "vertex", "region", cfg.Providers.Vertex.Region, "project", cfg.Providers.Vertex.ProjectID)
		}
	}

	// Claude CLI provider (subscription-based, no API key needed)
	if cfg.Providers.ClaudeCLI.CLIPath != "" {
		cliPath := cfg.Providers.ClaudeCLI.CLIPath
		var opts []providers.ClaudeCLIOption
		if cfg.Providers.ClaudeCLI.Model != "" {
			opts = append(opts, providers.WithClaudeCLIModel(cfg.Providers.ClaudeCLI.Model))
		}
		if cfg.Providers.ClaudeCLI.BaseWorkDir != "" {
			opts = append(opts, providers.WithClaudeCLIWorkDir(cfg.Providers.ClaudeCLI.BaseWorkDir))
		}
		if cfg.Providers.ClaudeCLI.PermMode != "" {
			opts = append(opts, providers.WithClaudeCLIPermMode(cfg.Providers.ClaudeCLI.PermMode))
		}
		// Build per-session MCP config: external MCP servers + GoClaw bridge
		gatewayAddr := loopbackAddr(cfg.Gateway.Host, cfg.Gateway.Port)
		mcpData := providers.BuildCLIMCPConfigData(cfg.Tools.McpServers, gatewayAddr, cfg.Gateway.Token)
		opts = append(opts, providers.WithClaudeCLIMCPConfigData(mcpData))
		// Enable GoClaw security hooks (shell deny patterns, path restrictions)
		opts = append(opts, providers.WithClaudeCLISecurityHooks(
			cfg.Providers.ClaudeCLI.BaseWorkDir, true))
		registry.Register(providers.NewClaudeCLIProvider(cliPath, opts...))
		slog.Info("registered provider", "name", "claude-cli")
	}

	// ACP provider (config-based) — orchestrates any ACP-compatible agent binary.
	// Only register here if the shim deps are already wired; otherwise the
	// caller will invoke registerACPFromConfig directly after shim startup
	// so per-session McpServers can be populated (Phase 4).
	if cfg.Providers.ACP.Binary != "" && (acpDeps.Shim == nil) {
		// Shim not yet available at this call site — defer to post-shim wiring
		// (gateway.go calls registerACPFromConfig again with full deps).
		slog.Info("acp: deferring config registration until shim is ready")
	} else if cfg.Providers.ACP.Binary != "" {
		registerACPFromConfig(registry, cfg.Providers.ACP, acpDeps)
	}
}

// buildMCPServerLookup creates an MCPServerLookup from an MCPServerStore.
// Returns nil if mcpStore is nil.
func buildMCPServerLookup(mcpStore store.MCPServerStore) providers.MCPServerLookup {
	if mcpStore == nil {
		return nil
	}
	return func(ctx context.Context, agentID string) []providers.MCPServerEntry {
		aid, err := uuid.Parse(agentID)
		if err != nil {
			return nil
		}
		accessible, err := mcpStore.ListAccessible(ctx, aid, "")
		if err != nil {
			slog.Warn("claude-cli: failed to list agent MCP servers", "agent_id", agentID, "error", err)
			return nil
		}
		var entries []providers.MCPServerEntry
		for _, info := range accessible {
			srv := info.Server
			if !srv.Enabled {
				continue
			}
			entry := providers.MCPServerEntry{
				Name:      srv.Name,
				Transport: srv.Transport,
				Command:   srv.Command,
				URL:       srv.URL,
				Args:      jsonToStringSlice(srv.Args),
				Headers:   jsonToStringMap(srv.Headers),
				Env:       jsonToStringMap(srv.Env),
			}
			entries = append(entries, entry)
		}
		return entries
	}
}

// jsonToStringSlice converts a json.RawMessage to []string.
func jsonToStringSlice(data json.RawMessage) []string {
	if len(data) == 0 {
		return nil
	}
	var result []string
	if err := json.Unmarshal(data, &result); err != nil {
		return nil
	}
	return result
}

// jsonToStringMap converts a json.RawMessage to map[string]string.
func jsonToStringMap(data json.RawMessage) map[string]string {
	if len(data) == 0 {
		return nil
	}
	var result map[string]string
	if err := json.Unmarshal(data, &result); err != nil {
		return nil
	}
	return result
}

// registerProvidersFromDB loads providers from Postgres and registers them.
// DB providers are registered after config providers, so they take precedence (overwrite).
// gatewayAddr is used to inject GoClaw MCP bridge for Claude CLI providers.
// mcpStore is optional; when provided, per-agent MCP servers are injected into CLI config.
// cfg provides fallback api_base values from config/env when DB providers have none set.
func registerProvidersFromDB(registry *providers.Registry, provStore store.ProviderStore, secretStore store.ConfigSecretsStore, gatewayAddr, gatewayToken string, mcpStore store.MCPServerStore, cfg *config.Config, modelReg providers.ModelRegistry, acpDeps ACPDeps) {
	dbProviders, err := provStore.ListAllProviders(context.Background())
	if err != nil {
		slog.Warn("failed to load providers from DB", "error", err)
		return
	}
	for _, p := range dbProviders {
		// Claude CLI doesn't need API key
		if !p.Enabled {
			continue
		}
		if p.ProviderType == store.ProviderClaudeCLI {
			cliPath := p.APIBase // reuse APIBase field for CLI path
			if cliPath == "" {
				cliPath = "claude"
			}
			// Validate: only accept "claude" or absolute path
			if cliPath != "claude" && !filepath.IsAbs(cliPath) {
				slog.Warn("security.claude_cli: invalid path from DB, using default", "path", cliPath)
				cliPath = "claude"
			}
			if _, err := exec.LookPath(cliPath); err != nil {
				slog.Warn("claude-cli: binary not found, skipping", "path", cliPath, "error", err)
				continue
			}
			var cliOpts []providers.ClaudeCLIOption
			cliOpts = append(cliOpts, providers.WithClaudeCLIName(p.Name))
			cliOpts = append(cliOpts, providers.WithClaudeCLISecurityHooks("", true))
			if gatewayAddr != "" {
				mcpData := providers.BuildCLIMCPConfigData(nil, gatewayAddr, gatewayToken)
				mcpData.AgentMCPLookup = buildMCPServerLookup(mcpStore)
				cliOpts = append(cliOpts, providers.WithClaudeCLIMCPConfigData(mcpData))
			}
			registry.RegisterForTenant(p.TenantID, providers.NewClaudeCLIProvider(cliPath, cliOpts...))
			slog.Info("registered provider from DB", "name", p.Name)
			continue
		}
		// ACP provider — no API key needed (agents manage their own auth).
		if p.ProviderType == store.ProviderACP {
			registerACPFromDB(registry, p, acpDeps)
			continue
		}
		// Local Ollama requires no API key — handle before the key guard (same pattern as ClaudeCLI).
		// api_base is stored with /v1 (normalized at write time), so no suffix appending needed.
		if p.ProviderType == store.ProviderOllama {
			host := p.APIBase
			if host == "" {
				host = "http://localhost:11434/v1"
			}
			registry.RegisterForTenant(p.TenantID, providers.NewOpenAIProvider(p.Name, "ollama", config.DockerLocalhost(host), "llama3.3"))
			slog.Info("registered provider from DB", "name", p.Name)
			continue
		}
		// Vertex supports ADC (empty api_key) — handle before the generic key guard.
		if p.ProviderType == store.ProviderVertex {
			vsettings := store.ParseVertexProviderSettings(p.Settings)
			if vsettings == nil {
				slog.Warn("vertex: missing project_id/region in settings, skipping", "name", p.Name)
				continue
			}
			vcfg := providers.VertexConfig{
				Name:            p.Name,
				CredentialsJSON: p.APIKey,
				ProjectID:       vsettings.ProjectID,
				Region:          vsettings.Region,
				DefaultModel:    vsettings.Model,
				APIBaseOverride: p.APIBase,
			}
			prov, err := providers.NewVertexProviderWithTimeout(vcfg)
			if err != nil {
				slog.Warn("vertex: init from DB failed", "name", p.Name, "error", err)
				continue
			}
			registry.RegisterForTenant(p.TenantID, prov)
			slog.Info("registered provider from DB", "name", p.Name, "type", "vertex", "region", vsettings.Region)
			continue
		}

		if p.APIKey == "" {
			continue
		}
		// Fall back to config/env api_base when DB provider has none set.
		if p.APIBase == "" && cfg != nil {
			if base := cfg.Providers.APIBaseForType(p.ProviderType); base != "" {
				p.APIBase = base
				slog.Info("provider api_base inherited from config", "name", p.Name, "api_base", base)
			}
		}
		switch p.ProviderType {
		case store.ProviderChatGPTOAuth:
			ts := oauth.NewDBTokenSource(provStore, secretStore, p.Name).WithTenantID(p.TenantID)
			codex := providers.NewCodexProvider(p.Name, ts, p.APIBase, "")
			if oauthSettings := store.ParseChatGPTOAuthProviderSettings(p.Settings); oauthSettings != nil {
				codex.WithRoutingDefaults(oauthSettings.CodexPool.Strategy, oauthSettings.CodexPool.ExtraProviderNames)
			}
			registry.RegisterForTenant(p.TenantID, codex)
		case store.ProviderAnthropicNative:
			registry.RegisterForTenant(p.TenantID, providers.NewAnthropicProvider(p.APIKey,
				providers.WithAnthropicName(p.Name),
				providers.WithAnthropicBaseURL(p.APIBase),
				providers.WithAnthropicRegistry(modelReg)))
		case store.ProviderDashScope:
			registry.RegisterForTenant(p.TenantID, providers.NewDashScopeProvider(p.Name, p.APIKey, p.APIBase, ""))
		case store.ProviderBailian:
			base := p.APIBase
			if base == "" {
				base = "https://coding-intl.dashscope.aliyuncs.com/v1"
			}
			registry.RegisterForTenant(p.TenantID, providers.NewOpenAIProvider(p.Name, p.APIKey, base, "qwen3.5-plus"))
		case store.ProviderZai:
			base := p.APIBase
			if base == "" {
				base = "https://api.z.ai/api/paas/v4"
			}
			registry.RegisterForTenant(p.TenantID, providers.NewOpenAIProvider(p.Name, p.APIKey, base, "glm-5"))
		case store.ProviderZaiCoding:
			base := p.APIBase
			if base == "" {
				base = "https://api.z.ai/api/coding/paas/v4"
			}
			registry.RegisterForTenant(p.TenantID, providers.NewOpenAIProvider(p.Name, p.APIKey, base, "glm-5"))
		case store.ProviderOllamaCloud:
			base := p.APIBase
			if base == "" {
				base = "https://ollama.com/v1"
			}
			registry.RegisterForTenant(p.TenantID, providers.NewOpenAIProvider(p.Name, p.APIKey, base, "llama3.3"))
		case store.ProviderNovita:
			base := p.APIBase
			if base == "" {
				base = store.NovitaDefaultAPIBase
			}
			registry.RegisterForTenant(p.TenantID, providers.NewOpenAIProvider(p.Name, p.APIKey, base, store.NovitaDefaultModel))
		case store.ProviderBytePlus:
			base := p.APIBase
			if base == "" {
				base = store.BytePlusDefaultAPIBase
			}
			prov := providers.NewOpenAIProvider(p.Name, p.APIKey, base, store.BytePlusDefaultModel)
			prov.WithProviderType(p.ProviderType)
			registry.RegisterForTenant(p.TenantID, prov)
		case store.ProviderBytePlusCoding:
			base := p.APIBase
			if base == "" {
				base = store.BytePlusCodingDefaultAPIBase
			}
			prov := providers.NewOpenAIProvider(p.Name, p.APIKey, base, store.BytePlusDefaultModel)
			prov.WithProviderType(p.ProviderType)
			registry.RegisterForTenant(p.TenantID, prov)
		default:
			prov := providers.NewOpenAIProvider(p.Name, p.APIKey, p.APIBase, "")
			prov.WithProviderType(p.ProviderType)
			if p.ProviderType == store.ProviderMiniMax {
				prov.WithChatPath("/text/chatcompletion_v2")
			}
			if p.ProviderType == store.ProviderOpenRouter {
				prov.WithSiteInfo("https://goclaw.sh", "GoClaw")
			}
			registry.RegisterForTenant(p.TenantID, prov)
		}
		slog.Info("registered provider from DB", "name", p.Name)
	}
}

// registerACPFromConfig registers an ACP provider from config file settings.
func registerACPFromConfig(registry *providers.Registry, cfg config.ACPConfig, acpDeps ACPDeps) {
	if _, err := exec.LookPath(cfg.Binary); err != nil {
		slog.Warn("acp: binary not found, skipping", "binary", cfg.Binary, "error", err)
		return
	}
	idleTTL := 5 * time.Minute
	if cfg.IdleTTL != "" {
		if d, err := time.ParseDuration(cfg.IdleTTL); err == nil {
			idleTTL = d
		}
	}
	workDir := cfg.WorkDir
	if workDir == "" {
		workDir = defaultACPWorkDir()
	}
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		slog.Warn("acp: failed to create work dir, spawn will fail", "dir", workDir, "error", err)
	}
	var opts []providers.ACPOption
	if cfg.Model != "" {
		opts = append(opts, providers.WithACPModel(cfg.Model))
	}
	if cfg.PermMode != "" {
		opts = append(opts, providers.WithACPPermMode(cfg.PermMode))
	}
	if acpDeps.Shim != nil {
		opts = append(opts, providers.WithACPShim(acpDeps.Shim))
	}
	if acpDeps.Resolver != nil {
		opts = append(opts, providers.WithACPResolver(acpDeps.Resolver))
	}
	if acpDeps.CtxReader != nil {
		opts = append(opts, providers.WithACPContextReader(acpDeps.CtxReader))
	}
	if acpDeps.CfgReader != nil {
		// Config-file ACP runs in the master tenant scope (no per-tenant
		// row in providers DB) — the master-bound CfgReader from
		// setupACPShim is exactly what we want here.
		opts = append(opts, providers.WithACPConfigReader(acpDeps.CfgReader))
	}
	registry.Register(providers.NewACPProvider(
		cfg.Binary, cfg.Args, workDir, idleTTL, tools.DefaultDenyPatterns(), opts...,
	))
	slog.Info("registered provider", "name", "acp", "binary", cfg.Binary, "shim", acpDeps.Shim != nil)
}

// registerACPFromDB registers an ACP provider from a DB provider row.
func registerACPFromDB(registry *providers.Registry, p store.LLMProviderData, acpDeps ACPDeps) {
	binary := p.APIBase // repurpose api_base as binary path
	if binary == "" {
		slog.Warn("acp: no binary specified in DB provider", "name", p.Name)
		return
	}
	if binary != "claude" && binary != "codex" && binary != "gemini" && !filepath.IsAbs(binary) {
		slog.Warn("security.acp: invalid binary path from DB", "path", binary)
		return
	}
	if _, err := exec.LookPath(binary); err != nil {
		slog.Warn("acp: binary not found, skipping", "binary", binary, "error", err)
		return
	}
	// Parse settings JSONB for extra config
	var settings struct {
		Args     []string `json:"args"`
		IdleTTL  string   `json:"idle_ttl"`
		PermMode string   `json:"perm_mode"`
		WorkDir  string   `json:"work_dir"`
	}
	if p.Settings != nil {
		if err := json.Unmarshal(p.Settings, &settings); err != nil {
			slog.Warn("acp: invalid settings JSON, using defaults", "name", p.Name, "error", err)
		}
	}
	idleTTL := 5 * time.Minute
	if settings.IdleTTL != "" {
		if d, err := time.ParseDuration(settings.IdleTTL); err == nil {
			idleTTL = d
		}
	}
	workDir := settings.WorkDir
	if workDir == "" {
		workDir = defaultACPWorkDir()
	}
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		slog.Warn("acp: failed to create work dir, spawn will fail", "name", p.Name, "dir", workDir, "error", err)
	}
	opts := []providers.ACPOption{
		providers.WithACPName(p.Name),
		providers.WithACPModel(p.Name),
		providers.WithACPTenantID(p.TenantID.String()),
	}
	if acpDeps.Shim != nil {
		opts = append(opts, providers.WithACPShim(acpDeps.Shim))
	}
	if acpDeps.Resolver != nil {
		opts = append(opts, providers.WithACPResolver(acpDeps.Resolver))
	}
	if acpDeps.CtxReader != nil {
		opts = append(opts, providers.WithACPContextReader(acpDeps.CtxReader))
	}
	// Per-tenant CfgReader so acp.shim.enabled honors a tenant-specific
	// override row when present (falls back to master at the SQL layer
	// only if the operator chooses to seed both — current PG impl is
	// strict tenant-scoped, so most deployments use the master row).
	if base, ok := acpDeps.CfgReader.(systemConfigReader); ok && base.src != nil {
		opts = append(opts, providers.WithACPConfigReader(systemConfigReader{src: base.src, tenantID: p.TenantID}))
	} else if acpDeps.CfgReader != nil {
		opts = append(opts, providers.WithACPConfigReader(acpDeps.CfgReader))
	}
	registry.RegisterForTenant(p.TenantID, providers.NewACPProvider(
		binary, settings.Args, workDir, idleTTL, tools.DefaultDenyPatterns(), opts...,
	))
	slog.Info("registered provider from DB", "name", p.Name, "type", "acp", "shim", acpDeps.Shim != nil)
}

// defaultACPWorkDir returns the default workspace directory for ACP agents.
func defaultACPWorkDir() string {
	return filepath.Join(config.ResolvedDataDirFromEnv(), "acp-workspaces")
}

// setupACPShim constructs the in-process MCP shim + grants resolver used by
// ACP providers to advertise per-session HTTP MCP servers to
// claude-agent-acp. Returns a zero-valued ACPDeps when the shim cannot be
// built (listen failure, missing dependencies) — callers degrade gracefully
// to Phase 3 behavior.
//
// The shim binds to 127.0.0.1:0 (ephemeral port, localhost-only). Per-session
// allowlists are computed at session/new time from agents.acp_tools ∩
// BridgeToolNames ∪ mcp_agent_grants.tool_allow.
func setupACPShim(toolsReg *tools.Registry, msgBus *bus.MessageBus, pgStores *store.Stores) ACPDeps {
	if toolsReg == nil {
		slog.Info("acp.shim.skipped", "reason", "tools registry not available")
		return ACPDeps{}
	}
	shimSrv, err := mcp_shim.NewServer(mcp_shim.ServerConfig{
		ListenAddr: "127.0.0.1:0",
		Registry:   toolsReg,
		MsgBus:     msgBus,
		Version:    Version,
	})
	if err != nil {
		slog.Warn("acp.shim.startup_failed", "error", err)
		return ACPDeps{}
	}
	handle := mcp_shim.NewHandle(shimSrv)
	slog.Info("acp.shim.startup_ok", "url", handle.URL())

	// PG-backed grants store: requires both Agents (acp_tools lookup) and MCP
	// (agent_grants list). If either is missing the resolver still runs but
	// returns an empty grant list — sessions get empty allowlists, no crash.
	var grantSource mcp_shim.GrantsStore
	if pgStores != nil {
		var agentLookup mcp_shim.ACPToolsLookup
		if pgAgents, ok := pgStores.Agents.(mcp_shim.ACPToolsLookup); ok {
			agentLookup = pgAgents
		}
		var mcpLookup mcp_shim.MCPAccessLookup
		if pgStores.MCP != nil {
			mcpLookup = mcpAccessAdapter{src: pgStores.MCP}
		}
		grantSource = mcp_shim.NewPGGrantsStore(agentLookup, mcpLookup)
	}
	resolver := mcp_shim.NewResolver(grantSource)

	// CfgReader is left zero here and re-bound per-provider in
	// registerACPFromConfig / registerACPFromDB so the tenant scope on the
	// system_configs Get matches the owning tenant. Without per-provider
	// binding a config-file ACP provider would read the wrong tenant.
	var cfgReader providers.ACPConfigReader
	if pgStores != nil && pgStores.SystemConfigs != nil {
		// Master-tenant binding is the safe fallback when the caller
		// (e.g. registerACPFromConfig) does not override. registerACPFromDB
		// builds its own per-tenant reader below.
		cfgReader = systemConfigReader{src: pgStores.SystemConfigs, tenantID: store.MasterTenantID}
	}

	return ACPDeps{
		Shim:      handle,
		Resolver:  resolver,
		CtxReader: toolsCtxReader{},
		CfgReader: cfgReader,
	}
}
