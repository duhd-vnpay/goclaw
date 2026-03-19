package access

import (
	"context"
	"testing"
)

func TestAppAccessChecker_MediaAllowed(t *testing.T) {
	checker := &AppAccessChecker{}
	req := AccessRequest{
		SubjectID:    "user1",
		SessionHash:  "a1b2c3d4e5f6",
		Resource:     "/app/workspace/.media/a1b2c3d4e5f6/file.jpg",
		ResourceType: "media",
		Action:       ActionRead,
		Source:       "http",
	}
	allowed, err := checker.CanAccess(context.Background(), req)
	if err != nil || !allowed {
		t.Errorf("expected allowed for matching session hash")
	}
}

func TestAppAccessChecker_MediaDenied(t *testing.T) {
	checker := &AppAccessChecker{}
	req := AccessRequest{
		SubjectID:    "user1",
		SessionHash:  "a1b2c3d4e5f6",
		Resource:     "/app/workspace/.media/DIFFERENT123/file.jpg",
		ResourceType: "media",
		Action:       ActionRead,
		Source:       "http",
	}
	allowed, _ := checker.CanAccess(context.Background(), req)
	if allowed {
		t.Error("expected denied for mismatched session hash")
	}
}

func TestAppAccessChecker_AdminBypass(t *testing.T) {
	checker := &AppAccessChecker{}
	req := AccessRequest{
		SubjectID:    "admin",
		Resource:     "/app/workspace/.media/anything/file.jpg",
		ResourceType: "media",
		Action:       ActionRead,
		IsAdmin:      true,
	}
	allowed, _ := checker.CanAccess(context.Background(), req)
	if !allowed {
		t.Error("expected admin bypass to be allowed")
	}
}

func TestAppAccessChecker_RecordAccess(t *testing.T) {
	checker := &AppAccessChecker{}
	// RecordAccess without AuditWriter should not panic
	err := checker.RecordAccess(context.Background(), AccessRequest{
		SubjectID: "user1",
		Action:    ActionRead,
		Resource:  "/test",
	}, true)
	if err != nil {
		t.Errorf("RecordAccess without writer: %v", err)
	}
}
