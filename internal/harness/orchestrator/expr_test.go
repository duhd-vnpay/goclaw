package orchestrator

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEvalExpr_InList(t *testing.T) {
	vars := map[string]any{"pipeline_type": "full"}
	result, err := EvalExpr("variables.pipeline_type in ['full', 'new-feature']", vars)
	assert.NoError(t, err)
	assert.True(t, result)
}

func TestEvalExpr_NotInList(t *testing.T) {
	vars := map[string]any{"pipeline_type": "bug-fix"}
	result, err := EvalExpr("variables.pipeline_type in ['full', 'new-feature']", vars)
	assert.NoError(t, err)
	assert.False(t, result)
}

func TestEvalExpr_Comparison(t *testing.T) {
	vars := map[string]any{"compliance_tier": float64(2), "eval.score": 0.95}
	result, err := EvalExpr("variables.compliance_tier <= 2 && eval.score >= 0.9", vars)
	assert.NoError(t, err)
	assert.True(t, result)
}

func TestEvalExpr_EmptyExpression(t *testing.T) {
	result, err := EvalExpr("", nil)
	assert.NoError(t, err)
	assert.True(t, result)
}

func TestEvalExpr_Equality(t *testing.T) {
	vars := map[string]any{"status": "active"}
	result, err := EvalExpr("variables.status == active", vars)
	assert.NoError(t, err)
	assert.True(t, result)
}
