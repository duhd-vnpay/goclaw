// internal/ardenn/guards/token_limit_guard.go
package guards

import (
	"context"
	"fmt"

	"github.com/nextlevelbuilder/goclaw/internal/ardenn"
)

const defaultTokenLimit = 100000

// TokenLimitGuard checks that task input does not exceed the configured token limit.
// Limit comes from Constraints map: {"token_limit": 50000} or falls back to default.
type TokenLimitGuard struct {
	// estimateTokens estimates token count for a string.
	// Inject tokencount.CountTokens or a simple len/4 estimator.
	estimateTokens func(s string) int
}

// NewTokenLimitGuard creates a token limit guard with the given estimator.
// If estimator is nil, uses a simple len/4 heuristic.
func NewTokenLimitGuard(estimator func(string) int) *TokenLimitGuard {
	if estimator == nil {
		estimator = func(s string) int { return len(s) / 4 }
	}
	return &TokenLimitGuard{estimateTokens: estimator}
}

func (g *TokenLimitGuard) Name() string { return "token_limit" }

func (g *TokenLimitGuard) Check(_ context.Context, cc ardenn.ConstraintContext) ardenn.GuardResult {
	limit := defaultTokenLimit
	if raw, ok := cc.Metadata["token_limit"]; ok {
		switch v := raw.(type) {
		case float64:
			limit = int(v)
		case int:
			limit = v
		}
	}

	tokens := g.estimateTokens(cc.Input)
	if tokens > limit {
		return ardenn.GuardResult{
			Name: g.Name(), Pass: false,
			Reason:   fmt.Sprintf("input tokens (%d) exceed limit (%d)", tokens, limit),
			Severity: "block",
		}
	}

	return ardenn.GuardResult{Name: g.Name(), Pass: true, Severity: "block"}
}
