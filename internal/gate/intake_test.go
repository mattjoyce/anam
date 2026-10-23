package gate_test

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/config"
	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/fold"
	"github.com/mattjoyce/anam/internal/gate"
	"github.com/mattjoyce/anam/internal/store/ledger"
)

// Card 232: health.intake over the bundled Definition, a real gate and a real ledger.
// A person says what they took in (a coffee, two glasses of wine, a rich meal out); the
// gate holds the drink to one spelling, the count to a whole number, and each kind to
// its own fields through the schema's if and then.

func newIntakeGate(t *testing.T) (*gate.Gate, *ledger.Ledger) {
	t.Helper()
	l, err := ledger.Create(context.Background(), filepath.Join(t.TempDir(), "ledger.db"), person)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	reg, err := def.Bundled()
	if err != nil {
		t.Fatal(err)
	}
	scopes := gate.StaticScopes{"conversation": {"health.intake"}}
	g := gate.New(l, reg, scopes, clock.Real{}, person, gate.Limits{MaxPayloadBytes: config.DefaultMaxPayloadBytes, MaxRetries: config.DefaultCommitRetries})
	return g, l
}

func sayIntake(t *testing.T, g *gate.Gate, payload string) (gate.Result, error) {
	t.Helper()
	env := facet.Envelope{Type: "health.intake", Source: "conversation", Injector: "conversation", Payload: []byte(payload)}
	b, err := g.Resolve(env)
	if err != nil {
		return gate.Result{}, err
	}
	res, _, err := g.Commit(context.Background(), env, b)
	return res, err
}

// intakePayload is a payload for the wall clock at, said in Sydney at +11:00, with the
// fields in body (a JSON fragment without braces) between kind and source_time.
func intakePayload(at, body string) string {
	return fmt.Sprintf(`{"at": %q, %s, %s}`, at, body, attentionClock(at))
}

func TestIntake_EachKindIsAdmittedAndAWrongShapeIsRefused(t *testing.T) {
	t.Parallel()
	const at = "2026-10-22 08:15"
	for _, tc := range []struct {
		name, body string
		ok         bool
	}{
		{"a coffee", `"kind": "caffeine", "drink": "flat-white", "count": 1`, true},
		{"a coffee with a size and a volume", `"kind": "caffeine", "drink": "espresso", "count": 1, "size": "small", "volume_ml": 60`, true},
		{"two glasses of wine", `"kind": "alcohol", "drink": "red-wine", "count": 2, "size": "regular", "volume_ml": 150, "abv": 13.5`, true},
		{"a pint at a place", `"kind": "alcohol", "drink": "beer", "count": 1, "size": "pint", "place": "corner-pub"`, true},
		{"a drink with a note", `"kind": "alcohol", "drink": "beer", "count": 1, "note": "a long day"`, true},
		{"a rich meal out", `"kind": "meal", "text": "ate out, rich meal", "rich": true, "out": true`, true},
		{"a plain meal at home, both answers false", `"kind": "meal", "text": "late pasta", "rich": false, "out": false`, true},
		{"a meal at a place", `"kind": "meal", "text": "dinner", "rich": true, "out": true, "place": "corner-cafe"`, true},

		{"a caffeine without a count", `"kind": "caffeine", "drink": "flat-white"`, false},
		{"a caffeine without a drink", `"kind": "caffeine", "count": 1`, false},
		{"an alcohol without a count", `"kind": "alcohol", "drink": "beer"`, false},
		{"a meal without rich", `"kind": "meal", "text": "dinner", "out": true`, false},
		{"a meal without out", `"kind": "meal", "text": "dinner", "rich": true`, false},
		{"a meal without text", `"kind": "meal", "rich": true, "out": true`, false},
		{"an unknown kind", `"kind": "snack", "drink": "tea", "count": 1`, false},
		{"no kind", `"drink": "tea", "count": 1`, false},
		{"a drink with spaces", `"kind": "caffeine", "drink": "flat white", "count": 1`, false},
		{"a drink in capitals", `"kind": "caffeine", "drink": "Espresso", "count": 1`, false},
		{"a size with spaces", `"kind": "alcohol", "drink": "beer", "count": 1, "size": "big one"`, false},
		{"a place with spaces", `"kind": "alcohol", "drink": "beer", "count": 1, "place": "corner pub"`, false},
		{"a count of zero", `"kind": "caffeine", "drink": "tea", "count": 0`, false},
		{"a fractional count", `"kind": "caffeine", "drink": "tea", "count": 1.5`, false},
		{"a count as text", `"kind": "caffeine", "drink": "tea", "count": "2"`, false},
		{"abv at the top", `"kind": "alcohol", "drink": "spirits", "count": 1, "abv": 100`, true},
		{"abv over the top", `"kind": "alcohol", "drink": "spirits", "count": 1, "abv": 101`, false},
		{"abv below zero", `"kind": "alcohol", "drink": "beer", "count": 1, "abv": -1`, false},
		{"a volume of zero", `"kind": "alcohol", "drink": "beer", "count": 1, "volume_ml": 0`, false},
		{"abv on a coffee", `"kind": "caffeine", "drink": "tea", "count": 1, "abv": 0.5`, false},
		{"a meal's text on a drink", `"kind": "alcohol", "drink": "beer", "count": 1, "text": "with dinner"`, false},
		{"a meal's rich on a coffee", `"kind": "caffeine", "drink": "tea", "count": 1, "rich": false`, false},
		{"a drink on a meal", `"kind": "meal", "text": "dinner", "rich": true, "out": true, "drink": "red-wine"`, false},
		{"a count on a meal", `"kind": "meal", "text": "dinner", "rich": true, "out": true, "count": 1`, false},
		{"an abv on a meal", `"kind": "meal", "text": "dinner", "rich": true, "out": true, "abv": 12`, false},
		{"an empty meal text", `"kind": "meal", "text": "", "rich": true, "out": true`, false},
		{"rich as text", `"kind": "meal", "text": "dinner", "rich": "yes", "out": true`, false},
		{"an empty note", `"kind": "caffeine", "drink": "tea", "count": 1, "note": ""`, false},
		{"a field of its own", `"kind": "caffeine", "drink": "tea", "count": 1, "calories": 5`, false},
		{"a part of the day nobody named", `"kind": "caffeine", "drink": "tea", "count": 1, "part_of_day": "dawn"`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g, l := newIntakeGate(t)
			res, err := sayIntake(t, g, intakePayload(at, tc.body))
			switch {
			case tc.ok && (err != nil || res.Outcome != gate.OutcomeAdmitted):
				t.Fatalf("got %+v, %v; want admitted", res, err)
			case !tc.ok && err == nil:
				t.Fatalf("admitted %+v; want a refusal", res)
			}
			if !tc.ok {
				t.Logf("refused: %v", err)
				got, err := fold.Read(context.Background(), l)
				if err != nil || len(got.Live) != 0 {
					t.Errorf("a refused payload left %d live facets (%v)", len(got.Live), err)
				}
			}
		})
	}
}

