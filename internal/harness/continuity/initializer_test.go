package continuity

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBuildResumeContext_NoArtifact(t *testing.T) {
	result := BuildResumeContext(nil)
	assert.Empty(t, result)
}

func TestBuildResumeContext_WithArtifact(t *testing.T) {
	a := &HandoffArtifact{
		Sequence:  2,
		Objective: "Build payment QR feature",
		Progress: Progress{
			CompletedTasks: []string{"PRD review", "Architecture design"},
			CurrentTask:    "Security threat model",
			RemainingTasks: []string{"Code review", "QA testing"},
			PercentDone:    40,
		},
		Decisions: []Decision{
			{What: "Chose FastAPI over Spring Boot", Why: "Faster prototyping"},
		},
		OpenQuestions: []string{"PCI scope for QR tokens?"},
		GitBranch:     "feat/payment-qr",
		GitCommit:     "abc12345def",
	}

	result := BuildResumeContext(a)
	assert.Contains(t, result, "Session 2")
	assert.Contains(t, result, "Build payment QR feature")
	assert.Contains(t, result, "40%")
	assert.Contains(t, result, "PRD review")
	assert.Contains(t, result, "Security threat model")
	assert.Contains(t, result, "Chose FastAPI over Spring Boot")
	assert.Contains(t, result, "PCI scope for QR tokens?")
	assert.Contains(t, result, "feat/payment-qr")
	assert.Contains(t, result, "abc12345")
	assert.Contains(t, result, "Do NOT re-do completed work")
}
