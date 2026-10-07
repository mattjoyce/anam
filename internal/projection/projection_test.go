package projection_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/consent"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/fold"
	"github.com/mattjoyce/anam/internal/projection"
)

// A canonical to prune. Five entries spanning three types, two statuses and all three
// classifications, so every knob has something to include and something to exclude.
func canonical(t *testing.T) fold.Value {
	t.Helper()

	entries := []fold.Entry{
		entry("f-bp-1", "health.blood-pressure", "2026-01-24T06:36:00.000000000Z", facet.StatusAdmitted, facet.ClassificationNormal),
		entry("f-bp-2", "health.blood-pressure", "2026-02-01T06:30:00.000000000Z", facet.StatusEndorsed, facet.ClassificationConfidential),
		entry("f-sleep-1", "health.sleep", "2026-01-25T22:00:00.000000000Z", facet.StatusAdmitted, facet.ClassificationNormal),
		entry("f-hr-1", "health.heart-rate", "2026-01-26T08:00:00.000000000Z", facet.StatusAdmitted, facet.ClassificationSensitive),
		entry("f-bp-3", "health.blood-pressure", "2026-03-15T06:20:00.000000000Z", facet.StatusTombstoned, facet.ClassificationNormal),
	}
	v, err := fold.NewValue(entries)
	if err != nil {
		t.Fatalf("fold.NewValue: %v", err)
	}
	return v
}

func entry(id string, typ facet.Type, factTime string, st facet.Status, c facet.Classification) fold.Entry {
	return fold.Entry{
		FacetID:        id,
		Type:           typ,
		FactTime:       factTime,
		Source:         "bp-csv",
		SourceRef:      facet.SourceRef("bp-csv", id),
		Status:         st,
		Classification: c,
		ContentHash:    "sha256:" + strings.Repeat("0", 64),
		Payload:        json.RawMessage(fmt.Sprintf(`{"id":%q,"note":"verbatim, untouched"}`, id)),
	}
}

func mustParse(t *testing.T, raw string) projection.Template {
	t.Helper()
	tmpl, err := projection.Parse([]byte(raw))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return tmpl
}

func ids(v fold.Value) []string {
	out := make([]string, 0, v.Len())
	for _, e := range v.Entries() {
		out = append(out, e.FacetID)
	}
	return out
}

// template builds a manifest with the given knobs; every field is spelled out because
// the parser requires it.
func template(name string, types, statuses []string, maxClass, from, to string) string {
	q := func(ss []string) string {
		parts := make([]string, len(ss))
		for i, s := range ss {
			parts[i] = fmt.Sprintf("%q", s)
		}
		return "[" + strings.Join(parts, ",") + "]"
	}
	return fmt.Sprintf(`{"name":%q,"version":1,"types":%s,"statuses":%s,"max_classification":%q,"from":%q,"to":%q}`,
		name, q(types), q(statuses), maxClass, from, to)
}

// --- the template as a value -------------------------------------------------------

func TestParse_RequiresEveryKnob(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"no name":               `{"version":1,"types":[],"statuses":[],"max_classification":"Normal"}`,
		"empty name":            `{"name":"","version":1,"types":[],"statuses":[],"max_classification":"Normal"}`,
		"no version":            `{"name":"x","types":[],"statuses":[],"max_classification":"Normal"}`,
		"zero version":          `{"name":"x","version":0,"types":[],"statuses":[],"max_classification":"Normal"}`,
		"no types":              `{"name":"x","version":1,"statuses":[],"max_classification":"Normal"}`,
		"no statuses":           `{"name":"x","version":1,"types":[],"max_classification":"Normal"}`,
		"no max_classification": `{"name":"x","version":1,"types":[],"statuses":[]}`,
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := projection.Parse([]byte(raw)); !errors.Is(err, projection.ErrBadTemplate) {
				t.Errorf("Parse(%s): err = %v, want ErrBadTemplate", name, err)
			}
		})
	}
}

