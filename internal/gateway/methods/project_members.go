package methods

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/gateway"
	"github.com/nextlevelbuilder/goclaw/internal/i18n"
	"github.com/nextlevelbuilder/goclaw/internal/permissions"
	"github.com/nextlevelbuilder/goclaw/internal/store"
	"github.com/nextlevelbuilder/goclaw/pkg/protocol"
)

// ProjectMembersMethods handles projects.members.* RPC methods.
type ProjectMembersMethods struct {
	memberStore store.ProjectMemberStore
}

// NewProjectMembersMethods creates a new ProjectMembersMethods handler.
func NewProjectMembersMethods(memberStore store.ProjectMemberStore) *ProjectMembersMethods {
	return &ProjectMembersMethods{memberStore: memberStore}
}

// Register registers project member management RPC methods.
func (m *ProjectMembersMethods) Register(router *gateway.MethodRouter) {
	router.Register(protocol.MethodProjectMembersList, m.handleList)
	router.Register(protocol.MethodProjectMembersAdd, m.handleAdd)
	router.Register(protocol.MethodProjectMembersRemove, m.handleRemove)
	router.Register(protocol.MethodProjectMembersUpdateRole, m.handleUpdateRole)
}

func (m *ProjectMembersMethods) handleList(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	locale := store.LocaleFromContext(ctx)

	var params struct {
		ProjectID string `json:"projectId"`
	}
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidJSON)))
			return
		}
	}

	projectID, err := uuid.Parse(params.ProjectID)
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidID, "projectId")))
		return
	}

	members, err := m.memberStore.List(ctx, projectID)
	if err != nil {
		slog.Error("projects.members.list failed", "error", err)
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, i18n.T(locale, i18n.MsgFailedToList, "project members")))
		return
	}
	if members == nil {
		members = []store.ProjectMemberData{}
	}
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]any{"members": members}))
}

func (m *ProjectMembersMethods) handleAdd(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	locale := store.LocaleFromContext(ctx)
	if !permissions.HasMinRole(client.Role(), permissions.RoleOperator) {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized, i18n.T(locale, i18n.MsgPermissionDenied, "projects.members.add")))
		return
	}

	var params struct {
		ProjectID   string          `json:"projectId"`
		UserID      string          `json:"userId"`
		Role        string          `json:"role"`
		Permissions json.RawMessage `json:"permissions"`
	}
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidJSON)))
			return
		}
	}

	projectID, err := uuid.Parse(params.ProjectID)
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidID, "projectId")))
		return
	}
	userID, err := uuid.Parse(params.UserID)
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidID, "userId")))
		return
	}
	if params.Role == "" {
		params.Role = "developer"
	}
	validRoles := map[string]bool{
		"owner": true, "lead": true, "developer": true, "reviewer": true, "viewer": true,
	}
	if !validRoles[params.Role] {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidRole)))
		return
	}

	member := &store.ProjectMemberData{
		ProjectID:   projectID,
		UserID:      userID,
		Role:        params.Role,
		Permissions: params.Permissions,
	}

	created, err := m.memberStore.Add(ctx, member)
	if err != nil {
		slog.Error("projects.members.add failed", "error", err)
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, i18n.T(locale, i18n.MsgFailedToCreate, "project member", err.Error())))
		return
	}
	client.SendResponse(protocol.NewOKResponse(req.ID, created))
}

func (m *ProjectMembersMethods) handleRemove(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	locale := store.LocaleFromContext(ctx)
	if !permissions.HasMinRole(client.Role(), permissions.RoleOperator) {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized, i18n.T(locale, i18n.MsgPermissionDenied, "projects.members.remove")))
		return
	}

	var params struct {
		ProjectID string `json:"projectId"`
		UserID    string `json:"userId"`
	}
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidJSON)))
			return
		}
	}

	projectID, err := uuid.Parse(params.ProjectID)
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidID, "projectId")))
		return
	}
	userID, err := uuid.Parse(params.UserID)
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidID, "userId")))
		return
	}

	if err := m.memberStore.Remove(ctx, projectID, userID); err != nil {
		slog.Error("projects.members.remove failed", "error", err)
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, i18n.T(locale, i18n.MsgFailedToDelete, "project member", err.Error())))
		return
	}
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]string{"ok": "true"}))
}

func (m *ProjectMembersMethods) handleUpdateRole(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	locale := store.LocaleFromContext(ctx)
	if !permissions.HasMinRole(client.Role(), permissions.RoleOperator) {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized, i18n.T(locale, i18n.MsgPermissionDenied, "projects.members.update_role")))
		return
	}

	var params struct {
		ProjectID   string          `json:"projectId"`
		UserID      string          `json:"userId"`
		Role        string          `json:"role"`
		Permissions json.RawMessage `json:"permissions"`
	}
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidJSON)))
			return
		}
	}

	projectID, err := uuid.Parse(params.ProjectID)
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidID, "projectId")))
		return
	}
	userID, err := uuid.Parse(params.UserID)
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidID, "userId")))
		return
	}
	if params.Role == "" {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgRequired, "role")))
		return
	}
	validRoles := map[string]bool{
		"owner": true, "lead": true, "developer": true, "reviewer": true, "viewer": true,
	}
	if !validRoles[params.Role] {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidRole)))
		return
	}

	if err := m.memberStore.UpdateRole(ctx, projectID, userID, params.Role, params.Permissions); err != nil {
		slog.Error("projects.members.update_role failed", "error", err)
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, i18n.T(locale, i18n.MsgFailedToUpdate, "project member role", err.Error())))
		return
	}
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]string{"ok": "true"}))
}
