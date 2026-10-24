package selection_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/mattjoyce/anam/internal/selection"
)

const (
	oct1 = "2026-10-01T00:00:00.000000000Z"
	oct8 = "2026-10-08T00:00:00.000000000Z"
)

// filters is every shape the chips make, the corners included: no criteria, ticks
// alone, exclusions without a rule, a prefix beside an exact type, several of each.
var filters = map[string]selection.Filter{
	"nothing":             {},
	"ticks alone":         {Ticks: []string{"b", "a"}},
	"exclusions alone":    {Exclusions: []string{"x"}},
	"from":                {From: oct1},
	"to":                  {To: oct8},
	"window":              {From: oct1, To: oct8},
	"exact type":          {Types: []string{"health.blood-pressure"}},
	"prefix":              {Types: []string{"activity.exercise-session."}},
	"prefix and exact":    {Types: []string{"health.heart-rate.", "health.blood-pressure"}},
	"source":              {Sources: []string{"withings"}},
	"two by two":          {Types: []string{"health.weight", "health.heart-rate."}, Sources: []string{"withings", "garmin"}},
	"superseded":          {Standing: selection.Superseded},
	"superseded windowed": {Standing: selection.Superseded, From: oct1},
	"live named":          {Standing: selection.Live, Types: []string{"health.weight"}},
	"everything": {
		From: oct1, To: oct8, Types: []string{"activity.exercise-session.", "health.heart-rate.alert"},
		Sources: []string{"garmin"}, Ticks: []string{"t2", "t1"}, Exclusions: []string{"x1", "x2"},
	},
	"repeats":             {Types: []string{"health.weight", "health.weight"}, Ticks: []string{"a", "a"}},
	"ticked and excluded": {Types: []string{"health.weight"}, Ticks: []string{"a"}, Exclusions: []string{"a", "b"}},
	"quoting":             {Sources: []string{`odd "source" \ name`}, Ticks: []string{"id with spaces"}},
}

// Compile, then parse, is the identity on the filter as a set.
func TestRoundTrip(t *testing.T) {
	t.Parallel()
	for name, f := range filters {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			text, err := f.Program()
			if err != nil {
				t.Fatalf("Program: %v", err)
			}
			got, err := selection.Parse(text)
			if err != nil {
				t.Fatalf("Parse:\n%s\n%v", text, err)
			}
			if want := f.Canonical(); !reflect.DeepEqual(got, want) {
				t.Errorf("Parse(Program(f)) = %+v, want %+v\n%s", got, want, text)
			}
			again, err := got.Program()
			if err != nil || again != text {
				t.Errorf("the parsed filter writes a different program:\n%s\nwant\n%s", again, text)
			}
		})
	}
}

// The program is the value whatever order the criteria were added in.
func TestOrderDoesNotMatter(t *testing.T) {
	t.Parallel()
	a, err := selection.Filter{From: oct1, Types: []string{"health.weight", "activity.steps"}}.Program()
	if err != nil {
		t.Fatal(err)
	}
	b, err := selection.Filter{Types: []string{"activity.steps", "health.weight"}, From: oct1}.Program()
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Errorf("one filter, two programs:\n%s\n%s", a, b)
	}
}

// A sample is pinned, so a change to the program's shape is a change someone sees.
func TestProgram_Sample(t *testing.T) {
	t.Parallel()
	text, err := selection.Filter{
		From: oct1, To: oct8, Types: []string{"activity.exercise-session."},
		Ticks: []string{"5d1c0a4e-0000-0000-0000-000000000001", "5d1c0a4e-0000-0000-0000-000000000002"},
	}.Program()
	if err != nil {
		t.Fatal(err)
	}
	want := `% An Anam selection (card 199): pick/1 holds the facets it selects.
pick/1.
pick(E) :- entry(E, T, At, _, _, _, _),
    T > "activity.exercise-session.", T < "activity.exercise-session/",
    At >= "2026-10-01T00:00:00.000000000Z", At < "2026-10-08T00:00:00.000000000Z".
pick("5d1c0a4e-0000-0000-0000-000000000001").
pick("5d1c0a4e-0000-0000-0000-000000000002").
`
	if text != want {
		t.Errorf("program =\n%s\nwant\n%s", text, want)
	}
	if strings.Contains(text, "attr(") {
		t.Error("the program names a pointer")
	}
}

