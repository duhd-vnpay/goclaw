package orchestrator

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkflowLoader_LoadFile(t *testing.T) {
	dir := t.TempDir()
	content := `
id: "test-workflow"
name: "Test Workflow"
version: "1.0.0"
trigger:
  type: manual
steps:
  - id: step1
    name: "Step One"
    agent: test-agent
    task: "Do something"
    depends_on: []
    harness:
      context_strategy: reset
      evaluation:
        computational: [no_secrets]
        max_rounds: 3
    gate:
      type: human
      approver: admin
on_failure:
  default: stop
`
	path := filepath.Join(dir, "test.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))

	loader := NewWorkflowLoader(dir)
	wf, err := loader.LoadFile(path)
	require.NoError(t, err)

	assert.Equal(t, "test-workflow", wf.ID)
	assert.Equal(t, "Test Workflow", wf.Name)
	assert.Len(t, wf.Steps, 1)
	assert.Equal(t, "step1", wf.Steps[0].ID)
	assert.Equal(t, "reset", wf.Steps[0].Harness.ContextStrategy)
	assert.Equal(t, "human", wf.Steps[0].Gate.Type)
	assert.Equal(t, "stop", wf.OnFailure.Default)
}

func TestWorkflowLoader_LoadAll(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"wf1.yaml", "wf2.yaml"} {
		content := `id: "` + name[:3] + `"
name: "` + name + `"
version: "1.0"
trigger:
  type: manual
steps: []
on_failure:
  default: stop
`
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0644))
	}

	loader := NewWorkflowLoader(dir)
	workflows, err := loader.LoadAll()
	require.NoError(t, err)
	assert.Len(t, workflows, 2)
}
