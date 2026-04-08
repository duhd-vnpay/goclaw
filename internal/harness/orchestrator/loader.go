package orchestrator

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// WorkflowLoader reads workflow YAML files from a directory.
type WorkflowLoader struct {
	dir string
}

// NewWorkflowLoader creates a loader for the given directory.
func NewWorkflowLoader(dir string) *WorkflowLoader {
	return &WorkflowLoader{dir: dir}
}

// LoadAll loads all .yaml workflows from the directory.
func (l *WorkflowLoader) LoadAll() (map[string]Workflow, error) {
	workflows := make(map[string]Workflow)

	files, err := filepath.Glob(filepath.Join(l.dir, "*.yaml"))
	if err != nil {
		return nil, fmt.Errorf("glob workflows: %w", err)
	}

	for _, f := range files {
		wf, err := l.LoadFile(f)
		if err != nil {
			return nil, fmt.Errorf("load %s: %w", f, err)
		}
		workflows[wf.ID] = wf
	}

	return workflows, nil
}

// LoadFile loads a single workflow YAML file.
func (l *WorkflowLoader) LoadFile(path string) (Workflow, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Workflow{}, err
	}

	var wf Workflow
	if err := yaml.Unmarshal(data, &wf); err != nil {
		return Workflow{}, fmt.Errorf("parse %s: %w", path, err)
	}

	return wf, nil
}
