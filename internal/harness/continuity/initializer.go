package continuity

import (
	"fmt"
	"strings"
)

// BuildResumeContext generates a system prompt section from a handoff artifact.
// Returns empty string if artifact is nil (first session, no resume needed).
func BuildResumeContext(artifact *HandoffArtifact) string {
	if artifact == nil {
		return ""
	}

	var b strings.Builder
	fmt.Fprintf(&b, "## Session Continuity — Resuming from Session %d\n\n", artifact.Sequence)
	fmt.Fprintf(&b, "### Objective\n%s\n\n", artifact.Objective)
	fmt.Fprintf(&b, "### Progress (%d%% complete)\n", artifact.Progress.PercentDone)

	if len(artifact.Progress.CompletedTasks) > 0 {
		fmt.Fprintf(&b, "**Done:** %s\n", strings.Join(artifact.Progress.CompletedTasks, ", "))
	}
	if artifact.Progress.CurrentTask != "" {
		fmt.Fprintf(&b, "**Current:** %s\n", artifact.Progress.CurrentTask)
	}
	if len(artifact.Progress.RemainingTasks) > 0 {
		fmt.Fprintf(&b, "**Remaining:** %s\n", strings.Join(artifact.Progress.RemainingTasks, ", "))
	}
	if len(artifact.Progress.BlockedTasks) > 0 {
		fmt.Fprintf(&b, "**Blocked:** %s\n", strings.Join(artifact.Progress.BlockedTasks, ", "))
	}

	if len(artifact.Decisions) > 0 {
		b.WriteString("\n### Key Decisions\n")
		for _, d := range artifact.Decisions {
			fmt.Fprintf(&b, "- **%s** — %s\n", d.What, d.Why)
		}
	}

	if len(artifact.OpenQuestions) > 0 {
		b.WriteString("\n### Open Questions\n")
		for _, q := range artifact.OpenQuestions {
			fmt.Fprintf(&b, "- %s\n", q)
		}
	}

	if artifact.GitBranch != "" {
		fmt.Fprintf(&b, "\n### Git State\nBranch: `%s`", artifact.GitBranch)
		if artifact.GitCommit != "" {
			end := 8
			if end > len(artifact.GitCommit) {
				end = len(artifact.GitCommit)
			}
			fmt.Fprintf(&b, " @ `%s`", artifact.GitCommit[:end])
		}
		b.WriteString("\n")
	}

	b.WriteString("\nContinue from where the previous session left off. Do NOT re-do completed work.\n")
	return b.String()
}
