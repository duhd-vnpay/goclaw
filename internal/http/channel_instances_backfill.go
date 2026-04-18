package http

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/channels"
	"github.com/nextlevelbuilder/goclaw/internal/i18n"
	"github.com/nextlevelbuilder/goclaw/internal/store"
	"github.com/nextlevelbuilder/goclaw/pkg/protocol"
)

// backfillRowTimeout bounds each resolver call during backfill. Telegram's
// getChatMember is normally sub-second; 3s covers retries/transient slowness.
const backfillRowTimeout = 3 * time.Second

// backfillInterRowDelay throttles bot API hits so a large backfill never
// approaches the 30 req/s Telegram ceiling — at ~10 req/s we stay safe with
// margin for other operations.
const backfillInterRowDelay = 100 * time.Millisecond

// handleBackfillWriterMetadata re-enriches file_writer rows whose metadata is
// empty (legacy rows created by the Web UI before auto-enrichment existed).
// Idempotent — safe to re-run. Query param `channel` optionally filters by
// channel name (e.g. `?channel=telegram` or `?channel=me-me-bot`).
//
// Response: {"total":X,"enriched":Y,"skipped":Z,"failed":W}.
func (h *ChannelInstancesHandler) handleBackfillWriterMetadata(w http.ResponseWriter, r *http.Request) {
	locale := store.LocaleFromContext(r.Context())
	// Global ops — scans/mutates rows across ALL tenants. Gate behind master
	// scope so a tenant-admin cannot touch another tenant's writer rows.
	if !requireMasterScope(w, r) {
		return
	}
	if h.memberResolver == nil {
		writeError(w, http.StatusServiceUnavailable, protocol.ErrInternal, i18n.T(locale, i18n.MsgFailedToCreate, "backfill", "member resolver not configured"))
		return
	}
	channelFilter := r.URL.Query().Get("channel")
	dryRun := r.URL.Query().Get("dry_run") == "1" || r.URL.Query().Get("dry_run") == "true"

	rows, err := h.configPermStore.ListFileWritersMissingMetadata(r.Context(), channelFilter)
	if err != nil {
		slog.Error("backfill.list_failed", "error", err)
		writeError(w, http.StatusInternalServerError, protocol.ErrInternal, i18n.T(locale, i18n.MsgFailedToCreate, "backfill", "list rows"))
		return
	}

	var enriched, skipped, failed int
	for i := range rows {
		row := rows[i]
		ctx, cancel := context.WithTimeout(r.Context(), backfillRowTimeout)
		meta, ok := channels.EnrichFileWriterMetadata(ctx, h.memberResolver, row.Scope, row.UserID)
		cancel()
		if !ok {
			// Either channel not supported, scope malformed, or resolver call
			// failed. Helper already logged the reason.
			if _, _, scopeOK := channels.ParseGroupScope(row.Scope); !scopeOK {
				skipped++
			} else {
				failed++
			}
			continue
		}
		if dryRun {
			enriched++
			slog.Info("backfill.dry_run", "scope", row.Scope, "user_id", row.UserID, "metadata", string(meta))
		} else {
			row.Metadata = meta
			if err := h.configPermStore.Grant(r.Context(), &row); err != nil {
				slog.Warn("backfill.grant_failed", "scope", row.Scope, "user_id", row.UserID, "error", err)
				failed++
				continue
			}
			enriched++
			slog.Info("backfill.enriched", "scope", row.Scope, "user_id", row.UserID)
		}
		// Rate-limit to stay well under Telegram's ceiling across runs.
		if i < len(rows)-1 {
			time.Sleep(backfillInterRowDelay)
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"total":    len(rows),
		"enriched": enriched,
		"skipped":  skipped,
		"failed":   failed,
		"dry_run":  dryRun,
	})
}