// A manifest is config, and config is external input: unparseable enums fail at load,
// not at the first serve, when the operator is nowhere near.
func TestParse_RejectsBadValuesAtLoad(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"unknown key":         `{"name":"x","version":1,"types":[],"statuses":[],"max_classification":"Normal","tombstones":true}`,
		"unnamespaced type":   template("x", []string{"bloodpressure"}, []string{"admitted"}, "Normal", "", ""),
		"unknown status":      template("x", []string{"health.sleep"}, []string{"published"}, "Normal", "", ""),
		"unknown class":       template("x", []string{"health.sleep"}, []string{"admitted"}, "Secret", "", ""),
		"unparseable from":    template("x", []string{"health.sleep"}, []string{"admitted"}, "Normal", "2026-01-01", ""),
		"from at or after to": template("x", []string{"health.sleep"}, []string{"admitted"}, "Normal", "2026-02-01T00:00:00.000000000Z", "2026-01-01T00:00:00.000000000Z"),
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := projection.Parse([]byte(raw)); !errors.Is(err, projection.ErrBadTemplate) {
				t.Errorf("Parse(%s): err = %v, want ErrBadTemplate", name, err)
			}
		})
	}
}

func TestBundled_ParsesBpAndSleep(t *testing.T) {
	t.Parallel()

	r, err := projection.Bundled()
	if err != nil {
		t.Fatalf("Bundled: %v", err)
	}
	if got, want := r.Names(), []string{"bp", "meds", "sleep"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("bundled names = %v, want %v", got, want)
	}
	for _, tmpl := range r.All() {
		if tmpl.Version < 1 {
			t.Errorf("%s: version %d", tmpl.Name, tmpl.Version)
		}
		if len(tmpl.Types) == 0 {
			t.Errorf("%s: serves no types — a bundled template that serves nothing is a mistake, not a policy", tmpl.Name)
		}
		if tmpl.Span() != "whole-person" {
			t.Errorf("%s: span %q, want whole-person", tmpl.Name, tmpl.Span())
		}
	}
}

