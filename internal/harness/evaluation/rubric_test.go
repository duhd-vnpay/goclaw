package evaluation

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRubric_ComputeScore(t *testing.T) {
	r := Rubric{
		Dimensions: []Dimension{
			{Name: "completeness", Weight: 0.6},
			{Name: "security", Weight: 0.4},
		},
		PassThreshold: 0.7,
	}
	// (0.8*0.6 + 0.9*0.4) / (0.6+0.4) = 0.84
	score := r.ComputeScore(map[string]float64{"completeness": 0.8, "security": 0.9})
	assert.InDelta(t, 0.84, score, 0.001)
	assert.True(t, r.Passes(score))
}

func TestRubric_FailsBelowThreshold(t *testing.T) {
	r := Rubric{
		Dimensions:    []Dimension{{Name: "quality", Weight: 1.0}},
		PassThreshold: 0.7,
	}
	score := r.ComputeScore(map[string]float64{"quality": 0.5})
	assert.InDelta(t, 0.5, score, 0.001)
	assert.False(t, r.Passes(score))
}

func TestRubric_ComputeScore_NoDimensions(t *testing.T) {
	r := Rubric{Dimensions: nil, PassThreshold: 0.5}
	score := r.ComputeScore(map[string]float64{"x": 1.0})
	assert.Equal(t, 0.0, score)
}
