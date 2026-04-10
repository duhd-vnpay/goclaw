package constraints

import (
	"testing"
	"github.com/stretchr/testify/assert"
)

func TestJSONValidTest_Pass(t *testing.T) {
	test := &JSONValidTest{}
	pass, feedback := test.Validate(`{"key": "value", "num": 42}`)
	assert.True(t, pass)
	assert.Empty(t, feedback)
}

func TestJSONValidTest_Fail(t *testing.T) {
	test := &JSONValidTest{}
	pass, feedback := test.Validate(`{"key": "value",}`)
	assert.False(t, pass)
	assert.Contains(t, feedback, "invalid JSON")
}

func TestNoSecretsTest_DetectsAPIKey(t *testing.T) {
	test := NewNoSecretsTest()
	pass, feedback := test.Validate(`config.api_key = "sk-proj-abc123def456"`)
	assert.False(t, pass)
	assert.Contains(t, feedback, "secret")
}

func TestNoSecretsTest_PassClean(t *testing.T) {
	test := NewNoSecretsTest()
	pass, _ := test.Validate(`config.timeout = 30`)
	assert.True(t, pass)
}

func TestRequiredSectionsTest_Pass(t *testing.T) {
	test := NewRequiredSectionsTest([]string{"## Summary", "## Test Plan"})
	pass, _ := test.Validate("## Summary\nContent\n## Test Plan\nTests here")
	assert.True(t, pass)
}

func TestRequiredSectionsTest_Missing(t *testing.T) {
	test := NewRequiredSectionsTest([]string{"## Summary", "## Test Plan"})
	pass, feedback := test.Validate("## Summary\nContent only")
	assert.False(t, pass)
	assert.Contains(t, feedback, "## Test Plan")
}
