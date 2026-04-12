package http

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/bus"
	"github.com/nextlevelbuilder/goclaw/internal/i18n"
	"github.com/nextlevelbuilder/goclaw/internal/permissions"
	"github.com/nextlevelbuilder/goclaw/internal/store"
	"github.com/nextlevelbuilder/goclaw/pkg/protocol"
)

// BuiltinToolsHandler handles built-in tool management endpoints.
// Built-in tools are seeded at startup; only enabled and settings are editable.
type BuiltinToolsHandler struct {
	store          store.BuiltinToolStore
	tenantCfgStore store.BuiltinToolTenantConfigStore
	tenantStore    store.TenantStore
	msgBus         *bus.MessageBus
}

// NewBuiltinToolsHandler creates a handler for built-in tool management endpoints.
func NewBuiltinToolsHandler(s store.BuiltinToolStore, tenantCfgs store.BuiltinToolTenantConfigStore, tenantStore store.TenantStore, msgBus *bus.MessageBus) *BuiltinToolsHandler {
	return &BuiltinToolsHandler{store: s, tenantCfgStore: tenantCfgs, tenantStore: tenantStore, msgBus: msgBus}
}

// RegisterRoutes registers all built-in tool routes on the given mux.
func (h *BuiltinToolsHandler) RegisterRoutes(mux *http.ServeMux) {
	// Builtin tools (reads: viewer+, writes: admin+)
	mux.HandleFunc("GET /v1/tools/builtin", h.auth(h.handleList))
	mux.HandleFunc("GET /v1/tools/builtin/{name}", h.auth(h.handleGet))
	mux.HandleFunc("PUT /v1/tools/builtin/{name}", h.adminAuth(h.handleUpdate))
	mux.HandleFunc("GET /v1/tools/builtin/{name}/tenant-config", h.adminAuth(h.handleGetTenantConfig))
	mux.HandleFunc("PUT /v1/tools/builtin/{name}/tenant-config", h.adminAuth(h.handleSetTenantConfig))
	mux.HandleFunc("DELETE /v1/tools/builtin/{name}/tenant-config", h.adminAuth(h.handleDeleteTenantConfig))
}

func (h *BuiltinToolsHandler) auth(next http.HandlerFunc) http.HandlerFunc {
	return requireAuth("", next)
}

func (h *BuiltinToolsHandler) adminAuth(next http.HandlerFunc) http.HandlerFunc {
	return requireAuth(permissions.RoleAdmin, next)
}

// emitCacheInvalidate broadcasts a cache invalidation event for a builtin tool.
// tenantID == uuid.Nil means global invalidation (master admin path).
func (h *BuiltinToolsHandler) emitCacheInvalidate(key string, tenantID uuid.UUID) {
	if h.msgBus == nil {
		return
	}
	h.msgBus.Broadcast(bus.Event{
		Name: protocol.EventCacheInvalidate,
		Payload: bus.CacheInvalidatePayload{
			Kind:     bus.CacheKindBuiltinTools,
			Key:      key,
			TenantID: tenantID,
		},
	})
}

func (h *BuiltinToolsHandler) handleList(w http.ResponseWriter, r *http.Request) {
	result, err := h.store.List(r.Context())
	if err != nil {
		slog.Error("builtin_tools.list", "error", err)
		locale := extractLocale(r)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": i18n.T(locale, i18n.MsgFailedToList, "tools")})
		return
	}

	// Merge per-tenant overrides (enabled + settings) into response when tenant-scoped.
	tid := store.TenantIDFromContext(r.Context())
	if tid != uuid.Nil && h.tenantCfgStore != nil {
		enabledOverrides, _ := h.tenantCfgStore.ListAll(r.Context(), tid)
		settingsOverrides, _ := h.tenantCfgStore.ListAllSettings(r.Context(), tid)
		if len(enabledOverrides) > 0 || len(settingsOverrides) > 0 {
			type toolWithTenant struct {
				store.BuiltinToolDef
				TenantEnabled  *bool           `json:"tenant_enabled"`
				TenantSettings json.RawMessage `json:"tenant_settings,omitempty"`
			}
			enriched := make([]toolWithTenant, len(result))
			for i, t := range result {
				enriched[i] = toolWithTenant{BuiltinToolDef: t}
				if enabled, ok := enabledOverrides[t.Name]; ok {
					enriched[i].TenantEnabled = &enabled
				}
				if raw, ok := settingsOverrides[t.Name]; ok {
					enriched[i].TenantSettings = raw
				}
			}
			writeJSON(w, http.StatusOK, map[string]any{"tools": enriched})
			return
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"tools": result})
}

func (h *BuiltinToolsHandler) handleGet(w http.ResponseWriter, r *http.Request) {
	locale := extractLocale(r)
	name := r.PathValue("name")
	if name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": i18n.T(locale, i18n.MsgRequired, "name")})
		return
	}

	def, err := h.store.Get(r.Context(), name)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": i18n.T(locale, i18n.MsgNotFound, "tool", name)})
		return
	}

	writeJSON(w, http.StatusOK, def)
}

