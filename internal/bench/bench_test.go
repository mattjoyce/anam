package bench_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mattjoyce/anam/internal/bench"
	"github.com/mattjoyce/anam/internal/catalog"
	"github.com/mattjoyce/anam/internal/catalog/catalogtest"
	"github.com/mattjoyce/anam/internal/catalog/fixture"
	"github.com/mattjoyce/anam/internal/facet"
)

// A tiny shape built and run end to end: the bench's own correctness, so a timing it
// reports is a timing of the reads it names.
func TestGenerateThenRun(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	sh := bench.Shape{
		Name: "tiny", Months: 3, PerMonth: 40, CorrectEvery: 10, Retract: 2, TailNew: 5, TailCorrect: 3,
		Start: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	cat := catalogtest.Engine.Catalog(t)
	if err := bench.Generate(context.Background(), cat, "test.pair", dir, sh, nil); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	rep, err := bench.Run(context.Background(), bench.Options{
		Name: "tiny", Subject: bench.Subject, Paths: bench.PathsIn(dir), Reps: 3, Timeout: time.Minute, Catalog: cat,
	}, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// 120 readings, 12 corrected (no new facets), 2 retracted, 5 new in the tail.
	if d := rep.Dataset; d.Facets != 125 || d.Live != 123 || d.Checkpoints == 0 {
		t.Errorf("dataset = %+v", d)
	}
	rows := map[string]int{}
	steps := map[string]int{}
	for _, s := range rep.Scenarios {
		rows[s.Name] = s.Rows
		steps[s.Name] = s.Steps
		if s.MinMS > s.MedianMS || s.MedianMS > s.MaxMS {
			t.Errorf("%s: min %f > med %f > max %f", s.Name, s.MinMS, s.MedianMS, s.MaxMS)
		}
	}
	if rows["timeline, whole"] != 123 || rows["timeline, one month"] == 0 || rows["changes, last day"] == 0 {
		t.Errorf("scenario rows = %v", rows)
	}
	for _, name := range []string{"query, every entry", "query, every attr", "query, current at every fact-time", "query, one type joined to its attrs"} {
		if steps[name] <= 0 {
			t.Errorf("%s steps = %d", name, steps[name])
		}
	}
	if rows["query, every entry"] != rep.Dataset.Live {
		t.Errorf("query, every entry rows = %d live = %d", rows["query, every entry"], rep.Dataset.Live)
	}
	if steps["replay all events"] != 0 {
		t.Errorf("non-query steps = %d", steps["replay all events"])
	}
	var out bytes.Buffer
	if err := bench.WriteComparison(&out, rep, rep); err != nil || !strings.Contains(out.String(), "full fold") {
		t.Errorf("comparison: %v\n%s", err, out.String())
	}
}

// The generator writes a time and two numbers, so it takes a type only when that
// payload satisfies the type's schema and keys it by the time alone; anything else is
// refused before a ledger is made, naming why.
func TestPairOf_RefusesWhatItCannotWrite(t *testing.T) {
	t.Parallel()
	engine := catalogtest.Engine.Catalog(t)
	if _, err := bench.PairOf(engine, "test.pair"); err != nil {
		t.Fatalf("PairOf(test.pair) = %v", err)
	}
	for _, tc := range []struct {
		typ  facet.Type
		want string
	}{
		{"test.absent", "holds no Definition"},
		{"test.measure", "identity key [/at /seq]"},
		{"test.reading", "identity key [/id]"},
		{"test.course.started", "identity key [/id]"},
	} {
		_, err := bench.PairOf(engine, tc.typ)
		if !errors.Is(err, bench.ErrType) || err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("PairOf(%s) = %v, want ErrType saying %q", tc.typ, err, tc.want)
		}
	}

	// test.pair with its schema changed, in a copy of Engine. A constraint on a field the
	// generator writes is refused by name: Sol's case was a maximum of 139 on a, which a
	// correction broke halfway through a build.
	for _, tc := range []struct {
		name     string
		required []string
		field    string
		keyword  string
		value    any
		want     string
	}{
		{"three numbers", []string{"at", "a", "b", "c"}, "", "", nil, "requires 3 numeric fields"},
		{"one number", []string{"at", "a"}, "", "", nil, "requires 1 numeric fields"},
		{"a string", []string{"at", "a", "b", "note"}, "", "", nil, "requires /note, which is not a number"},
		{"a maximum", nil, "a", "maximum", 139, "its /a declares maximum"},
		{"a minimum", nil, "b", "minimum", 0, "its /b declares minimum"},
		{"an exclusive maximum", nil, "a", "exclusiveMaximum", 1000, "its /a declares exclusiveMaximum"},
		{"an exclusive minimum", nil, "b", "exclusiveMinimum", 0, "its /b declares exclusiveMinimum"},
		{"a step", nil, "a", "multipleOf", 2, "its /a declares multipleOf"},
		{"an enum", nil, "b", "enum", []int{60, 61}, "its /b declares enum"},
		{"a const", nil, "a", "const", 120, "its /a declares const"},
		{"a time pattern", nil, "at", "pattern", "^2", "its fact time /at declares pattern"},
		{"a time length", nil, "at", "minLength", 20, "its fact time /at declares minLength"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			required := tc.required
			if required == nil {
				required = []string{"at", "a", "b"}
			}
			cat := pairVariant(t, required, tc.field, tc.keyword, tc.value)
			_, err := bench.PairOf(cat, "test.pair")
			if !errors.Is(err, bench.ErrType) || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("PairOf = %v, want ErrType saying %q", err, tc.want)
			}
			if err := bench.Generate(context.Background(), cat, "test.pair", t.TempDir(), bench.Shapes()["S"], nil); !errors.Is(err, bench.ErrType) {
				t.Errorf("Generate = %v, want ErrType", err)
			}
		})
	}
}

// pairVariant is a copy of Engine whose test.pair schema requires required, with a
// numeric c declared beside a and b and, when field is not "", keyword set to value
// on that field.
func pairVariant(t *testing.T, required []string, field, keyword string, value any) *catalog.Catalog {
	t.Helper()
	defs, projs := fixture.Engine()
	raw, err := fs.ReadFile(defs, "test.pair.json")
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	schema := m["schema"].(map[string]any)
	schema["required"] = required
	props := schema["properties"].(map[string]any)
	props["c"] = map[string]any{"type": "number"}
	if field != "" {
		props[field].(map[string]any)[keyword] = value
	}
	body, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	defsDir, projsDir := fixture.Dirs(t, defs, projs)
	if err := os.WriteFile(filepath.Join(defsDir, "test.pair.json"), body, 0o600); err != nil {
		t.Fatal(err)
	}
	cat, err := catalog.Load(context.Background(), defsDir, projsDir)
	if err != nil {
		t.Fatal(err)
	}
	return cat
}
