package inspect_test

import (
	"context"
	"fmt"
	"html"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mattjoyce/anam/internal/catalog/catalogtest"
	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/inspect"
	"github.com/mattjoyce/anam/internal/presentation"
	"github.com/mattjoyce/anam/internal/store/ledger"
)

type spatialFixture struct {
	l   *ledger.Ledger
	seq int64
}

func newSpatialFixture(t *testing.T) *spatialFixture {
	t.Helper()
	l, err := ledger.Create(context.Background(), filepath.Join(t.TempDir(), "spatial.db"), person)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	return &spatialFixture{l: l}
}

func (s *spatialFixture) admit(t *testing.T, typ, payload string, at time.Time) string {
	t.Helper()
	f := seedFacet(t, fmt.Sprintf("spatial:%d", s.seq), typ, payload)
	f.FactTime = at
	if err := s.l.Admit(context.Background(), ledger.Watermark(s.seq), f, event(facet.EventAdmitted, f, s.seq+1)); err != nil {
		t.Fatal(err)
	}
	s.seq++
	return f.ID
}

func (s *spatialFixture) handler(t *testing.T, typ string, hue *int, definitions func() *def.Registry) http.Handler {
	t.Helper()
	p, err := presentation.Compile(presentation.Spec{TrackType: facet.Type(typ), Domains: []presentation.DomainSpec{{ID: "custom", Hue: hue}}})
	if err != nil {
		t.Fatal(err)
	}
	return inspect.Handler(inspect.Deps{Ledger: s.l, Presentation: p, Timeout: 30 * time.Second, Buckets: defaultBuckets, Catalog: catalogtest.Engine.Catalog(t), Definitions: definitions})
}

func spatialPage(h http.Handler, path string) (int, string) {
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
	return w.Code, w.Body.String()
}

// Disabled routing must be independent of ledger availability. A nil ledger would
// panic if the route attempted a read, even through the trailing-slash form.
func TestSpatialDisabledDoesNotRead(t *testing.T) {
	t.Parallel()
	h := inspect.Handler(inspect.Deps{})
	for _, method := range []string{"GET", "HEAD"} {
		for _, path := range []string{"/location", "/location/"} {
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest(method, path, nil))
			if w.Code != 404 {
				t.Errorf("%s %s = %d", method, path, w.Code)
			}
		}
	}
}

func TestSpatialSelectionAndIsolation(t *testing.T) {
	t.Parallel()
	s := newSpatialFixture(t)
	hue := 6
	// Keep two server values alive together, with an empty current registry first.
	reg := &def.Registry{}
	enabled := s.handler(t, "custom.route", &hue, func() *def.Registry { return reg })
	disabled := s.handler(t, "", nil, func() *def.Registry { return reg })
	code, body := spatialPage(enabled, "/location")
	if code != 200 || !strings.Contains(body, "no custom.route facets are live") {
		t.Fatalf("empty = %d %s", code, body)
	}
	at := time.Date(2026, 10, 12, 2, 0, 0, 0, time.UTC)
	id := s.admit(t, "custom.route", hourA, at)
	ignored := s.admit(t, "place.track", hourB, at.Add(time.Hour))
	var err error
	reg, err = def.NewRegistry(def.Definition{Type: "custom.route", Kind: def.KindMeasure, Family: "location"})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for _, h := range []http.Handler{enabled, disabled} {
		wg.Add(1)
		go func(h http.Handler) {
			defer wg.Done()
			for _, path := range []string{"/?view=whole", "/facets", "/events", "/definitions", "/definition?type=test.measure", "/projection", "/select", "/facet?id=" + id, "/facet?id=" + ignored} {
				code, body := spatialPage(h, path)
				if code != 200 {
					t.Errorf("%s = %d", path, code)
				}
				if strings.Contains(body, `href="/location"`) != (h == enabled) {
					t.Errorf("nav on %s", path)
				}
				if strings.Contains(body, `class="track"`) != (h == enabled && path == "/facet?id="+id) {
					t.Errorf("facet selection on %s", path)
				}
				if path == "/facet?id="+id && (!strings.Contains(body, "tokay") || !strings.Contains(body, "151.2093")) {
					t.Error("JSON fallback lost")
				}
			}
		}(h)
	}
	wg.Wait()
	code, body = spatialPage(enabled, "/location")
	if code != 200 || !strings.Contains(body, "3 fixes over 1 hour") || strings.Contains(body, "4 fixes") || strings.Contains(body, "place.track") {
		t.Fatal("aggregate selector ignored or stale")
	}
	if !strings.Contains(body, `class="fix h2"`) {
		t.Error("aggregate lost UTC hour hue")
	}
	_, body = spatialPage(enabled, "/facet?id="+id)
	if !strings.Contains(body, `class="fix h6"`) {
		t.Error("facet lost selected domain hue")
	}
	neutral := s.handler(t, "custom.route", nil, func() *def.Registry { return reg })
	_, body = spatialPage(neutral, "/facet?id="+id)
	if !strings.Contains(body, `class="fix hx"`) {
		t.Error("unconfigured track hue not neutral")
	}
}

