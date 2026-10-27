package query

import (
	"context"

	"github.com/mattjoyce/anam/internal/fold"
	"github.com/mattjoyce/tarski"
)

type prepared struct {
	plan Plan
	base Base
}

// prepare parses, compiles and builds the base exactly as RunWith does.
func prepare(rules RuleSet, served fold.Value, versions map[string]int, defs Definitions,
	text, relation string,
) (prepared, error) {
	prog, err := Parse(text)
	if err != nil {
		return prepared{}, err
	}
	plan, err := Compile(BaseSchema, defs, rules.Program, prog)
	if err != nil {
		return prepared{}, err
	}
	base, err := NewBase(served, versions, defs, ReadsAttr([]string{relation}, rules.Program, prog))
	if err != nil {
		return prepared{}, err
	}
	return prepared{plan: plan, base: base}, nil
}

// Run is one query end to end over a served value: parse text, compile it with the
// bundled rules against the Definitions, evaluate within maxBytes (zero for none),
// and answer relation. It is what the read and the receipt's verify both call, so a
// receipt replays exactly the derivation that answered.
func Run(ctx context.Context, served fold.Value, versions map[string]int, defs Definitions,
	text, relation string, maxBytes int,
) (Answer, RuleSet, error) {
	rules, err := Bundled()
	if err != nil {
		return Answer{}, RuleSet{}, err
	}
	ans, err := RunWith(ctx, rules, served, versions, defs, text, relation, maxBytes)
	return ans, rules, err
}

// RunWith is Run under a rule set the caller chose, which is how a receipt replays
// under the text it was answered with (HeldRuleSet) and not the one this binary
// bundles now.
func RunWith(ctx context.Context, rules RuleSet, served fold.Value, versions map[string]int, defs Definitions,
	text, relation string, maxBytes int,
) (Answer, error) {
	prep, err := prepare(rules, served, versions, defs, text, relation)
	if err != nil {
		return Answer{}, err
	}
	res, err := Evaluate(ctx, prep.plan, prep.base, maxBytes)
	if err != nil {
		return Answer{}, err
	}
	return res.Relation(relation)
}

// Cost runs text over served as Run does, under the bundled rules, with no bound, and
// returns the number of tuples relation holds and the work tarski charged: the steps
// Options.MaxSteps bounds and the derived bytes Options.MaxBytes bounds. It is the
// measurement a bound is set from; no read calls it.
func Cost(ctx context.Context, served fold.Value, versions map[string]int, defs Definitions,
	text, relation string,
) (tuples, steps, bytes int, err error) {
	rules, err := Bundled()
	if err != nil {
		return 0, 0, 0, err
	}
	prep, err := prepare(rules, served, versions, defs, text, relation)
	if err != nil {
		return 0, 0, 0, err
	}
	res, err := tarski.Evaluate(ctx, prep.plan, prep.base, tarski.Options{})
	if err != nil {
		return 0, 0, 0, err
	}
	steps, bytes = res.Cost()
	ans, err := res.Relation(relation)
	if err != nil {
		return 0, 0, 0, err
	}
	return len(ans.Tuples()), steps, bytes, nil
}
