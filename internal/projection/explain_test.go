package projection_test

import (
	"reflect"
	"testing"

	"github.com/mattjoyce/anam/internal/consent"
	"github.com/mattjoyce/anam/internal/projection"
)

// Explain is what Apply is built from, not a companion to it. This is the property
// that keeps a projection's explanation honest: if the two could disagree, the page
// showing "excluded: type not in the allow-list" would be a story about a filter
// rather than a report of one.
//
// Exhaustive over the same closed knob space the prune-only property uses.
func TestExplain_AgreesWithApplyOverEveryTemplate(t *testing.T) {
	t.Parallel()

	v := canonical(t)
	types := [][]string{nil, {"health.blood-pressure"}, {"health.sleep"}, {"health.blood-pressure", "health.sleep", "health.heart-rate"}}
	statuses := [][]string{nil, {"admitted"}, {"endorsed"}, {"admitted", "endorsed", "tombstoned"}}
	classes := []string{"Normal", "Confidential", "Sensitive"}
	windows := [][2]string{
		{"", ""},
		{"2026-01-25T00:00:00.000000000Z", ""},
		{"", "2026-02-01T00:00:00.000000000Z"},
	}

	for _, ty := range types {
		for _, st := range statuses {
			for _, cl := range classes {
				for _, win := range windows {
					tmpl := mustParse(t, template("p", ty, st, cl, win[0], win[1]))

					served, err := projection.Apply(v, tmpl, consent.Open{})
					if err != nil {
						t.Fatalf("Apply: %v", err)
					}
					verdicts, err := projection.Explain(v, tmpl, consent.Open{})
					if err != nil {
						t.Fatalf("Explain: %v", err)
					}

					byExplain := []string{}
					for _, x := range verdicts {
						if x.Reason.Served() {
							byExplain = append(byExplain, x.Entry.FacetID)
						}
					}
					if got := ids(served); !reflect.DeepEqual(got, byExplain) {
						t.Fatalf("Apply and Explain disagree for %v/%v/%s/%v:\n Apply   %v\n Explain %v",
							ty, st, cl, win, got, byExplain)
					}

					// Every entry gets exactly one verdict, and every non-served one
					// names a reason rather than shrugging.
					if len(verdicts) != v.Len() {
						t.Fatalf("Explain returned %d verdicts for %d entries", len(verdicts), v.Len())
					}
					for _, x := range verdicts {
						if !x.Reason.Served() && x.Reason == "" {
							t.Fatalf("entry %s excluded with an empty reason", x.Entry.FacetID)
						}
					}
				}
			}
		}
	}
}

// Each knob must be able to be the named cause, or the explanation is not telling the
// operator anything they could act on.
func TestExplain_NamesTheKnobThatExcluded(t *testing.T) {
	t.Parallel()

	all := []string{"health.blood-pressure", "health.sleep", "health.heart-rate"}
	everyStatus := []string{"admitted", "endorsed", "tombstoned"}

	cases := []struct {
		name   string
		tmpl   string
		facet  string
		reason projection.Reason
	}{
		{"type", template("t", []string{"health.sleep"}, everyStatus, "Sensitive", "", ""),
			"f-bp-1", projection.ReasonType},
		{"status", template("t", all, []string{"admitted"}, "Sensitive", "", ""),
			"f-bp-2", projection.ReasonStatus},
		{"classification", template("t", all, everyStatus, "Normal", "", ""),
			"f-hr-1", projection.ReasonClassification},
		{"before the window", template("t", all, everyStatus, "Sensitive", "2026-02-01T00:00:00.000000000Z", ""),
			"f-bp-1", projection.ReasonBeforeWindow},
		{"at or after the window", template("t", all, everyStatus, "Sensitive", "", "2026-01-25T00:00:00.000000000Z"),
			"f-sleep-1", projection.ReasonAfterWindow},
		{"served", template("t", all, everyStatus, "Sensitive", "", ""),
			"f-bp-1", projection.ReasonServed},
	}

	v := canonical(t)
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			verdicts, err := projection.Explain(v, mustParse(t, c.tmpl), consent.Open{})
			if err != nil {
				t.Fatalf("Explain: %v", err)
			}
			for _, x := range verdicts {
				if x.Entry.FacetID != c.facet {
					continue
				}
				if x.Reason != c.reason {
					t.Errorf("%s: reason = %q, want %q", c.facet, x.Reason, c.reason)
				}
				return
			}
			t.Fatalf("%s not in the verdicts", c.facet)
		})
	}
}