func (h *BuiltinToolsHandler) handleUpdate(w http.ResponseWriter, r *http.Request) {
	locale := extractLocale(r)
	name := r.PathValue("name")
	if name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": i18n.T(locale, i18n.MsgRequired, "name")})
		return
	}

	var updates map[string]any
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&updates); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": i18n.T(locale, i18n.MsgInvalidJSON)})
		return
	}

	// Only allow enabled and settings to be updated
	allowed := make(map[string]any)
	if v, ok := updates["enabled"]; ok {
		allowed["enabled"] = v
	}
	if v, ok := updates["settings"]; ok {
		allowed["settings"] = v
	}

	if len(allowed) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": i18n.T(locale, i18n.MsgInvalidUpdates)})
		return
	}

	if err := h.store.Update(r.Context(), name, allowed); err != nil {
		slog.Error("builtin_tools.update", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	emitAudit(h.msgBus, r, "builtin_tool.updated", "builtin_tool", name)
	h.emitCacheInvalidate(name, uuid.Nil)
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// setTenantConfigRequest is the PUT body for tenant config overrides.
// Both fields are optional — at least one must be set. Pointer *bool
// distinguishes "not set" (pass-through) from "explicit false". json.RawMessage
// for settings preserves bytes for the store without an intermediate decode.
type setTenantConfigRequest struct {
	Enabled  *bool           `json:"enabled,omitempty"`
	Settings json.RawMessage `json:"settings,omitempty"`
}

// isValidSettingsJSON returns true if raw is a JSON object or the literal "null".
// Non-object types (arrays, primitives) are rejected to keep the schema predictable.
func isValidSettingsJSON(raw json.RawMessage) bool {
	s := string(raw)
	if s == "null" {
		return true
	}
	var v map[string]any
	return json.Unmarshal(raw, &v) == nil
}

// handleGetTenantConfig returns the tenant override view for a single tool.
// Response: { tool_name, enabled, settings } — enabled/settings nil when unset.
func (h *BuiltinToolsHandler) handleGetTenantConfig(w http.ResponseWriter, r *http.Request) {
	if h.tenantCfgStore == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "tenant config not available"})
		return
	}
	if !requireTenantAdmin(w, r, h.tenantStore) {
		return
	}
	name := r.PathValue("name")
	tid := store.TenantIDFromContext(r.Context())
	if tid == uuid.Nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "tenant context required"})
		return
	}

	enabledAll, listErr := h.tenantCfgStore.ListAll(r.Context(), tid)
	if listErr != nil {
		slog.Warn("list tenant enabled overrides failed", "tenant", tid, "error", listErr)
	}
	settings, err := h.tenantCfgStore.GetSettings(r.Context(), tid, name)
	if err != nil {
		slog.Warn("get tenant tool settings failed", "tool", name, "tenant", tid, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	type response struct {
		ToolName string          `json:"tool_name"`
		Enabled  *bool           `json:"enabled,omitempty"`
		Settings json.RawMessage `json:"settings,omitempty"`
	}
	resp := response{ToolName: name, Settings: settings}
	if v, ok := enabledAll[name]; ok {
		resp.Enabled = &v
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleSetTenantConfig upserts per-tenant overrides for a builtin tool.
// Body: { enabled?, settings? } — both optional, at least one required.
// Settings passed as literal `null` clears the settings column without deleting the row.
func (h *BuiltinToolsHandler) handleSetTenantConfig(w http.ResponseWriter, r *http.Request) {
	if h.tenantCfgStore == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "tenant config not available"})
		return
	}
	if !requireTenantAdmin(w, r, h.tenantStore) {
		return
	}
	name := r.PathValue("name")
	tid := store.TenantIDFromContext(r.Context())
	if tid == uuid.Nil {
		// Defense-in-depth: owner-role bypass in requireTenantAdmin could
		// otherwise reach here without a tenant scope. A nil tid must never
		// flow into the cache invalidate emit as a global wipe.
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "tenant context required"})
		return
	}

	var body setTenantConfigRequest
	// 16KB cap — settings blobs should stay small (provider chains, toggles).
	// Large blobs indicate misuse; reject to prevent trivial DoS via oversized JSON.
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if body.Enabled == nil && body.Settings == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "at least one of enabled or settings required"})
		return
	}
	if body.Settings != nil && !isValidSettingsJSON(body.Settings) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "settings must be a JSON object or null"})
		return
	}

	// Write enabled if provided (preserves settings column via column-list upsert).
	if body.Enabled != nil {
		if err := h.tenantCfgStore.Set(r.Context(), tid, name, *body.Enabled); err != nil {
			slog.Warn("set tenant tool enabled failed", "tool", name, "tenant", tid, "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
	}
	// Write settings if provided (preserves enabled column). JSON literal `null`
	// maps to Go nil RawMessage after decode — pass through to store which writes SQL NULL.
	if body.Settings != nil {
		var payload json.RawMessage
		if string(body.Settings) != "null" {
			payload = body.Settings
		}
		if err := h.tenantCfgStore.SetSettings(r.Context(), tid, name, payload); err != nil {
			slog.Warn("set tenant tool settings failed", "tool", name, "tenant", tid, "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
	}

	emitAudit(h.msgBus, r, "builtin_tool.tenant_config.set", "builtin_tool", name)
	h.emitCacheInvalidate(name, tid)
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// handleDeleteTenantConfig removes a per-tenant override (reverts to default).
func (h *BuiltinToolsHandler) handleDeleteTenantConfig(w http.ResponseWriter, r *http.Request) {
	if h.tenantCfgStore == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "tenant config not available"})
		return
	}
	if !requireTenantAdmin(w, r, h.tenantStore) {
		return
	}
	name := r.PathValue("name")
	tid := store.TenantIDFromContext(r.Context())
	if tid == uuid.Nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "tenant context required"})
		return
	}

	if err := h.tenantCfgStore.Delete(r.Context(), tid, name); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	emitAudit(h.msgBus, r, "builtin_tool.tenant_config.deleted", "builtin_tool", name)
	h.emitCacheInvalidate(name, tid)
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
