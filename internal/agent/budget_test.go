package agent

import (
	"context"
	"testing"

	"github.com/maestroi/gamevision/internal/inference"
)

type budgetCompleter struct {
	min int
}

func (b *budgetCompleter) EnsureMaxTokens(min int) {
	b.min = min
}

func (b *budgetCompleter) Complete(context.Context, string, []byte) (inference.Result, error) {
	return inference.Result{}, nil
}

func TestNewEnsuresPolicyTokenBudget(t *testing.T) {
	c := &budgetCompleter{}
	_ = New(c)
	if c.min != minPolicyTokens {
		t.Fatalf("min token budget=%d want %d", c.min, minPolicyTokens)
	}
}
