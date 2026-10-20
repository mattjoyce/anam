package facet_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/facet"
)

func TestTypeValid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   facet.Type
		want bool
	}{
		{"simple namespaced", "health.sleep", true},
		{"hyphenated leaf", "health.blood-pressure", true},
		{"hyphenated leaf, multiple", "health.body-composition", true},
		{"digits allowed", "health.spo2", true},
		{"deep namespace", "health.vitals.blood-pressure", true},
		{"future namespaces", "calendar.event", true},
		{"note namespace", "note.zettel", true},

		{"empty", "", false},
		{"unnamespaced", "health", false},
		{"upper case", "Health.Sleep", false},
		{"snake case leaf", "health.blood_pressure", false},
		{"trailing dot", "health.", false},
		{"leading dot", ".sleep", false},
		{"empty segment", "health..sleep", false},
		{"leading hyphen in leaf", "health.-pressure", false},
		{"trailing hyphen in leaf", "health.pressure-", false},
		{"double hyphen", "health.blood--pressure", false},
		{"leading digit in segment", "health.2spo", false},
		{"space", "health.blood pressure", false},
		{"slash", "health/sleep", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.in.Valid(); got != tt.want {
				t.Errorf("Type(%q).Valid() = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestTypeNamespace(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in   facet.Type
		want string
	}{
		{"health.sleep", "health"},
		{"health.vitals.blood-pressure", "health"},
		{"calendar.event", "calendar"},
		{"health", ""},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(string(tt.in), func(t *testing.T) {
			t.Parallel()
			if got := tt.in.Namespace(); got != tt.want {
				t.Errorf("Type(%q).Namespace() = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestEnumValid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   interface{ Valid() bool }
		want bool
	}{
		{"classification normal", facet.ClassificationNormal, true},
		{"classification confidential", facet.ClassificationConfidential, true},
		{"classification sensitive", facet.ClassificationSensitive, true},
		{"classification unknown", facet.Classification("Public"), false},
		{"classification lower case", facet.Classification("normal"), false},
		{"classification empty", facet.Classification(""), false},

		{"status admitted", facet.StatusAdmitted, true},
		{"status superseded", facet.StatusSuperseded, true},
		{"status tombstoned", facet.StatusTombstoned, true},
		{"status endorsed", facet.StatusEndorsed, true},
		{"status rejected", facet.StatusRejected, true},
		{"status published is not a status", facet.Status("published"), false},
		{"status empty", facet.Status(""), false},

		{"event admitted", facet.EventAdmitted, true},
		{"event superseded", facet.EventSuperseded, true},
		{"event tombstoned", facet.EventTombstoned, true},
		{"event endorsed", facet.EventEndorsed, true},
		{"event rejected", facet.EventRejected, true},
		{"event published is not an anam-event", facet.AnamEventKind("published"), false},
		{"event unknown", facet.AnamEventKind("deleted"), false},
		{"event empty", facet.AnamEventKind(""), false},

		{"payload verbatim", facet.PayloadVerbatim, true},
		{"payload reference", facet.PayloadReference, true},
		{"payload unknown", facet.PayloadKind("inline"), false},

		{"assurance high", facet.AssuranceHigh, true},
		{"assurance medium", facet.AssuranceMedium, true},
		{"assurance review", facet.AssuranceReview, true},
		{"assurance low is not a level", facet.BindingAssurance("low"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.in.Valid(); got != tt.want {
				t.Errorf("%#v.Valid() = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestEnvelopeValidate(t *testing.T) {
	t.Parallel()

	valid := facet.Envelope{
		Type:      "health.blood-pressure",
		Source:    "bp-csv",
		Injector:  "bp-csv",
		SourceRef: "bp-csv:2026-01-24T06:30:00:118/76/61",
		Payload:   []byte(`{"systolic":"118"}`),
	}

	tests := []struct {
		name    string
		mutate  func(*facet.Envelope)
		wantErr bool
	}{
		{"valid", func(*facet.Envelope) {}, false},
		{"valid with optional fields", func(e *facet.Envelope) {
			e.AssertedPerson = "matt"
			e.SpanHint = "2026-01"
		}, false},
		{"missing type", func(e *facet.Envelope) { e.Type = "" }, true},
		{"malformed type", func(e *facet.Envelope) { e.Type = "blood_pressure" }, true},
		{"missing source", func(e *facet.Envelope) { e.Source = "" }, true},
		{"missing injector", func(e *facet.Envelope) { e.Injector = "" }, true},
		{"no source_ref: the gate composes it", func(e *facet.Envelope) { e.SourceRef = "" }, false},
		{"nil payload", func(e *facet.Envelope) { e.Payload = nil }, true},
		{"empty payload", func(e *facet.Envelope) { e.Payload = []byte{} }, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			env := valid
			tt.mutate(&env)
			err := env.Validate()

			if tt.wantErr {
				if err == nil {
					t.Fatalf("Validate() = nil, want error")
				}
				if !errors.Is(err, facet.ErrInvalidEnvelope) {
					t.Errorf("Validate() error = %v, want it to wrap ErrInvalidEnvelope", err)
				}
				return
			}
			if err != nil {
				t.Errorf("Validate() = %v, want nil", err)
			}
		})
	}
}

// TestFacetHasNoCurrentStatusField guards the invariant in ARCHITECTURE.md §12: a
// Facet's current status is derived by the fold from anam-events and is never stored
// on the artifact. A field named Status here would be a second source of truth and a
// place to overwrite, which is exactly what the append-only ledger forbids.
//
// No field is exempt. BirthStatus used to be, until it was found to carry no
// information a nil-check on Derivation did not already carry.
func TestFacetHasNoCurrentStatusField(t *testing.T) {
	t.Parallel()

	rt := reflect.TypeOf(facet.Facet{})
	for i := range rt.NumField() {
		name := rt.Field(i).Name
		if strings.Contains(name, "Status") {
			t.Errorf("Facet has field %q: current status must be derived by the fold, never stored", name)
		}
	}
}

// TestAnamEventReferencesFacetNotTheReverse guards the one-directional dependency in
// §5: an anam-event references a Facet; a Facet never references an anam-event,
// because the world does not know Anam exists. Reversing it would break
// rebuild-from-source.
func TestAnamEventReferencesFacetNotTheReverse(t *testing.T) {
	t.Parallel()

	if _, ok := reflect.TypeOf(facet.AnamEvent{}).FieldByName("FacetID"); !ok {
		t.Error("AnamEvent must reference the Facet it concerns via FacetID")
	}

	rt := reflect.TypeOf(facet.Facet{})
	for i := range rt.NumField() {
		name := rt.Field(i).Name
		if strings.Contains(name, "AnamEvent") || strings.Contains(name, "Seq") {
			t.Errorf("Facet has field %q: a Facet must never reference an anam-event", name)
		}
	}
}

func TestMatches(t *testing.T) {
	tests := []struct {
		name    string
		typ     facet.Type
		pattern string
		want    bool
	}{
		{"exact", "health.sleep", "health.sleep", true},
		{"exact other", "health.sleep", "health.weight", false},
		{"exact is not a prefix", "health.sleep.night", "health.sleep", false},
		{"prefix matches a leaf", "health.heart-rate.day", "health.heart-rate.", true},
		{"prefix matches a deeper leaf", "health.heart-rate.day.garmin", "health.heart-rate.", true},
		{"prefix stops at the segment boundary", "health.heart-rate-variability", "health.heart-rate.", false},
		{"prefix does not match its own concept", "health.heart-rate", "health.heart-rate.", false},
		{"prefix does not match another concept", "health.weight.day", "health.heart-rate.", false},
		{"empty pattern matches nothing", "health.sleep", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := facet.Matches(tc.typ, tc.pattern); got != tc.want {
				t.Errorf("Matches(%q, %q) = %v, want %v", tc.typ, tc.pattern, got, tc.want)
			}
		})
	}
}

func TestValidPattern(t *testing.T) {
	tests := []struct {
		pattern string
		want    bool
	}{
		{"health.sleep", true},
		{"health.heart-rate.", true},
		{"health.heart-rate.day.", true},
		{"health.", false},
		{"health", false},
		{".", false},
		{"", false},
		{"Health.sleep.", false},
		{"health.sleep..", false},
		{"health..sleep.", false},
		{"health.sleep-", false},
		{"health.sleep*", false},
	}
	for _, tc := range tests {
		t.Run(tc.pattern, func(t *testing.T) {
			if got := facet.ValidPattern(tc.pattern); got != tc.want {
				t.Errorf("ValidPattern(%q) = %v, want %v", tc.pattern, got, tc.want)
			}
		})
	}
}
