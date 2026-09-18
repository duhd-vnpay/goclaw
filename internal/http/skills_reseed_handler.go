package http

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/nextlevelbuilder/goclaw/internal/bus"
	"github.com/nextlevelbuilder/goclaw/internal/permissions"
	"github.com/nextlevelbuilder/goclaw/internal/skills"
)

// SkillReseedHandler handles POST /v1/system/skills/reseed.
// It re-runs the bundled skill seeder so that metadata updates (name, description)
// in SKILL.md are applied to the DB even when the file hash is unchanged.
// This is called by the CI deploy pipeline after a gateway upgrade completes.
type SkillReseedHandler struct {
	bundledDir string
	managedDir string
	store      skills.SystemSkillStore
	msgBus     *bus.MessageBus
}

// NewSkillReseedHandler creates a new reseed handler.
// bundledDir: /app/bundled-skills (or dev equivalent)
// managedDir: /app/data/skills-store
func NewSkillReseedHandler(bundledDir, managedDir string, store skills.SystemSkillStore, msgBus *bus.MessageBus) *SkillReseedHandler {
	return &SkillReseedHandler{
		bundledDir: bundledDir,
		managedDir: managedDir,
		store:      store,
		msgBus:     msgBus,
	}
}

func (h *SkillReseedHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/system/skills/reseed", requireAuth(permissions.RoleAdmin, h.handleReseed))
}

func (h *SkillReseedHandler) handleReseed(w http.ResponseWriter, r *http.Request) {
	if !requireMasterScope(w, r) {
		return
	}

	if h.bundledDir == "" || h.managedDir == "" || h.store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "skills seeder not configured (bundledDir or managedDir missing)",
		})
		return
	}

	seeder := skills.NewSeeder(h.bundledDir, h.managedDir, h.store)
	seeded, skipped, seededSkills, err := seeder.Seed(context.Background())
	if err != nil {
		slog.Error("skills reseed failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "skills reseed failed: " + err.Error(),
		})
		return
	}

	slog.Info("skills reseed completed", "seeded", seeded, "skipped", skipped)

	// Run async dep check for any newly seeded/updated skills.
	if len(seededSkills) > 0 {
		seeder.CheckDepsAsync(seededSkills, h.msgBus)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"seeded":  seeded,
		"skipped": skipped,
	})
}
