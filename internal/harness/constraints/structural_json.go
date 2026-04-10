package constraints

import "encoding/json"

type JSONValidTest struct{}

func (t *JSONValidTest) Name() string { return "json_valid" }

func (t *JSONValidTest) Applies(toolName, output string) bool {
	if len(output) < 2 {
		return false
	}
	return (output[0] == '{' || output[0] == '[')
}

func (t *JSONValidTest) Validate(output string) (bool, string) {
	if json.Valid([]byte(output)) {
		return true, ""
	}
	return false, "invalid JSON: output starts with '{' or '[' but does not parse as valid JSON. Check for trailing commas, missing quotes, or unescaped characters."
}
