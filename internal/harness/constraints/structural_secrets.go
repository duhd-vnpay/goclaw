package constraints

import "regexp"

var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(api[_-]?key|secret[_-]?key|password|token)\s*[:=]\s*['"][^'"]{8,}`),
	regexp.MustCompile(`(?i)sk-[a-zA-Z0-9]{20,}`),
	regexp.MustCompile(`(?i)ghp_[a-zA-Z0-9]{36}`),
	regexp.MustCompile(`(?i)glpat-[a-zA-Z0-9\-_]{20,}`),
}

type NoSecretsTest struct{}

func NewNoSecretsTest() *NoSecretsTest { return &NoSecretsTest{} }

func (t *NoSecretsTest) Name() string                    { return "no_secrets" }
func (t *NoSecretsTest) Applies(_ string, _ string) bool { return true }

func (t *NoSecretsTest) Validate(output string) (bool, string) {
	for _, p := range secretPatterns {
		if loc := p.FindStringIndex(output); loc != nil {
			end := loc[0] + 40
			if end > loc[1] {
				end = loc[1]
			}
			snippet := output[loc[0]:end]
			return false, "Possible secret detected in output: \"" + snippet + "...\". Remove or mask credentials before outputting."
		}
	}
	return true, ""
}
