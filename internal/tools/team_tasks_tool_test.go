package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// ── Mock stores ────────────────────────────────────────────────────────────────

type mockTeamStore struct {
	store.TeamStore // embed to satisfy full interface
	team        *store.TeamData
	members     []store.TeamMemberData
	createdTask *store.TeamTaskData
}

func (m *mockTeamStore) GetTeamForAgent(_ context.Context, _ uuid.UUID) (*store.TeamData, error) {
	if m.team == nil {
		return nil, nil
	}
	return m.team, nil
}

func (m *mockTeamStore) ListMembers(_ context.Context, _ uuid.UUID) ([]store.TeamMemberData, error) {
	return m.members, nil
}

func (m *mockTeamStore) AssignTask(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ uuid.UUID) error {
	return nil
}

func (m *mockTeamStore) CreateTask(_ context.Context, task *store.TeamTaskData) error {
	task.ID = uuid.New()
	task.CreatedAt = time.Now()
	m.createdTask = task
	return nil
}

type mockAgentStore struct {
	store.AgentStore
	agents map[string]*store.AgentData // key → agent
}

func (m *mockAgentStore) GetByKey(_ context.Context, key string) (*store.AgentData, error) {
	if m.agents != nil {
		if ag, ok := m.agents[key]; ok {
			return ag, nil
		}
	}
	return nil, fmt.Errorf("not found")
}
func (m *mockAgentStore) GetByID(_ context.Context, id uuid.UUID) (*store.AgentData, error) {
	if m.agents != nil {
		for _, ag := range m.agents {
			if ag.ID == id {
				return ag, nil
			}
		}
	}
	return nil, fmt.Errorf("not found")
}

// ── Helper ─────────────────────────────────────────────────────────────────────

func makeTestTeam(leadID uuid.UUID, settings json.RawMessage) *store.TeamData {
	return &store.TeamData{
		BaseModel:   store.BaseModel{ID: uuid.New()},
		Name:        "test-team",
		LeadAgentID: leadID,
		Status:      "active",
		Settings:    settings,
	}
}

func makeCtx(agentID uuid.UUID, userID, senderID, channel string) context.Context {
	ctx := context.Background()
	ctx = store.WithAgentID(ctx, agentID)
	ctx = store.WithUserID(ctx, userID)
	if senderID != "" {
		ctx = store.WithSenderID(ctx, senderID)
	}
	ctx = WithToolChannel(ctx, channel)
	return ctx
}

// ── Tests: sender_id tracking ──────────────────────────────────────────────────

func TestExecuteCreate_SenderIDTracking(t *testing.T) {
	leadID := uuid.New()
	memberID := uuid.New()
	team := makeTestTeam(leadID, nil)

	ts := &mockTeamStore{
		team:    team,
		members: []store.TeamMemberData{{AgentID: memberID, Role: "member"}},
	}
	as := &mockAgentStore{agents: map[string]*store.AgentData{
		"test-member": {BaseModel: store.BaseModel{ID: memberID}, AgentKey: "test-member"},
	}}
	mgr := NewTeamToolManager(ts, as, nil, "")
	tool := NewTeamTasksTool(mgr)

	ctx := makeCtx(leadID, "group:telegram:chat123", "user-456", "telegram")
	args := map[string]any{
		"action":   "create",
		"subject":  "Test task with sender_id",
		"assignee": "test-member",
	}

	result := tool.executeCreate(ctx, args)
	if result.IsError {
		t.Fatalf("expected success, got error: %s", result.ForLLM)
	}

	if ts.createdTask == nil {
		t.Fatal("expected task to be created")
	}

	// Channel is stored directly on the task (not in metadata)
	if ts.createdTask.Channel != "telegram" {
		t.Errorf("expected task.Channel=telegram, got %v", ts.createdTask.Channel)
	}
	// UserID propagated from context
	if ts.createdTask.UserID != "group:telegram:chat123" {
		t.Errorf("expected task.UserID=group:telegram:chat123, got %v", ts.createdTask.UserID)
	}
}

func TestExecuteCreate_NoSenderID(t *testing.T) {
	leadID := uuid.New()
	memberID := uuid.New()
	team := makeTestTeam(leadID, nil)

	ts := &mockTeamStore{
		team:    team,
		members: []store.TeamMemberData{{AgentID: memberID, Role: "member"}},
	}
	as := &mockAgentStore{agents: map[string]*store.AgentData{
		"test-member": {BaseModel: store.BaseModel{ID: memberID}, AgentKey: "test-member"},
	}}
	mgr := NewTeamToolManager(ts, as, nil, "")
	tool := NewTeamTasksTool(mgr)

	// No sender ID in context (delegate channel, internal agent-to-agent)
	ctx := makeCtx(leadID, "delegate:system", "", "delegate")
	args := map[string]any{
		"action":   "create",
		"subject":  "Internal task",
		"assignee": "test-member",
	}

	result := tool.executeCreate(ctx, args)
	if result.IsError {
		t.Fatalf("expected success, got error: %s", result.ForLLM)
	}

	// Channel stored on task field
	if ts.createdTask.Channel != "delegate" {
		t.Errorf("expected task.Channel=delegate, got %v", ts.createdTask.Channel)
	}
}

// ── Tests: requireLead ─────────────────────────────────────────────────────────

