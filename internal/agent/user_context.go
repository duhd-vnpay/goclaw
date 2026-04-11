package agent

import (
	"fmt"
	"sort"
	"strings"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// BuildUserContextPrompt generates the "## Current User" system prompt block
// based on the resolved UserProfile. Returns empty string if profile is nil.
//
// Paired users get full context:
//
//	## Current User
//	- Name: Hoang Du
//	- Email: hoang@vnpay.vn
//	- Role: lead (Engineering)
//	- Departments: Engineering (Backend Lead)
//	- Expertise: golang, k8s, terraform
//	- Permissions: can_deploy, can_approve, can_merge
//
// Unpaired users get anonymous context.
func BuildUserContextPrompt(profile *store.UserProfile, channelType, senderID string) string {
	if profile == nil {
		return buildAnonymousPrompt(channelType, senderID)
	}
	return buildPairedPrompt(profile)
}

func buildPairedPrompt(p *store.UserProfile) string {
	var sb strings.Builder
	sb.WriteString("## Current User\n")

	// Name
	name := p.DisplayName
	if name == "" {
		name = p.Email
	}
	sb.WriteString("- Name: ")
	sb.WriteString(name)
	sb.WriteByte('\n')

	// Email
	sb.WriteString("- Email: ")
	sb.WriteString(p.Email)
	sb.WriteByte('\n')

	// Role (tenant role + primary department)
	if p.TenantRole != "" {
		sb.WriteString("- Tenant role: ")
		sb.WriteString(p.TenantRole)
		sb.WriteByte('\n')
	}
	if p.ProjectRole != "" {
		sb.WriteString("- Project role: ")
		sb.WriteString(p.ProjectRole)
		sb.WriteByte('\n')
	}

	// Departments
	if len(p.Departments) > 0 {
		sb.WriteString("- Departments: ")
		var parts []string
		for _, d := range p.Departments {
			entry := d.DepartmentName
			if d.Title != "" {
				entry += " (" + d.Title + ")"
			} else if d.Role != "" && d.Role != "member" {
				entry += " (" + d.Role + ")"
			}
			parts = append(parts, entry)
		}
		sb.WriteString(strings.Join(parts, ", "))
		sb.WriteByte('\n')
	}

	// Expertise
	if len(p.Expertise) > 0 {
		sb.WriteString("- Expertise: ")
		sb.WriteString(strings.Join(p.Expertise, ", "))
		sb.WriteByte('\n')
	}

	// Permissions
	if len(p.Permissions) > 0 {
		var perms []string
		for k, v := range p.Permissions {
			if v {
				perms = append(perms, k)
			}
		}
		if len(perms) > 0 {
			sort.Strings(perms) // deterministic output for LLM cache
			sb.WriteString("- Permissions: ")
			sb.WriteString(strings.Join(perms, ", "))
			sb.WriteByte('\n')
		}
	}

	// Timezone
	if p.Timezone != "" {
		sb.WriteString("- Timezone: ")
		sb.WriteString(p.Timezone)
		sb.WriteByte('\n')
	}

	// Availability
	if p.Availability != "" {
		sb.WriteString("- Availability: ")
		sb.WriteString(p.Availability)
		sb.WriteByte('\n')
	}

	return sb.String()
}

func buildAnonymousPrompt(channelType, senderID string) string {
	if channelType == "" && senderID == "" {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("## Current User\n")
	sb.WriteString(fmt.Sprintf("- Anonymous (channel: %s, sender: %s)\n", channelType, senderID))
	sb.WriteString("- Note: This user has not paired their account. Only safe read-only tools are available.\n")
	return sb.String()
}