// A consent denial is reported as such, carrying the gate's own words, rather than
// being folded into whichever template knob happens to be nearby.
func TestExplain_ConsentDenialIsItsOwnReason(t *testing.T) {
	t.Parallel()

	tmpl := mustParse(t, template("t",
		[]string{"health.blood-pressure", "health.sleep"},
		[]string{"admitted", "endorsed", "tombstoned"}, "Sensitive", "", ""))

	verdicts, err := projection.Explain(canonical(t), tmpl, denyType{typ: "health.blood-pressure"})
	if err != nil {
		t.Fatalf("Explain: %v", err)
	}

	var denied, served int
	for _, x := range verdicts {
		switch x.Entry.Type {
		case "health.blood-pressure":
			if x.Reason != projection.ReasonConsent {
				t.Errorf("%s: reason = %q, want %q", x.Entry.FacetID, x.Reason, projection.ReasonConsent)
			}
			if x.Detail != "denied by test policy" {
				t.Errorf("%s: detail = %q, want the gate's own words", x.Entry.FacetID, x.Detail)
			}
			denied++
		case "health.sleep":
			if x.Reason.Served() {
				served++
			}
		}
	}
	if denied != 3 || served != 1 {
		t.Errorf("denied=%d served=%d, want 3 and 1", denied, served)
	}
}

// A facet the template already excluded must not be attributed to the gate: consent is
// asked only about what a consumer would otherwise see, so a type outside the
// allow-list was never a disclosure the gate had an opinion about.
func TestExplain_GateIsNotConsultedAboutAlreadyExcluded(t *testing.T) {
	t.Parallel()

	tmpl := mustParse(t, template("t", []string{"health.sleep"},
		[]string{"admitted", "endorsed", "tombstoned"}, "Sensitive", "", ""))

	verdicts, err := projection.Explain(canonical(t), tmpl, denyType{typ: "health.blood-pressure"})
	if err != nil {
		t.Fatalf("Explain: %v", err)
	}
	for _, x := range verdicts {
		if x.Entry.Type == "health.blood-pressure" && x.Reason != projection.ReasonType {
			t.Errorf("%s: reason = %q, want %q — the template excluded it before the gate saw it",
				x.Entry.FacetID, x.Reason, projection.ReasonType)
		}
	}
}

// An entry excluded by two knobs is attributed to the first in documented order, so the
// answer is stable rather than dependent on evaluation order.
func TestExplain_MultipleFailuresAttributeToTheFirstKnob(t *testing.T) {
	t.Parallel()

	// f-hr-1 is health.heart-rate, admitted, Sensitive, 2026-01-26. This template
	// excludes it by type AND by classification AND by window.
	tmpl := mustParse(t, template("t", []string{"health.sleep"}, []string{"admitted"},
		"Normal", "2026-03-01T00:00:00.000000000Z", ""))

	verdicts, err := projection.Explain(canonical(t), tmpl, consent.Open{})
	if err != nil {
		t.Fatalf("Explain: %v", err)
	}
	for _, x := range verdicts {
		if x.Entry.FacetID == "f-hr-1" && x.Reason != projection.ReasonType {
			t.Errorf("reason = %q, want %q (type is the first knob)", x.Reason, projection.ReasonType)
		}
	}
}

func TestExplain_RequiresAGate(t *testing.T) {
	t.Parallel()

	tmpl := mustParse(t, template("t", []string{"health.sleep"}, []string{"admitted"}, "Normal", "", ""))
	if _, err := projection.Explain(canonical(t), tmpl, nil); err == nil {
		t.Error("Explain with no consent gate was accepted")
	}
}
