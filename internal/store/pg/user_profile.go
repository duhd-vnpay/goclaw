package pg

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

const (
	profileCacheTTL     = 30 * time.Minute
	profileCacheMaxSize = 1000
)

// profileCacheEntry wraps a cached UserProfile with expiration.
type profileCacheEntry struct {
	profile   *store.UserProfile // nil = confirmed anonymous (no paired device)
	expiresAt time.Time
}

// PGProfileResolver resolves UserProfile from paired_devices → org_users → departments.
// Caches resolved profiles in memory with 30min TTL.
type PGProfileResolver struct {
	db    *sql.DB
	mu    sync.RWMutex
	cache map[string]profileCacheEntry // key: "senderID:channelType"
}

// NewPGProfileResolver creates a new PostgreSQL-backed profile resolver.
func NewPGProfileResolver(db *sql.DB) *PGProfileResolver {
	r := &PGProfileResolver{
		db:    db,
		cache: make(map[string]profileCacheEntry, 64),
	}
	// Start background sweeper to evict expired entries.
	go r.sweepLoop()
	return r
}

func (r *PGProfileResolver) cacheKey(senderID, channelType string) string {
	return senderID + ":" + channelType
}

func (r *PGProfileResolver) ResolveFromPairedDevice(ctx context.Context, senderID, channelType string) (*store.UserProfile, error) {
	key := r.cacheKey(senderID, channelType)

	// Check cache first (read lock).
	r.mu.RLock()
	if entry, ok := r.cache[key]; ok && time.Now().Before(entry.expiresAt) {
		r.mu.RUnlock()
		return entry.profile, nil
	}
	r.mu.RUnlock()

	// Cache miss — resolve from DB.
	profile, err := r.resolveFromDB(ctx, senderID, channelType)
	if err != nil {
		return nil, err
	}

	// Store in cache (even nil = confirmed anonymous).
	r.mu.Lock()
	// Evict if cache is too large (simple strategy: clear all).
	if len(r.cache) >= profileCacheMaxSize {
		r.cache = make(map[string]profileCacheEntry, 64)
	}
	r.cache[key] = profileCacheEntry{
		profile:   profile,
		expiresAt: time.Now().Add(profileCacheTTL),
	}
	r.mu.Unlock()

	return profile, nil
}

func (r *PGProfileResolver) InvalidateCache(senderID, channelType string) {
	key := r.cacheKey(senderID, channelType)
	r.mu.Lock()
	delete(r.cache, key)
	r.mu.Unlock()
}

// resolveFromDB performs the full resolution:
// 1. Look up verified_user_id from paired_devices
// 2. Fetch org_users row
// 3. Fetch department_members + departments (single JOIN)
// Returns nil if sender is not paired (no verified_user_id).
func (r *PGProfileResolver) resolveFromDB(ctx context.Context, senderID, channelType string) (*store.UserProfile, error) {
	tid := store.TenantIDFromContext(ctx)
	if tid == uuid.Nil {
		tid = store.MasterTenantID
	}

	// Step 1: Look up verified_user_id from paired_devices.
	var verifiedUserID *uuid.UUID
	err := r.db.QueryRowContext(ctx, `
		SELECT verified_user_id
		FROM paired_devices
		WHERE sender_id = $1 AND channel = $2 AND tenant_id = $3
	`, senderID, channelType, tid).Scan(&verifiedUserID)

	if err == sql.ErrNoRows || verifiedUserID == nil || *verifiedUserID == uuid.Nil {
		// Not paired — anonymous user.
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	// Step 2: Fetch org_users + tenant role in one query.
	var profile store.UserProfile
	var displayName, profileJSON sql.NullString
	var tenantRole sql.NullString

	err = r.db.QueryRowContext(ctx, `
		SELECT ou.id, ou.email, ou.display_name, ou.profile,
		       tu.role
		FROM org_users ou
		LEFT JOIN tenant_users tu ON tu.keycloak_id = ou.id AND tu.tenant_id = ou.tenant_id
		WHERE ou.id = $1 AND ou.tenant_id = $2
	`, *verifiedUserID, tid).Scan(
		&profile.ID, &profile.Email, &displayName, &profileJSON,
		&tenantRole,
	)
	if err == sql.ErrNoRows {
		// User record deleted but pairing still exists — treat as anonymous.
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if displayName.Valid {
		profile.DisplayName = displayName.String
	}
	if tenantRole.Valid {
		profile.TenantRole = tenantRole.String
	}

	// Parse profile JSONB for expertise, timezone, availability, preferred_channel.
	if profileJSON.Valid && profileJSON.String != "" {
		r.parseProfileJSON(profileJSON.String, &profile)
	}

	// Step 3: Fetch department memberships (single JOIN).
	deptRows, err := r.db.QueryContext(ctx, `
		SELECT d.name, dm.role, COALESCE(dm.title, '')
		FROM department_members dm
		JOIN departments d ON d.id = dm.department_id
		WHERE dm.user_id = $1
		ORDER BY dm.created_at
	`, profile.ID)
	if err != nil {
		// Non-fatal: log and continue without departments.
		slog.Warn("user_profile: failed to fetch departments",
			"user_id", profile.ID, "error", err)
	} else {
		defer deptRows.Close()
		for deptRows.Next() {
			var dm store.DepartmentMembership
			if err := deptRows.Scan(&dm.DepartmentName, &dm.Role, &dm.Title); err != nil {
				slog.Warn("user_profile: scan department row failed", "error", err)
				continue
			}
			profile.Departments = append(profile.Departments, dm)
		}
	}

	return &profile, nil
}

// parseProfileJSON extracts typed fields from org_users.profile JSONB.
func (r *PGProfileResolver) parseProfileJSON(raw string, profile *store.UserProfile) {
	var data struct {
		Expertise        []string `json:"expertise"`
		Timezone         string   `json:"timezone"`
		Availability     string   `json:"availability"`
		PreferredChannel string   `json:"preferred_channel"`
	}
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		slog.Debug("user_profile: failed to parse profile JSON", "error", err)
		return
	}
	profile.Expertise = data.Expertise
	profile.Timezone = data.Timezone
	profile.Availability = data.Availability
	profile.PreferredChannel = data.PreferredChannel
}

// sweepLoop periodically removes expired entries from the cache.
func (r *PGProfileResolver) sweepLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		r.mu.Lock()
		now := time.Now()
		for k, v := range r.cache {
			if now.After(v.expiresAt) {
				delete(r.cache, k)
			}
		}
		r.mu.Unlock()
	}
}
