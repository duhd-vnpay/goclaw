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

// DepartmentsMethods handles departments.* RPC methods.
type DepartmentsMethods struct {
	deptStore store.DepartmentStore
}

// NewDepartmentsMethods creates a new DepartmentsMethods handler.
func NewDepartmentsMethods(deptStore store.DepartmentStore) *DepartmentsMethods {
	return &DepartmentsMethods{deptStore: deptStore}
}

// Register registers department management RPC methods.
func (m *DepartmentsMethods) Register(router *gateway.MethodRouter) {
	router.Register(protocol.MethodDepartmentsList, m.handleList)
	router.Register(protocol.MethodDepartmentsCreate, m.handleCreate)
	router.Register(protocol.MethodDepartmentsUpdate, m.handleUpdate)
	router.Register(protocol.MethodDepartmentsDelete, m.handleDelete)
	router.Register(protocol.MethodDepartmentsMembersList, m.handleMembersList)
	router.Register(protocol.MethodDepartmentsMembersAdd, m.handleMembersAdd)
	router.Register(protocol.MethodDepartmentsMembersRemove, m.handleMembersRemove)
}

func (m *DepartmentsMethods) handleList(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	locale := store.LocaleFromContext(ctx)

	depts, err := m.deptStore.List(ctx)
	if err != nil {
		slog.Error("departments.list failed", "error", err)
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, i18n.T(locale, i18n.MsgFailedToList, "departments")))
		return
	}
	if depts == nil {
		depts = []store.DepartmentData{}
	}
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]any{"departments": depts}))
}

func (m *DepartmentsMethods) handleCreate(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	locale := store.LocaleFromContext(ctx)
	if !permissions.HasMinRole(client.Role(), permissions.RoleAdmin) {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized, i18n.T(locale, i18n.MsgPermissionDenied, "departments.create")))
		return
	}

	var params struct {
		Name        string  `json:"name"`
		Slug        string  `json:"slug"`
		ParentID    *string `json:"parentId"`
		HeadUserID  *string `json:"headUserId"`
		Description *string `json:"description"`
	}
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidJSON)))
			return
		}
	}

	if params.Name == "" {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgRequired, "name")))
		return
	}
	if params.Slug == "" {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgRequired, "slug")))
		return
	}
	if !slugRe.MatchString(params.Slug) {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidSlug, "slug")))
		return
	}

	dept := &store.DepartmentData{
		Name:        params.Name,
		Slug:        params.Slug,
		Description: params.Description,
	}

	if params.ParentID != nil {
		pid, err := uuid.Parse(*params.ParentID)
		if err != nil {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidID, "parentId")))
			return
		}
		dept.ParentID = &pid
	}
	if params.HeadUserID != nil {
		hid, err := uuid.Parse(*params.HeadUserID)
		if err != nil {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidID, "headUserId")))
			return
		}
		dept.HeadUserID = &hid
	}

	created, err := m.deptStore.Create(ctx, dept)
	if err != nil {
		slog.Error("departments.create failed", "error", err)
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, i18n.T(locale, i18n.MsgFailedToCreate, "department", err.Error())))
		return
	}
	client.SendResponse(protocol.NewOKResponse(req.ID, created))
}

func (m *DepartmentsMethods) handleUpdate(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	locale := store.LocaleFromContext(ctx)
	if !permissions.HasMinRole(client.Role(), permissions.RoleAdmin) {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized, i18n.T(locale, i18n.MsgPermissionDenied, "departments.update")))
		return
	}

	var params struct {
		ID          string  `json:"id"`
		Name        *string `json:"name"`
		Slug        *string `json:"slug"`
		ParentID    *string `json:"parentId"`
		HeadUserID  *string `json:"headUserId"`
		Description *string `json:"description"`
	}
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidJSON)))
			return
		}
	}

	id, err := uuid.Parse(params.ID)
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidID, "department")))
		return
	}

	updates := make(map[string]any)
	if params.Name != nil {
		updates["name"] = *params.Name
	}
	if params.Slug != nil {
		if !slugRe.MatchString(*params.Slug) {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidSlug, "slug")))
			return
		}
		updates["slug"] = *params.Slug
	}
	if params.ParentID != nil {
		pid, err := uuid.Parse(*params.ParentID)
		if err != nil {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidID, "parentId")))
			return
		}
		updates["parent_id"] = pid
	}
	if params.HeadUserID != nil {
		hid, err := uuid.Parse(*params.HeadUserID)
		if err != nil {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidID, "headUserId")))
			return
		}
		updates["head_user_id"] = hid
	}
	if params.Description != nil {
		updates["description"] = *params.Description
	}

	if len(updates) == 0 {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidUpdates)))
		return
	}

	if err := m.deptStore.Update(ctx, id, updates); err != nil {
		slog.Error("departments.update failed", "error", err)
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, i18n.T(locale, i18n.MsgFailedToUpdate, "department", err.Error())))
		return
	}
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]string{"ok": "true"}))
}

