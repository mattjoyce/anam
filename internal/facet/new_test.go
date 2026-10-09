package facet_test

import (
	"errors"
	"testing"
	"time"

	"github.com/mattjoyce/anam/internal/facet"
)

func validParams(t *testing.T) facet.NewParams {
	t.Helper()
	c, err := facet.Canonicalize([]byte(`{"systolic": 118}`))
	if err != nil {
		t.Fatal(err)
	}
	et := time.Date(2026, 1, 24, 6, 30, 0, 0, time.UTC)
	return facet.NewParams{
		Envelope: facet.Envelope{
			Type: "health.blood-pressure", Source: "bp-csv", Injector: "bp-csv",
			SourceRef: "bp-csv:2026-01-24T06:30:00", Payload: []byte(`{"systolic": 118}`),
		},
		Binding:         facet.Binding{PersonID: "matt", Assurance: facet.AssuranceHigh},
		Payload:         c,
		ArtifactVersion: 1, DefinitionVersion: 1,
		Classification: facet.ClassificationNormal,
		SourceSchema:   "bp-csv/reading", SchemaVersion: 1, MediaType: "application/json",
		Confidence: 1,
		FactTime:   et,
	}
}

// New computes identity and integrity itself: the ID from the source ref, the hash
// from the payload. Neither can be asserted independently of what it describes.
func TestNew_DerivesIdentityAndIntegrity(t *testing.T) {
	t.Parallel()
	p := validParams(t)

	f, err := facet.New(p)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if want := facet.NewID(p.Envelope.SourceRef); f.ID != want {
		t.Errorf("ID = %s, want %s (uuidv5 of source ref)", f.ID, want)
	}
	if want := facet.ContentHash(p.Payload); f.Descriptor.ContentHash != want {
		t.Errorf("ContentHash = %s, want %s (hash of own payload)", f.Descriptor.ContentHash, want)
	}
	if f.Descriptor.PayloadKind != facet.PayloadVerbatim {
		t.Errorf("PayloadKind = %s, want verbatim (references are reserved)", f.Descriptor.PayloadKind)
	}
	if f.Provenance.Derivation != nil {
		t.Error("captured fact carries a derivation")
	}
}

// A derived facet is marked by its Derivation and nothing else — the presence of the
// provenance IS the claim, so there is no second field to disagree with it.
func TestNew_DerivedFacetIsMarkedByItsDerivation(t *testing.T) {
	t.Parallel()
	p := validParams(t)
	p.Derivation = &facet.Derivation{Method: "trend", Model: "hr-drift-1", Watermark: "42"}

	f, err := facet.New(p)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if f.Provenance.Derivation == nil || f.Provenance.Derivation.Watermark != "42" {
		t.Errorf("derivation not carried: %+v", f.Provenance.Derivation)
	}
}

func TestNew_RejectsIllegalParams(t *testing.T) {
	t.Parallel()

	mutations := map[string]func(*facet.NewParams){
		"invalid envelope type":   func(p *facet.NewParams) { p.Envelope.Type = "notdotted" },
		"no source_ref":           func(p *facet.NewParams) { p.Envelope.SourceRef = "" },
		"empty binding person":    func(p *facet.NewParams) { p.Binding.PersonID = "" },
		"bad assurance":           func(p *facet.NewParams) { p.Binding.Assurance = "low" },
		"empty payload":           func(p *facet.NewParams) { p.Payload = facet.Canonical{} },
		"artifact version zero":   func(p *facet.NewParams) { p.ArtifactVersion = 0 },
		"definition version zero": func(p *facet.NewParams) { p.DefinitionVersion = 0 },
		"bad classification":      func(p *facet.NewParams) { p.Classification = "Public" },
		"no source schema":        func(p *facet.NewParams) { p.SourceSchema = "" },
		"schema version zero":     func(p *facet.NewParams) { p.SchemaVersion = 0 },
		"no media type":           func(p *facet.NewParams) { p.MediaType = "" },
		"confidence zero":         func(p *facet.NewParams) { p.Confidence = 0 },
		"confidence above one":    func(p *facet.NewParams) { p.Confidence = 1.1 },
		"zero fact time":          func(p *facet.NewParams) { p.FactTime = time.Time{} },
	}

	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			p := validParams(t)
			mutate(&p)
			if _, err := facet.New(p); !errors.Is(err, facet.ErrInvalidFacet) {
				t.Errorf("New = %v, want ErrInvalidFacet", err)
			}
		})
	}
}
