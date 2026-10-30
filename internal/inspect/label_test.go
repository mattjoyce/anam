package inspect_test

import (
	"context"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/inspect"
	"github.com/mattjoyce/anam/internal/store/ledger"
)

// Card 199, slice 3: a Definition's label reads a row as a thing. Synthetic facets of
// two labelled types (a weight, a dose), one type whose Definition declares no label (a
// heart-rate alert), a weight whose payload holds nothing the label reads, and a type no
// Definition names.
var labelled = []struct {
	pick
	at      time.Time
	payload string
}{
	{pick{"wt", "health.weight", "records"}, time.Date(2026, 10, 2, 7, 0, 0, 0, time.UTC), `{"kg": 80.4}`},
	{pick{"dose", "health.medication.dose", "conversation"}, time.Date(2026, 10, 2, 8, 0, 0, 0, time.UTC), `{"drug": "melatonin", "amount": 5, "unit": "mg"}`},
	{pick{"alert", "health.heart-rate-alert", "garmin"}, time.Date(2026, 10, 2, 9, 0, 0, 0, time.UTC), `{"heart_rate": 101}`},
	{pick{"bare", "health.weight", "records"}, time.Date(2026, 10, 2, 10, 0, 0, 0, time.UTC), `{"n": 1}`},
	{pick{"odd", "x.unnamed", "records"}, time.Date(2026, 10, 2, 11, 0, 0, 0, time.UTC), `{"kg": 1}`},
}

func labelServer(t *testing.T) *httptest.Server {
	t.Helper()
	ctx := context.Background()
	l, err := ledger.Create(ctx, filepath.Join(t.TempDir(), "ledger.db"), person)
	if err != nil {
		t.Fatalf("ledger.Create: %v", err)
	}
	t.Cleanup(func() { _ = l.Close() })
	for i, p := range labelled {
		f := seedFacet(t, "sel:"+p.ref, p.typ, p.payload)
		f.FactTime, f.Provenance.Source = p.at, p.source
		if err := l.Admit(ctx, ledger.Watermark(i), f, event(facet.EventAdmitted, f, int64(i+1))); err != nil {
			t.Fatalf("Admit %s: %v", p.ref, err)
		}
	}
	srv := httptest.NewServer(inspect.Handler(inspect.Deps{Catalog: testCatalog(), Ledger: l, Timeout: 30 * time.Second, Buckets: defaultBuckets, MaxReadBytes: 512 << 10}))
	t.Cleanup(srv.Close)
	return srv
}

// memberRE finds one member of a row: its box's id, then how its link reads.
var memberRE = regexp.MustCompile(`name="on" value="([^"]+)"[^>]*> (?:<a class="member-id member-label"[^>]*>([^<]*)</a> <span class="mono dim">([^<]*)</span>|<a class="mono member-id"[^>]*>([^<]*)</a>)`)

func TestSelect_ARowReadsAsItsLabelAndFallsBackWithout(t *testing.T) {
	t.Parallel()
	srv := labelServer(t)
	_, body, _ := fetch(t, srv.URL+"/select?from=2026-10-02&to=2026-10-02")
	got := map[string]string{} // id -> the link text
	short := map[string]string{}
	for _, m := range memberRE.FindAllStringSubmatch(body, -1) {
		if m[2] != "" {
			got[m[1]], short[m[1]] = m[2], m[3]
		} else {
			got[m[1]] = m[4]
		}
	}
	for ref, want := range map[string]string{"wt": "80.4 kg", "dose": "melatonin 5 mg"} {
		if got[pid(ref)] != want {
			t.Errorf("%s reads %q, want %q\n%s", ref, got[pid(ref)], want, body)
		}
		if short[pid(ref)] != pid(ref)[:8] {
			t.Errorf("%s keeps no short id beside its label: %q", ref, short[pid(ref)])
		}
	}
	// No label: the type has none, the payload holds nothing the label reads, or no
	// Definition names the type. The row keeps its short id, with its type and time in
	// the columns beside it.
	for _, ref := range []string{"alert", "bare", "odd"} {
		if got[pid(ref)] != pid(ref)[:8] {
			t.Errorf("%s reads %q, want its short id %q", ref, got[pid(ref)], pid(ref)[:8])
		}
	}
	if len(got) != 5 {
		t.Errorf("%d members drawn, want 5", len(got))
	}
	for _, want := range []string{"<h2>health.weight</h2>", "<h2>health.medication.dose</h2>", "2026-10-02T07:00:00.000000000Z"} {
		if !strings.Contains(body, want) {
			t.Errorf("the page lost %q: a labelled row still shows its type and time", want)
		}
	}
}

func TestSelect_TheBasketReadsAsTheLabelToo(t *testing.T) {
	t.Parallel()
	srv := labelServer(t)
	_, body, _ := fetch(t, srv.URL+"/select?tick="+pid("wt")+"&tick="+pid("alert")+"&untick="+pid("dose")+"&from=2026-10-02&to=2026-10-02")
	basket := body[strings.Index(body, `<section class="basket">`):]
	for _, want := range []string{`<strong class="label">80.4 kg</strong> health.weight`, `<strong class="label">melatonin 5 mg</strong> health.medication.dose`} {
		if !strings.Contains(basket, want) {
			t.Errorf("the basket lacks %q:\n%s", want, basket)
		}
	}
	if strings.Count(basket, `class="label"`) != 2 {
		t.Errorf("the alert has no label, so the basket shows its type alone:\n%s", basket)
	}
	if !strings.Contains(basket, "health.heart-rate-alert") {
		t.Error("the unlabelled alert lost its type")
	}
}
