package constraints

import "strings"

type RequiredSectionsTest struct {
	sections []string
}

func NewRequiredSectionsTest(sections []string) *RequiredSectionsTest {
	return &RequiredSectionsTest{sections: sections}
}

func (t *RequiredSectionsTest) Name() string                    { return "required_sections" }
func (t *RequiredSectionsTest) Applies(_ string, _ string) bool { return true }

func (t *RequiredSectionsTest) Validate(output string) (bool, string) {
	var missing []string
	for _, s := range t.sections {
		if !strings.Contains(output, s) {
			missing = append(missing, s)
		}
	}
	if len(missing) > 0 {
		return false, "Missing required sections: " + strings.Join(missing, ", ") + ". Add these sections to your output."
	}
	return true, ""
}
