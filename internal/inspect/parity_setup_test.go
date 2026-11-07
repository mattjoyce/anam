package inspect

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/mattjoyce/anam/internal/config"
	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/presentation"
	"github.com/mattjoyce/anam/internal/span"
	"github.com/mattjoyce/anam/internal/store/ledger"
)

// Only these two adapters know where the personal grammar comes from. In 256c,
// currentParityGrammar must read the compiled presentation fixture's queries and
// parityHandler must supply that same fixture through Deps. Leave the pinned
// literals and the rendering assertions unchanged. Never set package globals.
// The compiled presentation must enumerate its ordered domains, its ordered
// families and its complete override set. An adapter that could only look up the
// 28 pinned keys would make TestParityGrammar's count check pass by construction.
func parityPresentation() (presentation.Value, error) {
	c, err := config.LoadOverriding("testdata/personal-inspector.yaml", map[string]string{"subject_id": "parity-subject", "server": ""})
	return c.Presentation(), err
}

func currentParityGrammar() parityGrammar {
	g := parityGrammar{Overrides: map[string]parityOverride{}}
	p, err := parityPresentation()
	if err != nil {
		// The independent count check fails on a broken fixture.
		return g
	}
	for _, d := range p.Domains() {
		g.Domains = append(g.Domains, parityDomain{d.ID, d.Hue})
	}
	for _, f := range p.Families() {
		g.Families = append(g.Families, parityFamily{f.ID, f.Label})
	}
	for _, o := range p.Overrides() {
		g.Overrides[string(o.Type)] = parityOverride{string(o.Type), o.LegacyFamily, o.LegacyKind, o.Shape}
	}
	return g
}

func parityHandler(d Deps) http.Handler {
	p, err := parityPresentation()
	if err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		})
	}
	d.Presentation = p
	return Handler(d)
}

// These render-only Definitions deliberately include old incomplete metadata.
// NewRegistry holds them without inventing or rewriting historical manifest bytes.
// A Definition for an off-vocabulary head alone must not create a lane.
func parityRegistry(t *testing.T) *def.Registry {
	t.Helper()
	r, err := def.NewRegistry(
		def.Definition{Type: "health.sleep"},
		def.Definition{Type: "health.intake", Family: "intake", Kind: def.KindEvent},
		def.Definition{Type: "health.note", Family: "notes", Kind: def.KindEvent},
		def.Definition{Type: "health.alpha", Family: "alpha", Kind: def.KindReading},
		def.Definition{Type: "health.zeta", Family: "zeta", Kind: def.KindMeasure},
		def.Definition{Type: "health.aa-reading", Family: "vitals", Kind: def.KindReading},
		def.Definition{Type: "health.zz-measure", Family: "vitals", Kind: def.KindMeasure},
		def.Definition{Type: "health.aa-event", Family: "vitals", Kind: def.KindEvent},
		def.Definition{Type: "place.sample", Family: "location", Kind: def.KindMeasure},
		def.Definition{Type: "obsolete.name", Family: "documents", Kind: def.KindEvent},
	)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

type paritySeed struct {
	Type string
	Day  int
}

// A real ledger at W, followed by a tombstone outside W. The reader exposes the
// historical head because the timeline has no as-of URL parameter. All replay and
// facet reads still go to the real ledger, including fold's normal Read path.
type paritySnapshot struct {
	Ledger
	w ledger.Watermark
}

func (l paritySnapshot) Head(context.Context) (ledger.Watermark, error) { return l.w, nil }

func parityLedger(t *testing.T, seeds []paritySeed) Ledger {
	t.Helper()
	ctx := context.Background()
	payload, err := facet.Canonicalize([]byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	l, err := ledger.Create(ctx, filepath.Join(t.TempDir(), "ledger.db"), "parity-subject")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	var first facet.Facet
	for i, seed := range seeds {
		ref := fmt.Sprintf("parity:%04d", i)
		f := facet.Facet{
			ID: facet.NewID(ref), Type: facet.Type(seed.Type), Confidence: 1,
			Classification: facet.ClassificationNormal, ArtifactVersion: 1, DefinitionVersion: 1,
			FactTime:   time.Date(2026, 1, seed.Day, 0, 0, 0, 0, time.UTC),
			Binding:    facet.Binding{SubjectID: "parity-subject", Assurance: facet.AssuranceHigh},
			Provenance: facet.Provenance{Source: "parity", Injector: "parity", SourceRef: ref},
			Descriptor: facet.Descriptor{SourceSchema: "parity", SchemaVersion: 1,
				ContentHash: facet.ContentHash(payload), PayloadKind: facet.PayloadVerbatim, MediaType: "application/json"},
			Payload: payload,
		}
		if i == 0 {
			first = f
		}
		seq := int64(i + 1)
		ev := parityEvent(facet.EventAdmitted, f, seq)
		if err := l.Admit(ctx, ledger.Watermark(i), f, ev); err != nil {
			t.Fatal(err)
		}
	}
	w := ledger.Watermark(len(seeds))
	if err := l.Mark(ctx, w, parityEvent(facet.EventTombstoned, first, int64(w)+1)); err != nil {
		t.Fatal(err)
	}
	return paritySnapshot{Ledger: l, w: w}
}

func parityEvent(k facet.AnamEventKind, f facet.Facet, seq int64) facet.AnamEvent {
	return facet.AnamEvent{Kind: k, FacetID: f.ID, Seq: seq, ArtifactVersion: 1,
		AnamTime: time.Date(2026, 4, 1, 0, 0, 0, int(seq), time.UTC)}
}

func facetIDForParitySeed(i int) string { return facet.NewID(fmt.Sprintf("parity:%04d", i)) }

func parityPage(t *testing.T, l Ledger, reg *def.Registry, query string) *parityNode {
	t.Helper()
	buckets, err := span.New(span.Monthly)
	if err != nil {
		t.Fatal(err)
	}
	h := parityHandler(Deps{Ledger: l, Timeout: 30 * time.Second, Buckets: buckets,
		Definitions: func() *def.Registry { return reg }, Zone: "UTC",
		Now: func() time.Time { return time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC) }})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet,
		"/?from=2026-01-01T00:00:00Z&to=2026-03-09T00:00:00Z&zone=UTC"+query, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("timeline returned %d\n%s", w.Code, w.Body.String())
	}
	return parseParityHTML(t, w.Body.String())
}
