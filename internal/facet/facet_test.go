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

		{"birth admitted", facet.BornAdmitted, true},
		{"birth proposed", facet.BornProposed, true},
		{"birth superseded is not a birth", facet.BirthStatus("superseded"), false},
		{"birth empty", facet.BirthStatus(""), false},

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
		{"event published", facet.EventPublished, true},
		{"event unknown", facet.RecordEventKind("deleted"), false},
		{"event empty", facet.RecordEventKind(""), false},

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
		{"missing source_ref", func(e *facet.Envelope) { e.SourceRef = "" }, true},
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
// Facet's current status is derived by the fold from record-events and is never stored
// on the artifact. A field named Status here would be a second source of truth and a
// place to overwrite, which is exactly what the append-only ledger forbids.
//
// BirthStatus is permitted: it is the immutable state the artifact was born in.
func TestFacetHasNoCurrentStatusField(t *testing.T) {
	t.Parallel()

	rt := reflect.TypeOf(facet.Facet{})
	for i := range rt.NumField() {
		name := rt.Field(i).Name
		if name == "BirthStatus" {
			continue
		}
		if strings.Contains(name, "Status") {
			t.Errorf("Facet has field %q: current status must be derived by the fold, never stored", name)
		}
	}
}

// TestRecordEventReferencesFacetNotTheReverse guards the one-directional dependency in
// §5: a record-event references a Facet; a Facet never references a record-event,
// because the world does not know Anam exists. Reversing it would break
// rebuild-from-source.
func TestRecordEventReferencesFacetNotTheReverse(t *testing.T) {
	t.Parallel()

	if _, ok := reflect.TypeOf(facet.RecordEvent{}).FieldByName("FacetID"); !ok {
		t.Error("RecordEvent must reference the Facet it concerns via FacetID")
	}

	rt := reflect.TypeOf(facet.Facet{})
	for i := range rt.NumField() {
		name := rt.Field(i).Name
		if strings.Contains(name, "RecordEvent") || strings.Contains(name, "Seq") {
			t.Errorf("Facet has field %q: a Facet must never reference a record-event", name)
		}
	}
}
