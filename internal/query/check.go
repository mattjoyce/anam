package query

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/mattjoyce/anam/internal/fold"
)

// Disagreement is one instant at which the bundled current/2 and fold.Snapshot
// differ: the ids each holds, sorted.
type Disagreement struct {
	At       string
	Current  []string
	Snapshot []string
}

// Report is what a Check found: how many instants it compared, how long they took,
// and the first disagreement if there was one.
type Report struct {
	FactTimes int // distinct fact-times in the value
	Instants  int // instants compared
	Elapsed   time.Duration
	Slowest   time.Duration
	SlowestAt string

	Disagreement *Disagreement
}

// Agrees reports whether every instant compared agreed.
func (r Report) Agrees() bool { return r.Disagreement == nil }

// Check is the equality check as a command runs it over a real value (card 173):
// current(E, At) against fold.Snapshot(v, At, closes) at every every-th distinct
// fact-time the value holds, one instant per program, so the engine never holds a
// facts × instants table. It stops at the first disagreement.
func Check(ctx context.Context, v fold.Value, versions map[string]int, defs Definitions, every int) (Report, error) {
	if every < 1 {
		return Report{}, fmt.Errorf("%w: every must be at least 1, got %d", ErrInvalid, every)
	}
	rules, err := Bundled()
	if err != nil {
		return Report{}, err
	}
	base, err := NewBase(v, versions, defs)
	if err != nil {
		return Report{}, err
	}
	instants := factTimes(v)
	report := Report{FactTimes: len(instants)}
	start := time.Now()
	for i := 0; i < len(instants); i += every {
		at := instants[i]
		began := time.Now()
		got, err := currentAt(ctx, rules, base, defs, at)
		if err != nil {
			return report, err
		}
		want, err := snapshotIDs(v, at, defs)
		if err != nil {
			return report, err
		}
		report.Instants++
		if took := time.Since(began); took > report.Slowest {
			report.Slowest, report.SlowestAt = took, at
		}
		if !equalIDs(got, want) {
			report.Disagreement = &Disagreement{At: at, Current: got, Snapshot: want}
			break
		}
	}
	report.Elapsed = time.Since(start)
	return report, nil
}

// factTimes is the value's distinct fact-times in order.
func factTimes(v fold.Value) []string {
	var out []string
	for _, e := range v.Entries() {
		if n := len(out); n == 0 || out[n-1] != e.FactTime {
			out = append(out, e.FactTime)
		}
	}
	return out
}

// currentAt evaluates the bundled rules with at as the one instant and returns the
// ids current then, sorted.
func currentAt(ctx context.Context, rules RuleSet, base Base, defs Definitions, at string) ([]string, error) {
	prog, err := Parse(fmt.Sprintf("instant(%q).", at))
	if err != nil {
		return nil, err
	}
	plan, err := Compile(BaseSchema, defs, rules.Program, prog)
	if err != nil {
		return nil, err
	}
	res, err := Evaluate(ctx, plan, base)
	if err != nil {
		return nil, err
	}
	ans, err := res.Relation("current")
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, ans.Len())
	for _, t := range ans.Tuples {
		ids = append(ids, t[0].Text())
	}
	sort.Strings(ids)
	return ids, nil
}

func snapshotIDs(v fold.Value, at string, defs Definitions) ([]string, error) {
	s, err := fold.Snapshot(v, at, defs.Closes)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, s.Len())
	for _, e := range s.Entries() {
		ids = append(ids, e.FacetID)
	}
	sort.Strings(ids)
	return ids, nil
}

func equalIDs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
