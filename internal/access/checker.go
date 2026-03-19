package access

import (
	"context"
	"strings"
)

// Action represents a file access action.
type Action string

const (
	ActionRead   Action = "read"
	ActionWrite  Action = "write"
	ActionDelete Action = "delete"
	ActionDeny   Action = "deny"
)

// AccessRequest describes a file access attempt.
type AccessRequest struct {
	SubjectID    string
	SessionHash  string // 12 hex chars from session key hash
	Resource     string
	ResourceType string // "media", "workspace", "team", "channel"
	Action       Action
	Source       string // "http", "tool", "channel_download"
	IPAddress    string
	IsAdmin      bool
}

// AccessChecker determines whether a subject can access a resource.
type AccessChecker interface {
	CanAccess(ctx context.Context, req AccessRequest) (bool, error)
	RecordAccess(ctx context.Context, req AccessRequest, allowed bool) error
}

// AppAccessChecker implements Phase A app-level access control.
type AppAccessChecker struct {
	AuditWriter *AuditWriter
}

// CanAccess checks if the request's session hash matches the resource's session directory.
func (c *AppAccessChecker) CanAccess(_ context.Context, req AccessRequest) (bool, error) {
	if req.IsAdmin {
		return true, nil
	}
	switch req.ResourceType {
	case "media":
		return strings.Contains(req.Resource, "/"+req.SessionHash+"/"), nil
	default:
		return req.SessionHash != "", nil
	}
}

// RecordAccess logs the access attempt via AuditWriter.
func (c *AppAccessChecker) RecordAccess(_ context.Context, req AccessRequest, allowed bool) error {
	if c.AuditWriter == nil {
		return nil
	}
	action := req.Action
	if !allowed {
		action = ActionDeny
	}
	c.AuditWriter.Log(context.Background(), FileAccessEvent{
		ActorID:      req.SubjectID,
		ActorType:    "user",
		Action:       string(action),
		Resource:     req.Resource,
		ResourceType: req.ResourceType,
		Source:       req.Source,
		IPAddress:    req.IPAddress,
	})
	return nil
}
