package fold_test

import (
	"encoding/json"
	"testing"

	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/fold"
)

func medEntry(id, typ, factTime, series string) fold.Entry {
	return fold.Entry{
		FacetID: id, Type: facet.Type(typ), FactTime: factTime, Series: series,
		Source: "self-log", SourceRef: facet.SourceRef("self-log", id),
		Status: facet.StatusAdmitted, ContentHash: "sha256:" + id,
		Payload: json.RawMessage(`{}`),
	}
}

func closes(t facet.Type) bool { return t == "health.medication-stopped" }

func ids(v fold.Value) []string {
	var out []string
	for _, e := range v.Entries() {
		out = append(out, e.FacetID)
	}
	return out
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// One regimen: started in March, dose changed in May, stopped in August, restarted
// as a new course in October. The snapshot at each date is the latest per series,
// with a closed series gone and a point fact left alone.
func TestSnapshot_LatestPerSeriesAndClosing(t *testing.T) {
	t.Parallel()
	const (
		course1 = "med.regimen:ramipril:1"
		course2 = "med.regimen:ramipril:2"
	)
	v, err := fold.NewValue([]fold.Entry{
		medEntry("start-1", "health.medication-started", "2026-03-01T00:00:00.000000000Z", course1),
		medEntry("dose-1", "health.medication-dose-changed", "2026-05-10T00:00:00.000000000Z", course1),
		medEntry("stop-1", "health.medication-stopped", "2026-08-01T00:00:00.000000000Z", course1),
		medEntry("start-2", "health.medication-started", "2026-10-01T00:00:00.000000000Z", course2),
		medEntry("weight", "health.weight", "2026-06-01T00:00:00.000000000Z", ""),
	})
	if err != nil {
		t.Fatalf("NewValue: %v", err)
	}

	cases := []struct {
		at   string
		want []string // in (fact_time, facet_id) order
	}{
		{"2026-02-01T00:00:00.000000000Z", nil},
		{"2026-03-01T00:00:00.000000000Z", []string{"start-1"}},
		{"2026-04-01T00:00:00.000000000Z", []string{"start-1"}},
		{"2026-06-15T00:00:00.000000000Z", []string{"dose-1", "weight"}},
		{"2026-09-01T00:00:00.000000000Z", []string{"weight"}},
		{"2026-12-01T00:00:00.000000000Z", []string{"weight", "start-2"}},
	}
	for _, c := range cases {
		t.Run(c.at[:10], func(t *testing.T) {
			t.Parallel()
			s, err := fold.Snapshot(v, c.at, closes)
			if err != nil {
				t.Fatalf("Snapshot: %v", err)
			}
			if got := ids(s); !equal(got, c.want) {
				t.Errorf("snapshot at %s = %v, want %v", c.at[:10], got, c.want)
			}
		})
	}
}

// Without a closing rule nothing closes: the stop is simply the latest fact of its
// series. And the result is a value: two reads at one instant hash the same.
func TestSnapshot_IsAValue(t *testing.T) {
	t.Parallel()
	v, err := fold.NewValue([]fold.Entry{
		medEntry("start-1", "health.medication-started", "2026-03-01T00:00:00.000000000Z", "med.regimen:ramipril:1"),
		medEntry("stop-1", "health.medication-stopped", "2026-08-01T00:00:00.000000000Z", "med.regimen:ramipril:1"),
	})
	if err != nil {
		t.Fatalf("NewValue: %v", err)
	}
	at := "2026-09-01T00:00:00.000000000Z"
	a, _ := fold.Snapshot(v, at, nil)
	b, _ := fold.Snapshot(v, at, nil)
	if !equal(ids(a), []string{"stop-1"}) {
		t.Errorf("snapshot with no closing rule = %v, want the stop itself", ids(a))
	}
	if a.Hash() != b.Hash() {
		t.Error("two snapshots of one value at one instant differ")
	}
}

// An entry without a series serialises exactly as it did before the field existed, so
// a record with no series types keeps every checkpoint hash it had.
func TestEntry_NoSeriesIsAbsentFromTheBytes(t *testing.T) {
	t.Parallel()
	v, err := fold.NewValue([]fold.Entry{medEntry("w", "health.weight", "2026-06-01T00:00:00.000000000Z", "")})
	if err != nil {
		t.Fatalf("NewValue: %v", err)
	}
	if got := string(v.Bytes()); contains(got, "series") {
		t.Errorf("an entry without a series carries the key: %s", got)
	}
	s, _ := fold.NewValue([]fold.Entry{medEntry("m", "health.medication-started", "2026-06-01T00:00:00.000000000Z", "med.regimen:x:1")})
	if got := string(s.Bytes()); !contains(got, `"series":"med.regimen:x:1"`) {
		t.Errorf("a stamped entry does not carry its series: %s", got)
	}
}

func contains(s, sub string) bool {
	return len(sub) <= len(s) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}

// Until keeps the entries at or before its bound, the bound itself included, and an
// empty bound keeps everything.
func TestUntil_CutsAtOrBefore(t *testing.T) {
	t.Parallel()
	v, err := fold.NewValue([]fold.Entry{
		medEntry("start-1", "health.medication-started", "2026-03-01T00:00:00.000000000Z", "med.regimen:ramipril:1"),
		medEntry("stop-1", "health.medication-stopped", "2026-08-01T00:00:00.000000000Z", "med.regimen:ramipril:1"),
	})
	if err != nil {
		t.Fatalf("NewValue: %v", err)
	}
	cases := []struct {
		name, at string
		want     []string
	}{
		{"no bound", "", []string{"start-1", "stop-1"}},
		{"before both", "2026-02-01T00:00:00.000000000Z", nil},
		{"on the start", "2026-03-01T00:00:00.000000000Z", []string{"start-1"}},
		{"between", "2026-06-15T00:00:00.000000000Z", []string{"start-1"}},
		{"after both", "2026-12-01T00:00:00.000000000Z", []string{"start-1", "stop-1"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			u, err := fold.Until(v, c.at)
			if err != nil {
				t.Fatalf("Until: %v", err)
			}
			if got := ids(u); !equal(got, c.want) {
				t.Errorf("Until(%q) = %v, want %v", c.at, got, c.want)
			}
		})
	}
}
