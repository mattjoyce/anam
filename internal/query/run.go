package query

import (
	"context"

	"github.com/mattjoyce/anam/internal/fold"
)

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
	prog, err := Parse(text)
	if err != nil {
		return Answer{}, rules, err
	}
	plan, err := Compile(BaseSchema, defs, rules.Program, prog)
	if err != nil {
		return Answer{}, rules, err
	}
	base, err := NewBase(served, versions, defs, ReadsAttr([]string{relation}, rules.Program, prog))
	if err != nil {
		return Answer{}, rules, err
	}
	res, err := Evaluate(ctx, plan, base, maxBytes)
	if err != nil {
		return Answer{}, rules, err
	}
	ans, err := res.Relation(relation)
	return ans, rules, err
}