// Layout, comments and rule order are not the grammar; anything else is.
func TestParse_Grammar(t *testing.T) {
	t.Parallel()
	inside := `pick("b").
% a comment
pick(E) :- entry(E, T, _, _, _, _, _), T = "health.weight".
pick("a").`
	f, err := selection.Parse(inside)
	if err != nil {
		t.Fatalf("a reordered program in the grammar: %v", err)
	}
	if want := (selection.Filter{Types: []string{"health.weight"}, Ticks: []string{"a", "b"}}).Canonical(); !reflect.DeepEqual(f, want) {
		t.Errorf("Parse = %+v, want %+v", f, want)
	}

	outside := map[string]string{
		"not a program": "pick(E :- ",
		"a join":        `pick(E) :- entry(E, _, _, _, _, _, _), linked(L, E).`,
		"an attr":       `pick(E) :- entry(E, _, _, _, _, _, _), attr(E, "/systolic", V), V > 140.`,
		"another head":  `seen(E) :- entry(E, _, _, _, _, _, _).`,
		"another fact":  `seen("a").`,
		"a declaration": "instant/1.\npick(\"a\").",
		"a bad type":    `pick(E) :- entry(E, T, _, _, _, _, _), T = "weight".`,
		"a bare date":   `pick(E) :- entry(E, _, At, _, _, _, _), At >= "2026-10-01".`,
		"a lost bound":  `pick(E) :- entry(E, T, _, _, _, _, _), T > "activity.exercise-session.".`,
		"an exclusion the rule does not read": `pick(E) :- entry(E, T, _, _, _, _, _), T = "health.weight".
unpick("a").`,
		"two windows": `pick(E) :- entry(E, T, At, _, _, _, _), T = "health.weight", At >= "2026-10-01T00:00:00.000000000Z".
pick(E) :- entry(E, T, At, _, _, _, _), T = "activity.steps", At >= "2026-10-02T00:00:00.000000000Z".`,
		"a missing pair": `pick(E) :- entry(E, T, _, _, _, S, _), T = "health.weight", S = "garmin".
pick(E) :- entry(E, T, _, _, _, S, _), T = "health.weight", S = "withings".
pick(E) :- entry(E, T, _, _, _, S, _), T = "activity.steps", S = "garmin".`,
	}
	for name, text := range outside {
		if _, err := selection.Parse(text); !errors.Is(err, selection.ErrNotChips) {
			t.Errorf("%s: Parse = %v, want ErrNotChips", name, err)
		}
	}
}

func TestValidate_Refusals(t *testing.T) {
	t.Parallel()
	for name, f := range map[string]selection.Filter{
		"bare domain":   {Types: []string{"health."}},
		"date not time": {From: "2026-10-01"},
		"short lexeme":  {To: "2026-10-01T00:00:00Z"},
		"empty source":  {Sources: []string{""}},
		"empty tick":    {Ticks: []string{""}},
		"standing":      {Standing: "maybe"},
	} {
		if _, err := f.Program(); !errors.Is(err, selection.ErrInvalid) {
			t.Errorf("%s: Program = %v, want ErrInvalid", name, err)
		}
	}
}

func TestDays(t *testing.T) {
	t.Parallel()
	lo, hi, err := selection.Days("2026-10-01", "2026-10-07", time.UTC)
	if err != nil || lo != oct1 || hi != oct8 {
		t.Errorf("Days = %q %q %v, want %q %q", lo, hi, err, oct1, oct8)
	}
	if _, _, err := selection.Days("10/01/2026", "", time.UTC); !errors.Is(err, selection.ErrInvalid) {
		t.Errorf("a date not YYYY-MM-DD: %v", err)
	}
}

// A bare date is a day in the reader's zone (card 236): 12 Oct in Sydney starts at
// 13:00 UTC on the 11th, and its end is the next Sydney midnight.
func TestDays_ReadsTheReadersZone(t *testing.T) {
	t.Parallel()
	sydney, err := time.LoadLocation("Australia/Sydney")
	if err != nil {
		t.Fatal(err)
	}
	lo, hi, err := selection.Days("2026-10-12", "2026-10-12", sydney)
	// Sydney is on daylight time from 4 October: UTC+11.
	if want := "2026-10-11T13:00:00.000000000Z"; err != nil || lo != want {
		t.Errorf("from = %q %v, want %q", lo, err, want)
	}
	if want := "2026-10-12T13:00:00.000000000Z"; hi != want {
		t.Errorf("to = %q, want %q", hi, want)
	}
}

// The program names the zone its dates were read in, in a comment Parse ignores, and
// ZoneOf reads it back; a filter with no bound read no date and names none.
func TestProgramIn_NamesTheZone(t *testing.T) {
	t.Parallel()
	f := selection.Filter{From: "2026-10-11T13:00:00.000000000Z", Types: []string{"health.weight"}}
	text, err := f.ProgramIn("Australia/Sydney")
	if err != nil {
		t.Fatal(err)
	}
	if want := "% dates read in Australia/Sydney\n"; !strings.Contains(text, want) {
		t.Errorf("program lacks %q:\n%s", want, text)
	}
	if z := selection.ZoneOf(text); z != "Australia/Sydney" {
		t.Errorf("ZoneOf = %q", z)
	}
	back, err := selection.Parse(text)
	if err != nil || !reflect.DeepEqual(back, f.Canonical()) {
		t.Errorf("Parse = %+v %v, want %+v", back, err, f.Canonical())
	}
	if canon := selection.CanonicalText(text); canon != text {
		t.Errorf("saving changes the text, so the zone is lost:\n%s", canon)
	}

	plain, err := selection.Filter{Types: []string{"health.weight"}}.ProgramIn("Australia/Sydney")
	if err != nil || strings.Contains(plain, "dates read in") {
		t.Errorf("a filter with no bound names a zone: %v\n%s", err, plain)
	}
	for _, bad := range []string{"", "Local", "Mars/Olympus"} {
		text, err := f.ProgramIn(bad)
		if err != nil || strings.Contains(text, "dates read in") {
			t.Errorf("zone %q was written: %v\n%s", bad, err, text)
		}
	}
	if z := selection.ZoneOf("% dates read in Mars/Olympus\npick/1.\n"); z != "" {
		t.Errorf("ZoneOf an unknown zone = %q", z)
	}
	if z := selection.ZoneOf(strings.Replace(text, "Australia/Sydney", "Local", 1)); z != "" {
		t.Errorf("ZoneOf Local = %q", z)
	}
}
