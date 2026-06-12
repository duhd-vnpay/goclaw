package acp

import "testing"

func TestParseTeamIDFromSessionKey(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			"team session",
			"agent:dwight-research:team:019e4d7f-91aa-73f4-a5b6-42d3e40edb7f:019ebb79-25f4-7caa-a961-6887f124e9ab",
			"019e4d7f-91aa-73f4-a5b6-42d3e40edb7f",
		},
		{
			"cron session",
			"agent:litellm-daily-ops:cron:abc123",
			"",
		},
		{
			"malformed missing teamID",
			"agent:x:team::xyz",
			"",
		},
		{
			"empty input",
			"",
			"",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseTeamIDFromSessionKey(tc.in)
			if got != tc.want {
				t.Errorf("ParseTeamIDFromSessionKey(%q)=%q want %q", tc.in, got, tc.want)
			}
		})
	}
}
