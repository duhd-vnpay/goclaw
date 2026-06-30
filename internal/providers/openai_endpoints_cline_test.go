package providers

import "testing"

func TestOpenAIProvider_isClineEndpoint(t *testing.T) {
	cases := []struct {
		name    string
		apiBase string
		pname   string
		want    bool
	}{
		{"cline api host", "https://api.cline.bot/api/v1", "cline", true},
		{"cline name only", "https://custom-proxy.example.com/v1", "cline", true},
		{"cline name uppercase", "https://x/v1", "Cline", true},
		{"cline name with whitespace", "https://x/v1", " cline ", true},
		{"cline subpath only (host matches)", "https://api.cline.bot/v2", "anything", true},
		{"openai native", "https://api.openai.com/v1", "openai", false},
		{"openrouter", "https://openrouter.ai/api/v1", "openrouter", false},
		{"dashscope", "https://dashscope.aliyuncs.com/v1", "qwen", false},
		{"empty", "", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := &OpenAIProvider{apiBase: tc.apiBase, name: tc.pname}
			if got := p.isClineEndpoint(); got != tc.want {
				t.Errorf("isClineEndpoint() = %v, want %v (apiBase=%q name=%q)",
					got, tc.want, tc.apiBase, tc.pname)
			}
		})
	}
}
