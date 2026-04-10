package ardenn

import "testing"

func TestParseTier(t *testing.T) {
	tests := []struct {
		input string
		want  Tier
		err   bool
	}{
		{"full", TierFull, false},
		{"standard", TierStandard, false},
		{"light", TierLight, false},
		{"FULL", TierFull, false},
		{"invalid", 0, true},
		{"", 0, true},
	}
	for _, tt := range tests {
		got, err := ParseTier(tt.input)
		if (err != nil) != tt.err {
			t.Errorf("ParseTier(%q) error = %v, wantErr %v", tt.input, err, tt.err)
			continue
		}
		if got != tt.want {
			t.Errorf("ParseTier(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestTierHasLayer(t *testing.T) {
	if TierLight.Has(LayerConstraints) {
		t.Error("light tier should not have constraints")
	}
	if !TierLight.Has(LayerOrchestration) {
		t.Error("light tier should have orchestration")
	}
	if !TierStandard.Has(LayerConstraints) {
		t.Error("standard tier should have constraints")
	}
	if TierStandard.Has(LayerEvaluation) {
		t.Error("standard tier should not have evaluation")
	}
	if !TierStandard.Has(LayerOrchestration) {
		t.Error("standard tier should have orchestration")
	}
	if !TierFull.Has(LayerConstraints) {
		t.Error("full tier should have constraints")
	}
	if !TierFull.Has(LayerContinuity) {
		t.Error("full tier should have continuity")
	}
	if !TierFull.Has(LayerEvaluation) {
		t.Error("full tier should have evaluation")
	}
	if !TierFull.Has(LayerOrchestration) {
		t.Error("full tier should have orchestration")
	}
}