// An unknown name lists what is available: the set is closed and shipped with the
// binary, so there is no reason to make the operator go looking.
func TestLookup_UnknownNamesTheAlternatives(t *testing.T) {
	t.Parallel()

	r, err := projection.Bundled()
	if err != nil {
		t.Fatalf("Bundled: %v", err)
	}
	_, err = r.Lookup("bloodpressure")
	if !errors.Is(err, projection.ErrUnknown) {
		t.Fatalf("Lookup: err = %v, want ErrUnknown", err)
	}
	for _, want := range []string{"bloodpressure", "bp", "sleep"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

func TestSpan_RendersTheWindow(t *testing.T) {
	t.Parallel()

	cases := []struct{ from, to, want string }{
		{"", "", "whole-person"},
		{"2026-01-01T00:00:00.000000000Z", "", "fact-time[2026-01-01T00:00:00.000000000Z,+inf)"},
		{"", "2026-02-01T00:00:00.000000000Z", "fact-time[-inf,2026-02-01T00:00:00.000000000Z)"},
		{"2026-01-01T00:00:00.000000000Z", "2026-02-01T00:00:00.000000000Z", "fact-time[2026-01-01T00:00:00.000000000Z,2026-02-01T00:00:00.000000000Z)"},
	}
	for _, c := range cases {
		tmpl := mustParse(t, template("x", []string{"health.sleep"}, []string{"admitted"}, "Normal", c.from, c.to))
		if got := tmpl.Span(); got != c.want {
			t.Errorf("Span(%q,%q) = %q, want %q", c.from, c.to, got, c.want)
		}
	}
}

// --- the four knobs ----------------------------------------------------------------

// An allow-list that admits everything when empty is not an allow-list.
func TestApply_TypeAllowListDeniesByDefault(t *testing.T) {
	t.Parallel()

	v := canonical(t)
	if v.Len() == 0 {
		t.Fatal("the canonical under test is empty; the criterion would pass vacuously")
	}

	tmpl := mustParse(t, template("empty", nil, []string{"admitted", "endorsed"}, "Sensitive", "", ""))
	served, err := projection.Apply(v, tmpl, consent.Open{}, nil)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if served.Len() != 0 {
		t.Errorf("an empty type allow-list served %d of %d entries: %v", served.Len(), v.Len(), ids(served))
	}
}

func TestApply_StatusAllowListDeniesByDefault(t *testing.T) {
	t.Parallel()

	v := canonical(t)
	tmpl := mustParse(t, template("nostatus", []string{"health.blood-pressure"}, nil, "Sensitive", "", ""))
	served, err := projection.Apply(v, tmpl, consent.Open{}, nil)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if served.Len() != 0 {
		t.Errorf("an empty status list served %v", ids(served))
	}
}

func TestApply_Knobs(t *testing.T) {
	t.Parallel()

	all := []string{"health.blood-pressure", "health.sleep", "health.heart-rate"}
	everyStatus := []string{"admitted", "endorsed", "tombstoned"}

	cases := []struct {
		name string
		tmpl string
		want []string
	}{
		{
			"type allow-list narrows to one type",
			template("t", []string{"health.blood-pressure"}, everyStatus, "Sensitive", "", ""),
			[]string{"f-bp-1", "f-bp-2", "f-bp-3"},
		},
		{
			"status filter admits only what it lists",
			template("t", all, []string{"endorsed"}, "Sensitive", "", ""),
			[]string{"f-bp-2"},
		},
		{
			"max classification Normal excludes Confidential and Sensitive",
			template("t", all, everyStatus, "Normal", "", ""),
			[]string{"f-bp-1", "f-sleep-1", "f-bp-3"},
		},
		{
			"max classification Confidential admits at the ceiling, not above",
			template("t", all, everyStatus, "Confidential", "", ""),
			[]string{"f-bp-1", "f-sleep-1", "f-bp-2", "f-bp-3"},
		},
		{
			"from is inclusive",
			template("t", all, everyStatus, "Sensitive", "2026-01-26T08:00:00.000000000Z", ""),
			[]string{"f-hr-1", "f-bp-2", "f-bp-3"},
		},
		{
			"to is exclusive",
			template("t", all, everyStatus, "Sensitive", "", "2026-01-26T08:00:00.000000000Z"),
			[]string{"f-bp-1", "f-sleep-1"},
		},
		{
			"a window narrows both ends",
			template("t", all, everyStatus, "Sensitive", "2026-01-25T00:00:00.000000000Z", "2026-02-01T06:30:00.000000000Z"),
			[]string{"f-sleep-1", "f-hr-1"},
		},
		{
			"knobs compose: bp, admitted only, Normal only",
			template("t", []string{"health.blood-pressure"}, []string{"admitted"}, "Normal", "", ""),
			[]string{"f-bp-1"},
		},
	}

	v := canonical(t)
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			served, err := projection.Apply(v, mustParse(t, c.tmpl), consent.Open{}, nil)
			if err != nil {
				t.Fatalf("Apply: %v", err)
			}
			if got := ids(served); !reflect.DeepEqual(got, c.want) {
				t.Errorf("served %v, want %v", got, c.want)
			}
		})
	}
}

// --- prune-only: the structural claims ---------------------------------------------