func TestIntake_NoSourceTimeIsRefused(t *testing.T) {
	t.Parallel()
	g, _ := newIntakeGate(t)
	if res, err := sayIntake(t, g, `{"at": "2026-10-22 08:15", "kind": "caffeine", "drink": "tea", "count": 1}`); err == nil {
		t.Fatalf("admitted %+v without a source_time", res)
	}
}

// A part of the day stands in for a time the person did not give: the fact sits at the
// part's nominal minute, and the payload says it is nominal.
func TestIntake_APartOfTheDayIsAdmittedAtItsNominalMinute(t *testing.T) {
	t.Parallel()
	g, l := newIntakeGate(t)
	res, err := sayIntake(t, g, intakePayload("2026-10-22 19:00", `"kind": "alcohol", "drink": "red-wine", "count": 2, "part_of_day": "evening"`))
	if err != nil || res.Outcome != gate.OutcomeAdmitted {
		t.Fatalf("got %+v, %v; want admitted", res, err)
	}
	got, err := fold.Read(context.Background(), l)
	if err != nil {
		t.Fatal(err)
	}
	var factTime string
	for _, e := range got.Value.Entries() {
		if e.FacetID == res.FacetID {
			factTime = e.FactTime
		}
	}
	if factTime != "2026-10-22T08:00:00.000000000Z" {
		t.Errorf("fact time = %q; want 19:00 at +11:00 placed at 08:00 UTC", factTime)
	}
}

// The id is composed from the kind, the minute and the offset, so two of one kind in
// one minute are one fact, a different kind in that minute is its own, and one minute
// said in two zones is two facts.
func TestIntake_TheIdIsComposedAndOneKindInOneMinuteIsOneFact(t *testing.T) {
	t.Parallel()
	g, _ := newIntakeGate(t)
	const at = "2026-10-22 08:15"
	coffee := intakePayload(at, `"kind": "caffeine", "drink": "flat-white", "count": 1`)

	a, err := sayIntake(t, g, coffee)
	if err != nil || a.Outcome != gate.OutcomeAdmitted {
		t.Fatalf("coffee: %+v, %v", a, err)
	}
	want := facet.NewID(facet.SourceRef("conversation", "conversation:intake/caffeine/2026-10-22T08:15+11:00"))
	if a.FacetID != want {
		t.Errorf("id = %s; want %s", a.FacetID, want)
	}
	again, err := sayIntake(t, g, coffee)
	if err != nil || again.Outcome != gate.OutcomeNoOp || again.FacetID != a.FacetID {
		t.Errorf("saying it again = %+v, %v; want a no-op on %s", again, err, a.FacetID)
	}
	wine, err := sayIntake(t, g, intakePayload(at, `"kind": "alcohol", "drink": "red-wine", "count": 1`))
	if err != nil || wine.Outcome != gate.OutcomeAdmitted || wine.FacetID == a.FacetID {
		t.Errorf("another kind, same minute = %+v, %v; want its own fact", wine, err)
	}
	london := fmt.Sprintf(`{"at": %q, "kind": "caffeine", "drink": "flat-white", "count": 1, "source_time": {"value": %q, "zone": "Europe/London", "offset": "+01:00", "basis": "source"}}`, at, at)
	b, err := sayIntake(t, g, london)
	if err != nil || b.Outcome != gate.OutcomeAdmitted || b.FacetID == a.FacetID {
		t.Errorf("same minute in London = %+v, %v; want its own fact", b, err)
	}

	// An id sent must be the composition; sent wrong it is refused, sent right it is the same fact.
	right := intakePayload(at, `"id": "conversation:intake/caffeine/2026-10-22T08:15+11:00", "kind": "caffeine", "drink": "flat-white", "count": 1`)
	if res, err := sayIntake(t, g, right); err != nil || res.FacetID != a.FacetID {
		t.Errorf("right id = %+v, %v; want the coffee's fact", res, err)
	}
	wrong := intakePayload(at, `"id": "conversation:intake/caffeine/wrong", "kind": "caffeine", "drink": "flat-white", "count": 1`)
	if res, err := sayIntake(t, g, wrong); err == nil || !strings.Contains(err.Error(), "id") {
		t.Errorf("wrong id = %+v, %v; want a refusal naming the id", res, err)
	}
}
