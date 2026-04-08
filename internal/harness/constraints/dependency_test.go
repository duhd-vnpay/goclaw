package constraints

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDependencyEngine_ValidateCall_AllowedLayer(t *testing.T) {
	cfg := DependencyConfig{
		Layers: []DependencyLayer{
			{Name: "planning", Order: 1, AllowsUp: []string{"coding"}},
			{Name: "coding", Order: 2, AllowsUp: []string{"testing"}},
			{Name: "testing", Order: 3, AllowsUp: []string{"review"}},
		},
		AgentMapping: map[string]string{
			"planner-agent": "planning",
			"coder-agent":   "coding",
			"tester-agent":  "testing",
		},
		Enforcement: "block",
	}
	engine := NewDependencyEngine(cfg)
	err := engine.ValidateCall("planner-agent", "coder-agent")
	assert.NoError(t, err)
	err = engine.ValidateCall("planner-agent", "tester-agent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "dependency layer violation")
}

func TestDependencyEngine_ValidateCall_UnmappedAgent(t *testing.T) {
	cfg := DependencyConfig{
		Layers:       []DependencyLayer{{Name: "coding", Order: 1}},
		AgentMapping: map[string]string{},
		Enforcement:  "block",
	}
	engine := NewDependencyEngine(cfg)
	err := engine.ValidateCall("unknown-agent", "another-agent")
	assert.NoError(t, err)
}

func TestDependencyEngine_ValidateCall_WarnMode(t *testing.T) {
	cfg := DependencyConfig{
		Layers: []DependencyLayer{
			{Name: "planning", Order: 1, AllowsUp: []string{}},
			{Name: "coding", Order: 2, AllowsUp: []string{}},
		},
		AgentMapping: map[string]string{"planner": "planning", "coder": "coding"},
		Enforcement:  "warn",
	}
	engine := NewDependencyEngine(cfg)
	err := engine.ValidateCall("planner", "coder")
	assert.NoError(t, err)
	assert.Len(t, engine.Warnings(), 1)
}
