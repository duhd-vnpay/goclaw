package evaluation

// Rubric defines scoring dimensions and pass threshold for inferential evaluation.
type Rubric struct {
	Dimensions    []Dimension `json:"dimensions" yaml:"dimensions"`
	PassThreshold float64     `json:"pass_threshold" yaml:"pass_threshold"`
}

// Dimension is a single scoring criterion with weight.
type Dimension struct {
	Name        string        `json:"name" yaml:"name"`
	Weight      float64       `json:"weight" yaml:"weight"`
	Description string        `json:"description" yaml:"description"`
	ScoreGuide  []ScoreAnchor `json:"anchors,omitempty" yaml:"anchors"`
}

// ScoreAnchor maps a numeric score to a description for calibration.
type ScoreAnchor struct {
	Score   float64 `json:"score" yaml:"score"`
	Example string  `json:"example" yaml:"example"`
}

// FewShotExample is a calibration example for the LLM judge.
type FewShotExample struct {
	Input    string  `json:"input" yaml:"input"`
	Output   string  `json:"output" yaml:"output"`
	Score    float64 `json:"score" yaml:"score"`
	Feedback string  `json:"feedback" yaml:"feedback"`
}

// ComputeScore calculates the weighted average score across dimensions.
func (r *Rubric) ComputeScore(dimensionScores map[string]float64) float64 {
	var totalWeight, weightedSum float64
	for _, d := range r.Dimensions {
		if score, ok := dimensionScores[d.Name]; ok {
			weightedSum += score * d.Weight
			totalWeight += d.Weight
		}
	}
	if totalWeight == 0 {
		return 0
	}
	return weightedSum / totalWeight
}

// Passes returns true if the score meets or exceeds the threshold.
func (r *Rubric) Passes(score float64) bool {
	return score >= r.PassThreshold
}
