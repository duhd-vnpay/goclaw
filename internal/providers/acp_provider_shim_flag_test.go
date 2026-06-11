package providers

import (
	"context"
	"errors"
	"testing"
)

// fakeACPCfgReader is a minimal in-memory ACPConfigReader for the kill-switch tests.
type fakeACPCfgReader struct {
	value string
	err   error
	miss  bool // when true, return "" + nil error (simulates "row missing")
}

func (f fakeACPCfgReader) GetSystemConfig(_ context.Context, _ string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	if f.miss {
		return "", nil
	}
	return f.value, nil
}

// TestShimEnabled_DefaultsOpen covers Task 9 fail-OPEN semantics: a missing
// row, a transient read error, or no reader wired must NOT disable the
// shim. Only an explicit "false" (case-insensitive) routes through the
// legacy path.
func TestShimEnabled_DefaultsOpen(t *testing.T) {
	cases := []struct {
		name   string
		reader ACPConfigReader
		want   bool
	}{
		{"no reader wired", nil, true},
		{"row missing", fakeACPCfgReader{miss: true}, true},
		{"read error", fakeACPCfgReader{err: errors.New("transient")}, true},
		{"value true", fakeACPCfgReader{value: "true"}, true},
		{"value TRUE", fakeACPCfgReader{value: "TRUE"}, true},
		{"value 1", fakeACPCfgReader{value: "1"}, true},
		{"value yes", fakeACPCfgReader{value: "yes"}, true},
		{"value on", fakeACPCfgReader{value: "on"}, true},
		{"value with whitespace", fakeACPCfgReader{value: "  true  "}, true},
		{"garbage value falls open with warn", fakeACPCfgReader{value: "maybe"}, true},
		{"empty string falls open", fakeACPCfgReader{value: ""}, true},
		{"value false", fakeACPCfgReader{value: "false"}, false},
		{"value FALSE", fakeACPCfgReader{value: "FALSE"}, false},
		{"value 0", fakeACPCfgReader{value: "0"}, false},
		{"value no", fakeACPCfgReader{value: "no"}, false},
		{"value off", fakeACPCfgReader{value: "off"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := &ACPProvider{cfgReader: tc.reader}
			got := p.shimEnabled(context.Background())
			if got != tc.want {
				t.Errorf("shimEnabled(%q) = %v, want %v", tc.name, got, tc.want)
			}
		})
	}
}