func TestSpatialOmissionAndHourBuckets(t *testing.T) {
	t.Parallel()
	s := newSpatialFixture(t)
	at := time.Date(2026, 10, 12, 2, 0, 0, 0, time.UTC)
	s.admit(t, "custom.route", hourA, at)
	bad := s.admit(t, "custom.route", strings.Replace(hourA, `-33.8688`, `91`, 1), at)
	s.admit(t, "custom.route", strings.Replace(hourA, `14`, `1.7976931348623157e308`, 1), at)
	s.admit(t, "custom.route", `{"device":"private-invalid-device","points":[]}`, at)
	// A malformed facet outside the requested bucket must not inflate omissions.
	s.admit(t, "custom.route", `{}`, at.Add(-time.Hour))
	h := s.handler(t, "custom.route", nil, func() *def.Registry { return &def.Registry{} })
	_, body := spatialPage(h, "/location?from=2026-10-12T02:00:00Z&to=2026-10-12T02:01:00Z")
	for _, want := range []string{"3 fixes over 1 hour", "3 undrawable custom.route facets omitted", "02:50Z"} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %s", want)
		}
	}
	if strings.Contains(body, "private-invalid-device") || strings.Contains(body, "151.2093") {
		t.Fatal("aggregate diagnostics leaked payload")
	}
	_, body = spatialPage(h, "/facet?id="+bad)
	if strings.Contains(body, `class="track"`) || !strings.Contains(body, "151.2093") {
		t.Fatal("malformed facet lost JSON fallback")
	}
	_, body = spatialPage(h, "/location?from=2026-10-12T02:01:00Z&to=2026-10-12T03:00:00Z")
	if strings.Contains(body, `class="track"`) || strings.Contains(body, "facets omitted") {
		t.Fatal("partial hour selected by point time")
	}
}

func TestSpatialDeviceNavigationAndLocalAssets(t *testing.T) {
	t.Parallel()
	s := newSpatialFixture(t)
	at := time.Date(2026, 10, 12, 2, 0, 0, 0, time.UTC)
	device := "phone & + # 東京"
	payload := strings.ReplaceAll(hourA, "tokay", device)
	id := s.admit(t, "custom.route", payload, at)
	h := s.handler(t, "custom.route", nil, func() *def.Registry { return &def.Registry{} })
	_, body := spatialPage(h, "/location?"+url.Values{"device": {device}}.Encode())
	if !strings.Contains(body, "3 fixes over 1 hour") {
		t.Fatal("device filter failed")
	}
	links := regexp.MustCompile(`href="([^"]+)" title="(earlier|wider|narrower|later)"`).FindAllStringSubmatch(body, -1)
	if len(links) != 4 {
		t.Fatalf("navigation links = %d", len(links))
	}
	for _, link := range links {
		u, err := url.Parse(html.UnescapeString(link[1]))
		if err != nil {
			t.Fatal(err)
		}
		if u.Query().Get("device") != device {
			t.Errorf("device changed through %s", link[2])
		}
		code, page := spatialPage(h, u.String())
		if code != 200 {
			t.Fatal(code)
		}
		if strings.Contains(page, "fixes over") && !strings.Contains(page, "3 fixes over 1 hour") {
			t.Fatal("device changed through rendered navigation")
		}
	}
	_, detail := spatialPage(h, "/facet?id="+id)
	external := regexp.MustCompile(`(?i)(?:href|src|action)\s*=\s*["'](?:https?:)?//|url\(\s*["']?(?:https?:)?//|fetch\(`)
	for _, page := range []string{body, detail} {
		if external.MatchString(page) {
			t.Fatal("outbound asset or request")
		}
	}
}

// A deployment-sized bounded fixture verifies complete drawing, including the last
// admitted hour. It does not impose a new truncation limit or resource budget.
func TestSpatialDeploymentScale(t *testing.T) {
	t.Parallel()
	s := newSpatialFixture(t)
	at := time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 240; i++ {
		hour := at.Add(time.Duration(i%24) * time.Hour)
		payload := fmt.Sprintf(`{"device":"phone-%d","points":[`, i/24)
		for j := 0; j < 60; j++ {
			if j > 0 {
				payload += ","
			}
			payload += fmt.Sprintf(`{"t":%q,"lat":-33.8,"lon":151.2,"acc":10}`, hour.Add(time.Duration(j)*time.Minute).Format(time.RFC3339))
		}
		payload += `]}`
		s.admit(t, "custom.route", payload, hour)
	}
	h := s.handler(t, "custom.route", nil, func() *def.Registry { return &def.Registry{} })
	_, body := spatialPage(h, "/location")
	if !strings.Contains(body, "14400 fixes over 240 hours") || strings.Count(body, `class="line"`) != 10 || strings.Count(body, `class="ring"`) != 14400 {
		t.Fatal("deployment drawing truncated")
	}
}
