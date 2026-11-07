package inspect

import (
	"fmt"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/presentation"
)

func compilePolicy(t *testing.T, spec presentation.Spec) presentation.Value {
	t.Helper()
	p, err := presentation.Compile(spec)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestPresentationDefaultsAndOrdering(t *testing.T) {
	t.Parallel()
	reg, err := def.NewRegistry(
		def.Definition{Type: "health.a", Family: "zeta", Kind: def.KindEvent},
		def.Definition{Type: "health.b", Family: "alpha", Kind: def.KindReading},
		def.Definition{Type: "health.c", Family: "alpha", Kind: def.KindMeasure},
		def.Definition{Type: "health.d", Family: "alpha", Kind: def.KindEvent},
		def.Definition{Type: "health.e", Family: "alpha", Kind: def.KindReading},
		def.Definition{Type: "money.empty"}, def.Definition{Type: "retired.empty"},
	)
	if err != nil {
		t.Fatal(err)
	}
	hue := 2
	p := compilePolicy(t, presentation.Spec{Domains: []presentation.DomainSpec{{ID: "retired", Hue: &hue}, {ID: "health", Hue: &hue}}, Families: []presentation.Family{{ID: "zeta", Label: "Z"}}})
	for _, policy := range []presentation.Value{{}, p} {
		st := newStyles(reg, policy)
		for range 40 {
			types := []facet.Type{"health.a", "health.b", "health.c", "health.d", "health.e", "health.unknown", "custom.head"}
			rand.Shuffle(len(types), func(i, j int) { types[i], types[j] = types[j], types[i] })
			lanes := st.lanesFor(types)
			var ids []string
			for _, l := range lanes {
				ids = append(ids, l.ID)
			}
			wantIDs := []string{"custom", "health", "money"}
			wantRows := []string{"alpha", "zeta", "other"}
			if len(policy.Domains()) > 0 {
				wantIDs = []string{"health", "custom", "money"}
				wantRows = []string{"zeta", "alpha", "other"}
			}
			if !reflect.DeepEqual(ids, wantIDs) {
				t.Fatalf("lanes %v, want %v", ids, wantIDs)
			}
			for _, l := range lanes {
				if l.Hue != policy.Hue(l.ID) {
					t.Fatal("lane hue differs from policy")
				}
				if l.ID != "health" {
					continue
				}
				var rows []string
				for _, row := range l.Families {
					rows = append(rows, row.Family)
					if row.Hue != l.Hue {
						t.Fatal("unknown family did not inherit domain hue")
					}
					if row.Family == "alpha" && !reflect.DeepEqual(row.Types, []facet.Type{"health.c", "health.b", "health.e", "health.d"}) {
						t.Fatalf("kind/type order %v", row.Types)
					}
				}
				if !reflect.DeepEqual(rows, wantRows) {
					t.Fatalf("family order %v, want %v", rows, wantRows)
				}
			}
			if st.markClass("custom.head") != "hx" || st.of("health.unknown") != (typeStyle{Family: "other", Shape: shapeCircle, Kind: kindMeasure}) {
				t.Fatal("unknown defaults inherited preferences")
			}
		}
	}
	if lanes := newStyles(nil, presentation.Value{}).lanesFor(nil); len(lanes) != 0 {
		t.Fatal("empty catalog has lanes")
	}
	for _, id := range []string{"health", "activity", "place", "record", "mind", "org", "money", "social", "media", "work"} {
		r, err := def.NewRegistry(def.Definition{Type: facet.Type(id + ".empty")})
		if err != nil {
			t.Fatal(err)
		}
		if lanes := newStyles(r, presentation.Value{}).lanesFor(nil); len(lanes) != 1 || lanes[0].ID != id || lanes[0].Hue != "hx" {
			t.Fatalf("vocabulary depends on policy: %s %+v", id, lanes)
		}
	}
}

func TestPresentationShapeGuards(t *testing.T) {
	t.Parallel()
	p := compilePolicy(t, presentation.Spec{Types: []presentation.Override{
		{Type: "custom.guarded", Shape: "star", LegacyKind: "reading", LegacyFamily: "legacy"},
		{Type: "custom.unguarded", Shape: "cross"},
	}})
	for _, kind := range []def.Kind{"", def.KindMeasure, def.KindReading, def.KindEvent} {
		r, err := def.NewRegistry(def.Definition{Type: "custom.guarded", Family: "declared", Kind: kind}, def.Definition{Type: "custom.unguarded", Kind: kind})
		if err != nil {
			t.Fatal(err)
		}
		st := newStyles(r, p)
		got := st.of("custom.guarded")
		wantKind := kindOf(kind)
		if kind == "" {
			wantKind = kindReading
		}
		wantShape := shapeFor(wantKind)
		if wantKind == kindReading {
			wantShape = shapeStar
		}
		if got.Family != "declared" || got.Kind != wantKind || got.Shape != wantShape {
			t.Fatalf("Definition priority/guard: %s %+v", kind, got)
		}
		if st.of("custom.unguarded").Shape != shapeCross {
			t.Fatal("shape-only override suppressed")
		}
	}
}

func policyPage(h http.Handler, query string) (int, string) {
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/?view=whole&zone=UTC"+query, nil))
	return w.Code, w.Body.String()
}

func TestPresentationConcurrentServersAndRegistration(t *testing.T) {
	t.Parallel()
	l := parityLedger(t, []paritySeed{{"health.sample", 2}, {"health.sample", 3}})
	var registry atomic.Pointer[def.Registry]
	var reads atomic.Int64
	hueA, hueB := 0, 6
	handler := func(hue *int, label string) http.Handler {
		p := compilePolicy(t, presentation.Spec{Domains: []presentation.DomainSpec{{ID: "health", Hue: hue}}, Families: []presentation.Family{{ID: "custom", Label: label}}, Types: []presentation.Override{{Type: "health.sample", Shape: "star", LegacyKind: "reading", LegacyFamily: "custom"}}})
		h := Handler(Deps{Ledger: l, Timeout: 30 * time.Second, Presentation: p, Definitions: func() *def.Registry { reads.Add(1); return registry.Load() }})
		p.Families()[0].Label = "mutated"
		return h
	}
	a, b := handler(&hueA, " <A> "), handler(&hueB, "B")
	for _, tc := range []struct {
		h          http.Handler
		hue, label string
	}{{a, "h0", " &lt;A&gt; "}, {b, "h6", "B"}} {
		t.Run(tc.hue, func(t *testing.T) {
			t.Parallel()
			for range 8 {
				code, body := policyPage(tc.h, "&open=health")
				if code != 200 || !strings.Contains(body, `class="`+tc.hue+` hollow"`) || !strings.Contains(body, tc.label) || strings.Contains(body, "mutated") {
					t.Fatalf("presentation leaked between servers: %d", code)
				}
			}
		})
	}
	// Registration replaces an immutable registry between renders, without changing policy.
	t.Run("registration", func(t *testing.T) {
		before := reads.Load()
		_, body := policyPage(a, "&open=health")
		if reads.Load()-before != 1 {
			t.Fatal("more than one registry snapshot per render")
		}
		if !strings.Contains(body, `class="h0 hollow"`) {
			t.Fatal("nil registry lost historical fallback")
		}
		reg, err := def.NewRegistry(def.Definition{Type: "health.sample", Kind: def.KindEvent, Family: "registered"})
		if err != nil {
			t.Fatal(err)
		}
		registry.Store(reg)
		_, body = policyPage(a, "&open=health&from=2026-01-02&to=2026-01-03")
		if !strings.Contains(body, ">registered</text>") || strings.Contains(body, `class="h0 hollow"`) {
			t.Fatal("later registration invisible or guarded shape retained")
		}
		point := parseParityHTML(t, body).one(t, "svg", "band").find("a", "pt")
		if len(point) != 2 {
			t.Fatalf("lost historical facets: %d", len(point))
		}
		for _, pt := range point {
			if got := pt.one(t, "path", "").Attr["d"]; !strings.Contains(got, "l") {
				t.Fatalf("event did not get diamond: %s", got)
			}
		}
	})
	registry.Store(nil)
}

func TestPresentationEveryColourSurface(t *testing.T) {
	t.Parallel()
	seeds := []paritySeed{{"health.sample", 2}, {"health.sample", 3}}
	for i := 0; i < 1501; i++ {
		seeds = append(seeds, paritySeed{"health.sample", 4})
	}
	l := parityLedger(t, seeds)
	hue := 4
	p := compilePolicy(t, presentation.Spec{Domains: []presentation.DomainSpec{{ID: "health", Hue: &hue}}})
	h := Handler(Deps{Ledger: l, Timeout: 30 * time.Second, Presentation: p, Definitions: func() *def.Registry { return nil }})
	for _, query := range []string{"", "&open=health", "&type=health.sample", "&from=2026-01-01&to=2026-01-05"} {
		code, body := policyPage(h, query)
		if code != 200 {
			t.Fatal(code)
		}
		for _, surface := range []string{`class="sw h4"`, `class="h4"`} {
			if !strings.Contains(body, surface) {
				t.Errorf("surface %s lost policy for %s", surface, query)
			}
		}
		if !strings.Contains(body, `class="tally"`) || !strings.Contains(body, `class="run"`) {
			t.Fatal("fixture did not exercise run/tally")
		}
	}
	payload, err := facet.Canonicalize([]byte(`{"device":"test","points":[{"t":"2026-01-01T00:00:00Z","lat":1,"lon":2}]}`))
	if err != nil {
		t.Fatal(err)
	}
	f := facet.Facet{Type: "place.track", Payload: payload}
	track := facetTrack(f, p.Hue("place"))
	if track == nil {
		t.Fatal("no track")
	}
	if got := fmt.Sprint(track); strings.Contains(got, "h1") {
		t.Fatal("facet track retained personal hue")
	}
}
