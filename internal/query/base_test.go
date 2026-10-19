package query_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/query"
)

// attr/3 lists every scalar leaf of a served payload under the pointer def.Resolve
// reads it back by: the two agree on every leaf, so a query's pointer and a read's
// field name the same thing.
func TestNewBase_AttrLeavesResolve(t *testing.T) {
	t.Parallel()
	payload := `{"measured_at": "2026-01-01T07:00:00Z", "systolic": 118, "diastolic": 76.5, "tags": ["am", "sitting"],
	  "cuff": {"arm": "left", "a/b": true, "ti~lde": 1}, "note": null}`
	v, versions := value(t, entry("r1", "health.blood-pressure", day("2026-01-01"), "", payload))
	ans := run(t, v, versions, `leaf(P, V) :- attr("r1", P, V).`, "leaf")
	want := []string{
		"/cuff/arm left", "/cuff/a~1b true", "/cuff/ti~0lde 1", "/diastolic 76.5", "/measured_at 2026-01-01T07:00:00Z",
		"/systolic 118", "/tags/0 am", "/tags/1 sitting",
	}
	if got := rows(ans); !same(got, want) {
		t.Fatalf("leaves = %v, want %v", got, want)
	}
	doc, err := def.Decode([]byte(payload))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	for _, tup := range ans.Tuples {
		got, err := def.Resolve(doc, tup[0].Text())
		if err != nil {
			t.Errorf("Resolve(%s): %v", tup[0].Text(), err)
			continue
		}
		if fmt.Sprint(got) != tup[1].Text() {
			t.Errorf("Resolve(%s) = %v, attr says %s", tup[0].Text(), got, tup[1].Text())
		}
	}
}

// entry/7 carries the envelope, with the series as "" when there is none and the
// Definition version joined back from the fold; closes/1 is the registry's word.
func TestNewBase_EntryAndCloses(t *testing.T) {
	t.Parallel()
	v, versions := value(t,
		entry("m1", "med.started", day("2026-03-01"), "med.regimen:ramipril:1", `{"id": "m1", "date": "2026-03-01", "drug": "ramipril", "course": 1}`),
		entry("w1", "health.weight", day("2026-04-01"), "", `{"measured_at": "x", "kg": 80}`),
	)
	versions["w1"] = 3
	got := rows(run(t, v, versions, `e(E, T, F, S, St, Src, V) :- entry(E, T, F, S, St, Src, V).`, "e"))
	want := []string{
		"m1 med.started " + day("2026-03-01") + " med.regimen:ramipril:1 admitted self-log 1",
		"w1 health.weight " + day("2026-04-01") + "  admitted self-log 3",
	}
	if !same(got, want) {
		t.Errorf("entry = %v, want %v", got, want)
	}
	if got := rows(run(t, v, versions, `c(T) :- closes(T).`, "c")); !same(got, []string{"med.stopped"}) {
		t.Errorf("closes = %v, want [med.stopped]", got)
	}
	// A verbatim type declares no schema, so none of its fields can be named (the
	// fields' own rule, card 138); its served payload still has leaves a variable
	// pointer reaches.
	if _, err := tryRun(v, versions, registry(t), `kg(V) :- attr("w1", "/kg", V).`, "kg"); !errors.Is(err, query.ErrInvalid) {
		t.Errorf("a pointer only a verbatim payload holds = %v, want ErrInvalid", err)
	}
	if got := rows(run(t, v, versions, `kg(P, V) :- attr("w1", P, V), P != "/measured_at".`, "kg")); !same(got, []string{"/kg 80"}) {
		t.Errorf("attr over a verbatim type = %v, want [/kg 80]", got)
	}
}

// A served entry the fold does not hold is refused: the value and the versions must
// come from one watermark.
func TestNewBase_RefusesAnEntryNotInTheFold(t *testing.T) {
	t.Parallel()
	v, versions := value(t, entry("m1", "med.started", day("2026-03-01"), "", `{}`))
	delete(versions, "m1")
	_, err := query.NewBase(v, versions, registry(t))
	if !errors.Is(err, query.ErrBase) {
		t.Errorf("NewBase = %v, want ErrBase", err)
	}
}
