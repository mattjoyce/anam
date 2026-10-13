package inspect_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/inspect"
	"github.com/mattjoyce/anam/internal/store/ledger"
)

// supersededServer holds one blood pressure facet admitted at v1 and corrected at v2,
// both under the bundled Definition, and returns the inspector and the facet's id.
func supersededServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	ctx := context.Background()
	l, err := ledger.Create(ctx, filepath.Join(t.TempDir(), "ledger.db"), person)
	if err != nil {
		t.Fatalf("ledger.Create: %v", err)
	}
	t.Cleanup(func() { _ = l.Close() })
	cur, err := def.Parse(bundledBody(t, "health.blood-pressure"))
	if err != nil {
		t.Fatal(err)
	}

	const ref = "bp-csv:2026-1-24 06:30:00:1"
	v1 := seedFacet(t, ref, "health.blood-pressure",
		`{"date_excel": "2026-1-24 06:30:00", "reading": "1", "systolic_blood_pressure": {"value": 128, "unit": "mmHg"}, "diastolic_blood_pressure": {"value": 79, "unit": "mmHg"}, "effective_time_frame": {"date_time": "2026-01-24T06:30:00+11:00"}}`)
	v1.DefinitionVersion = cur.Version
	if err := l.Admit(ctx, 0, v1, event(facet.EventAdmitted, v1, 1)); err != nil {
		t.Fatalf("Admit v1: %v", err)
	}
	v2 := seedFacet(t, ref, "health.blood-pressure",
		`{"date_excel": "2026-1-24 06:30:00", "reading": "1", "systolic_blood_pressure": {"value": 130, "unit": "mmHg"}, "diastolic_blood_pressure": {"value": 79, "unit": "mmHg"}, "effective_time_frame": {"date_time": "2026-01-24T06:30:00+11:00"}, "comment": "redone"}`)
	v2.DefinitionVersion, v2.ArtifactVersion = cur.Version, 2
	sup := facet.AnamEvent{Kind: facet.EventSuperseded, FacetID: v2.ID, Seq: 2, ArtifactVersion: 2,
		PriorArtifactVersion: 1, AnamTime: time.Date(2026, 7, 11, 9, 0, 0, 0, time.UTC)}
	if err := l.Admit(ctx, 1, v2, sup); err != nil {
		t.Fatalf("Admit v2: %v", err)
	}
	srv := httptest.NewServer(inspect.Handler(inspect.Deps{Ledger: l, Timeout: 30 * time.Second, Buckets: defaultBuckets}))
	t.Cleanup(srv.Close)
	return srv, v1.ID
}

// The newest version says what it changed from the one before, and reads its payload
// through the Definition it was admitted under, field by field.
func TestFacet_ShowsWhatAVersionChangedAndWhatEachFieldMeans(t *testing.T) {
	t.Parallel()
	srv, id := supersededServer(t)
	code, body := get(t, srv.URL+"/facet?id="+id)
	if code != http.StatusOK {
		t.Fatalf("GET /facet = %d", code)
	}
	for _, want := range []string{
		"changed from v1",
		`<td class="mono">/systolic_blood_pressure/value</td><td class="mono">128</td><td class="mono">130</td>`,
		`<td class="mono">/comment</td><td class="mono"><span class="dim">absent</span></td>`,
		"read through definition v",
		"Systolic pressure as an OMH unit-value", // the field's own words
		`<span class="role">fact time</span>`,
		`href="/facet?id=` + id + `&v=1"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("facet page lacks %q", want)
		}
	}
	if strings.Contains(body, "/diastolic_blood_pressure/value</td>") {
		t.Error("an unchanged field is listed as changed")
	}
}

// ?v= opens an earlier version as it was; the first has nothing before it.
func TestFacet_OpensAnEarlierVersion(t *testing.T) {
	t.Parallel()
	srv, id := supersededServer(t)
	code, body := get(t, srv.URL+"/facet?id="+id+"&v=1")
	if code != http.StatusOK {
		t.Fatalf("GET v=1 = %d", code)
	}
	if !strings.Contains(body, `<span class="j-num">128</span>`) || strings.Contains(body, `<span class="j-num">130</span>`) {
		t.Error("v1 does not show v1's payload")
	}
	if strings.Contains(body, "changed from") {
		t.Error("v1 claims a version before it")
	}
	if !strings.Contains(body, "(newest)") {
		t.Error("v1 does not point at the newest version")
	}
	for _, q := range []string{"0", "3", "x"} {
		if code, _ := get(t, srv.URL+"/facet?id="+id+"&v="+q); code != http.StatusBadRequest {
			t.Errorf("?v=%s = %d, want 400", q, code)
		}
	}
}