func TestExecuteCreate_RequireLead_Rejected(t *testing.T) {
	leadID := uuid.New()
	nonLeadID := uuid.New()
	team := makeTestTeam(leadID, nil)

	ts := &mockTeamStore{team: team}
	mgr := NewTeamToolManager(ts, &mockAgentStore{}, nil, "")
	tool := NewTeamTasksTool(mgr)

	// Non-lead agent trying to create task via telegram (member_requests not enabled)
	ctx := makeCtx(nonLeadID, "group:telegram:chat123", "user-789", "telegram")
	args := map[string]any{
		"action":  "create",
		"subject": "Unauthorized task",
	}

	result := tool.executeCreate(ctx, args)
	if !result.IsError {
		t.Fatal("expected error for non-lead agent")
	}
	if !strings.Contains(result.ForLLM, "Members cannot create tasks") {
		t.Errorf("expected 'Members cannot create tasks' error, got: %s", result.ForLLM)
	}
}

func TestExecuteCreate_RequireLead_DelegateBypass(t *testing.T) {
	leadID := uuid.New()
	nonLeadID := uuid.New()
	memberID := uuid.New()
	team := makeTestTeam(leadID, nil)

	ts := &mockTeamStore{
		team:    team,
		members: []store.TeamMemberData{{AgentID: memberID, Role: "member"}},
	}
	as := &mockAgentStore{agents: map[string]*store.AgentData{
		"test-member": {BaseModel: store.BaseModel{ID: memberID}, AgentKey: "test-member"},
	}}
	mgr := NewTeamToolManager(ts, as, nil, "")
	tool := NewTeamTasksTool(mgr)

	// Non-lead agent via teammate channel (internal agent-to-agent) should bypass — acts as lead
	ctx := makeCtx(nonLeadID, "delegate:system", "", ChannelTeammate)
	args := map[string]any{
		"action":   "create",
		"subject":  "Delegated task",
		"assignee": "test-member",
	}

	result := tool.executeCreate(ctx, args)
	if result.IsError {
		t.Fatalf("teammate channel should bypass requireLead, got: %s", result.ForLLM)
	}
}

// ── Tests: checkTeamAccess ─────────────────────────────────────────────────────

func TestCheckTeamAccess_AllowChannels(t *testing.T) {
	settings := json.RawMessage(`{"allow_channels":["telegram","delegate","system"]}`)

	// Allowed channel
	if err := checkTeamAccess(settings, "user1", "telegram"); err != nil {
		t.Errorf("telegram should be allowed: %v", err)
	}

	// Blocked channel
	if err := checkTeamAccess(settings, "user1", "slack"); err == nil {
		t.Error("slack should be denied")
	}

	// delegate always passes
	if err := checkTeamAccess(settings, "user1", "delegate"); err != nil {
		t.Errorf("delegate should always pass: %v", err)
	}

	// system always passes
	if err := checkTeamAccess(settings, "user1", "system"); err != nil {
		t.Errorf("system should always pass: %v", err)
	}
}

func TestCheckTeamAccess_DenyOverAllow(t *testing.T) {
	settings := json.RawMessage(`{
		"allow_user_ids": ["user-A", "user-B"],
		"deny_user_ids": ["user-B"]
	}`)

	// user-A allowed
	if err := checkTeamAccess(settings, "user-A", "telegram"); err != nil {
		t.Errorf("user-A should be allowed: %v", err)
	}

	// user-B denied (deny > allow)
	if err := checkTeamAccess(settings, "user-B", "telegram"); err == nil {
		t.Error("user-B should be denied (deny overrides allow)")
	}

	// user-C not in allow list
	if err := checkTeamAccess(settings, "user-C", "telegram"); err == nil {
		t.Error("user-C should be denied (not in allow list)")
	}
}

func TestCheckTeamAccess_EmptySettings(t *testing.T) {
	// Empty settings = open access
	if err := checkTeamAccess(nil, "anyone", "any-channel"); err != nil {
		t.Errorf("empty settings should allow all: %v", err)
	}
	if err := checkTeamAccess(json.RawMessage(`{}`), "anyone", "any-channel"); err != nil {
		t.Errorf("empty JSON settings should allow all: %v", err)
	}
}

func TestCheckTeamAccess_DenyChannels(t *testing.T) {
	settings := json.RawMessage(`{"deny_channels":["whatsapp"]}`)

	if err := checkTeamAccess(settings, "user1", "telegram"); err != nil {
		t.Errorf("telegram should be allowed: %v", err)
	}
	if err := checkTeamAccess(settings, "user1", "whatsapp"); err == nil {
		t.Error("whatsapp should be denied")
	}
}

// ── Tests: requireLead unit ────────────────────────────────────────────────────

func TestRequireLead_LeadAllowed(t *testing.T) {
	leadID := uuid.New()
	team := makeTestTeam(leadID, nil)
	mgr := NewTeamToolManager(&mockTeamStore{}, &mockAgentStore{}, nil, "")

	ctx := makeCtx(leadID, "user1", "", "telegram")
	if err := mgr.requireLead(ctx, team, leadID); err != nil {
		t.Errorf("lead should be allowed: %v", err)
	}
}

func TestRequireLead_NonLeadRejected(t *testing.T) {
	leadID := uuid.New()
	otherID := uuid.New()
	team := makeTestTeam(leadID, nil)
	mgr := NewTeamToolManager(&mockTeamStore{}, &mockAgentStore{}, nil, "")

	ctx := makeCtx(otherID, "user1", "", "telegram")
	if err := mgr.requireLead(ctx, team, otherID); err == nil {
		t.Error("non-lead should be rejected")
	}
}

func TestRequireLead_SystemBypass(t *testing.T) {
	leadID := uuid.New()
	otherID := uuid.New()
	team := makeTestTeam(leadID, nil)
	mgr := NewTeamToolManager(&mockTeamStore{}, &mockAgentStore{}, nil, "")

	for _, ch := range []string{ChannelTeammate, ChannelSystem} {
		ctx := makeCtx(otherID, "user1", "", ch)
		if err := mgr.requireLead(ctx, team, otherID); err != nil {
			t.Errorf("channel %q should bypass requireLead: %v", ch, err)
		}
	}
}