// ∀ template over this canonical: every served entry is deep-equal to a canonical
// entry, and the served set is a subset. Nothing is reshaped and nothing is generated.
//
// The templates are enumerated rather than randomly generated: the knob space is four
// small closed sets, so the cross-product IS the universal quantifier, exhaustively
// rather than by sampling.
func TestApply_PropertyPruneOnly(t *testing.T) {
	t.Parallel()

	v := canonical(t)
	byID := map[string]fold.Entry{}
	for _, e := range v.Entries() {
		byID[e.FacetID] = e
	}

	types := [][]string{
		nil,
		{"health.blood-pressure"},
		{"health.sleep"},
		{"health.blood-pressure", "health.sleep", "health.heart-rate"},
	}
	statuses := [][]string{nil, {"admitted"}, {"endorsed"}, {"admitted", "endorsed", "tombstoned"}}
	classes := []string{"Normal", "Confidential", "Sensitive"}
	windows := [][2]string{
		{"", ""},
		{"2026-01-25T00:00:00.000000000Z", ""},
		{"", "2026-02-01T00:00:00.000000000Z"},
		{"2026-01-25T00:00:00.000000000Z", "2026-03-01T00:00:00.000000000Z"},
	}

	checked := 0
	for _, ty := range types {
		for _, st := range statuses {
			for _, cl := range classes {
				for _, win := range windows {
					tmpl := mustParse(t, template("p", ty, st, cl, win[0], win[1]))
					served, err := projection.Apply(v, tmpl, consent.Open{}, nil)
					if err != nil {
						t.Fatalf("Apply(%v/%v/%s/%v): %v", ty, st, cl, win, err)
					}
					for _, e := range served.Entries() {
						src, ok := byID[e.FacetID]
						if !ok {
							t.Fatalf("Apply(%v/%v/%s/%v) served %s, which is not in the canonical", ty, st, cl, win, e.FacetID)
						}
						if !reflect.DeepEqual(e, src) {
							t.Fatalf("Apply(%v/%v/%s/%v) reshaped %s:\n got %+v\nwant %+v", ty, st, cl, win, e.FacetID, e, src)
						}
						if string(e.Payload) != string(src.Payload) {
							t.Fatalf("Apply(%v/%v/%s/%v) moved %s's payload bytes:\n got %s\nwant %s",
								ty, st, cl, win, e.FacetID, e.Payload, src.Payload)
						}
					}
					if served.Len() > v.Len() {
						t.Fatalf("Apply(%v/%v/%s/%v) served %d entries from a canonical of %d", ty, st, cl, win, served.Len(), v.Len())
					}
					checked++
				}
			}
		}
	}
	if want := len(types) * len(statuses) * len(classes) * len(windows); checked != want {
		t.Errorf("checked %d templates, want the full cross-product of %d", checked, want)
	}
}

// A wide-open template serves the canonical unchanged, hash and all. This is what pins
// the served serialisation to the canonical's own: any second serialisation, however
// carefully written, would move the hash here.
func TestApply_WideOpenReproducesTheCanonicalHash(t *testing.T) {
	t.Parallel()

	v := canonical(t)
	tmpl := mustParse(t, template("everything",
		[]string{"health.blood-pressure", "health.sleep", "health.heart-rate"},
		[]string{"admitted", "endorsed", "tombstoned", "rejected", "superseded"},
		"Sensitive", "", ""))

	served, err := projection.Apply(v, tmpl, consent.Open{}, nil)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if served.Hash() != v.Hash() {
		t.Errorf("wide-open served hash %s, canonical %s", served.Hash(), v.Hash())
	}
	if string(served.Bytes()) != string(v.Bytes()) {
		t.Error("wide-open served bytes differ from the canonical's")
	}
}

// --- the consent seam ---------------------------------------------------------------

// denyType is a gate that refuses one facet type, standing in for the policy §10
// defers. Its only job here is to prove the seam is load-bearing rather than decorative.
type denyType struct{ typ facet.Type }

func (d denyType) Evaluate(r consent.Request) consent.Decision {
	if r.Type == d.typ {
		return consent.Decision{Allow: false, Reason: "denied by test policy"}
	}
	return consent.Decision{Allow: true}
}

func TestApply_ConsentGateCanExclude(t *testing.T) {
	t.Parallel()

	v := canonical(t)
	tmpl := mustParse(t, template("t",
		[]string{"health.blood-pressure", "health.sleep"},
		[]string{"admitted", "endorsed", "tombstoned"}, "Sensitive", "", ""))

	open, err := projection.Apply(v, tmpl, consent.Open{}, nil)
	if err != nil {
		t.Fatalf("Apply(open): %v", err)
	}
	if open.Len() == 0 {
		t.Fatal("the open gate served nothing; the denial below would pass vacuously")
	}

	denied, err := projection.Apply(v, tmpl, denyType{typ: "health.blood-pressure"}, nil)
	if err != nil {
		t.Fatalf("Apply(deny): %v", err)
	}
	if got, want := ids(denied), []string{"f-sleep-1"}; !reflect.DeepEqual(got, want) {
		t.Errorf("with blood-pressure denied, served %v, want %v", got, want)
	}
}

// The seam is in the signature: a serve path cannot be written that forgets it.
func TestApply_RequiresAGate(t *testing.T) {
	t.Parallel()

	tmpl := mustParse(t, template("t", []string{"health.sleep"}, []string{"admitted"}, "Normal", "", ""))
	if _, err := projection.Apply(canonical(t), tmpl, nil, nil); err == nil {
		t.Error("Apply with no consent gate was accepted")
	}
}
