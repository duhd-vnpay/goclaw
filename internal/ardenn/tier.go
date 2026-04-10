package ardenn

import (
	"fmt"
	"strings"
)

type Tier int

const (
	TierLight    Tier = 1 // L4 only
	TierStandard Tier = 2 // L1 + L4
	TierFull     Tier = 3 // L1 + L2 + L3 + L4
)

func (t Tier) String() string {
	switch t {
	case TierLight:
		return "light"
	case TierStandard:
		return "standard"
	case TierFull:
		return "full"
	default:
		return "unknown"
	}
}

func ParseTier(s string) (Tier, error) {
	switch strings.ToLower(s) {
	case "light":
		return TierLight, nil
	case "standard":
		return TierStandard, nil
	case "full":
		return TierFull, nil
	default:
		return 0, fmt.Errorf("unknown tier: %q", s)
	}
}

type Layer int

const (
	LayerConstraints   Layer = iota // L1
	LayerContinuity                 // L2
	LayerEvaluation                 // L3
	LayerOrchestration              // L4
)

func (t Tier) Has(l Layer) bool {
	switch l {
	case LayerOrchestration:
		return true
	case LayerConstraints:
		return t >= TierStandard
	case LayerContinuity:
		return t >= TierFull
	case LayerEvaluation:
		return t >= TierFull
	default:
		return false
	}
}