func (m *DepartmentsMethods) handleDelete(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	locale := store.LocaleFromContext(ctx)
	if !permissions.HasMinRole(client.Role(), permissions.RoleAdmin) {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized, i18n.T(locale, i18n.MsgPermissionDenied, "departments.delete")))
		return
	}

	var params struct {
		ID string `json:"id"`
	}
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidJSON)))
			return
		}
	}

	id, err := uuid.Parse(params.ID)
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidID, "department")))
		return
	}

	if err := m.deptStore.Delete(ctx, id); err != nil {
		slog.Error("departments.delete failed", "error", err)
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, i18n.T(locale, i18n.MsgFailedToDelete, "department", err.Error())))
		return
	}
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]string{"ok": "true"}))
}

func (m *DepartmentsMethods) handleMembersList(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	locale := store.LocaleFromContext(ctx)

	var params struct {
		DepartmentID string `json:"departmentId"`
	}
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidJSON)))
			return
		}
	}

	deptID, err := uuid.Parse(params.DepartmentID)
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidID, "departmentId")))
		return
	}

	members, err := m.deptStore.ListMembers(ctx, deptID)
	if err != nil {
		slog.Error("departments.members.list failed", "error", err)
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, i18n.T(locale, i18n.MsgFailedToList, "department members")))
		return
	}
	if members == nil {
		members = []store.DepartmentMemberData{}
	}
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]any{"members": members}))
}

func (m *DepartmentsMethods) handleMembersAdd(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	locale := store.LocaleFromContext(ctx)
	if !permissions.HasMinRole(client.Role(), permissions.RoleAdmin) {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized, i18n.T(locale, i18n.MsgPermissionDenied, "departments.members.add")))
		return
	}

	var params struct {
		DepartmentID string  `json:"departmentId"`
		UserID       string  `json:"userId"`
		Role         string  `json:"role"`
		Title        *string `json:"title"`
	}
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidJSON)))
			return
		}
	}

	deptID, err := uuid.Parse(params.DepartmentID)
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidID, "departmentId")))
		return
	}
	userID, err := uuid.Parse(params.UserID)
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidID, "userId")))
		return
	}
	if params.Role == "" {
		params.Role = "member"
	}
	validRoles := map[string]bool{"head": true, "lead": true, "member": true}
	if !validRoles[params.Role] {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidRole)))
		return
	}

	member := &store.DepartmentMemberData{
		DepartmentID: deptID,
		UserID:       userID,
		Role:         params.Role,
		Title:        params.Title,
	}

	created, err := m.deptStore.AddMember(ctx, member)
	if err != nil {
		slog.Error("departments.members.add failed", "error", err)
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, i18n.T(locale, i18n.MsgFailedToCreate, "department member", err.Error())))
		return
	}
	client.SendResponse(protocol.NewOKResponse(req.ID, created))
}

func (m *DepartmentsMethods) handleMembersRemove(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	locale := store.LocaleFromContext(ctx)
	if !permissions.HasMinRole(client.Role(), permissions.RoleAdmin) {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized, i18n.T(locale, i18n.MsgPermissionDenied, "departments.members.remove")))
		return
	}

	var params struct {
		DepartmentID string `json:"departmentId"`
		UserID       string `json:"userId"`
	}
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidJSON)))
			return
		}
	}

	deptID, err := uuid.Parse(params.DepartmentID)
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidID, "departmentId")))
		return
	}
	userID, err := uuid.Parse(params.UserID)
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidID, "userId")))
		return
	}

	if err := m.deptStore.RemoveMember(ctx, deptID, userID); err != nil {
		slog.Error("departments.members.remove failed", "error", err)
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, i18n.T(locale, i18n.MsgFailedToDelete, "department member", err.Error())))
		return
	}
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]string{"ok": "true"}))
}
