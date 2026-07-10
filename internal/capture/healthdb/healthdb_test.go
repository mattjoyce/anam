package healthdb_test

import (
	"bytes"
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/mattjoyce/anam/internal/capture/healthdb"
	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
)

// fixture builds a real source database in the test's temp dir — the source's own
// schema, a handful of rows: two sleep days, one body month, one heart day whose
// bp_* rows must vanish, one stress day for the generic type, and a NULL min/max.
func fixture(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "healthdata.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer func() { _ = db.Close() }()

	stmts := `
	CREATE TABLE metric_groups (group_id TEXT PRIMARY KEY, name TEXT NOT NULL);
	CREATE TABLE metrics (metric_id TEXT PRIMARY KEY, group_id TEXT NOT NULL, name TEXT NOT NULL, unit TEXT NOT NULL);
	CREATE TABLE summaries (
	  period_type TEXT NOT NULL, period_start DATE NOT NULL, period_end DATE NOT NULL,
	  metric_id TEXT NOT NULL, avg_value REAL, min_value REAL, max_value REAL,
	  sample_count INTEGER, last_updated TIMESTAMP,
	  PRIMARY KEY (period_type, period_start, metric_id));

	INSERT INTO metric_groups VALUES ('sleep','Sleep'),('body','Body Composition'),('heart','Heart Rate'),('stress','Stress');
	INSERT INTO metrics VALUES
	  ('sleep_total','sleep','Total Sleep','min'),
	  ('sleep_rem','sleep','REM Sleep','min'),
	  ('weight','body','Weight','kg'),
	  ('rhr','heart','Resting Heart Rate','bpm'),
	  ('bp_systolic','heart','Blood Pressure (systolic)','mmHg'),
	  ('stress','stress','Stress (avg)','score');

	INSERT INTO summaries VALUES
	  ('day','2026-03-07T00:00:00','2026-03-07T23:59:59.999999','sleep_total',408.0,NULL,NULL,1,'2026-03-22 11:07:22'),
	  ('day','2026-03-07T00:00:00','2026-03-07T23:59:59.999999','sleep_rem',123.0,NULL,NULL,1,'2026-03-22 11:07:22'),
	  ('day','2026-03-08T00:00:00','2026-03-08T23:59:59.999999','sleep_total',418.0,NULL,NULL,1,'2026-03-22 11:07:22'),
	  ('month','2026-03-01T00:00:00','2026-03-31T23:59:59.999999','weight',86.4,85.9,87.1,4,'2026-04-01 09:00:00'),
	  ('day','2026-03-07T00:00:00','2026-03-07T23:59:59.999999','rhr',54.0,51.0,58.0,96,'2026-03-22 11:07:22'),
	  ('day','2026-03-07T00:00:00','2026-03-07T23:59:59.999999','bp_systolic',118.0,110.0,126.0,3,'2026-03-22 11:07:22'),
	  ('day','2026-03-07T00:00:00','2026-03-07T23:59:59.999999','stress',31.0,12.0,74.0,88,'2026-03-22 11:07:22');`
	if _, err := db.Exec(stmts); err != nil {
		t.Fatalf("seed fixture: %v", err)
	}
	return path
}

// Every envelope the worker composes must clear the same bundled contract the gate
// will hold it to: definition present, payload compliant, times parseable, and —
// the strong one — the worker's source_ref byte-identical to the one the
// definition composes from the payload. Identity is definition-owned (§6a); this
// test is the worker agreeing in advance.
func TestScan_ComposesGateCompliantEnvelopes(t *testing.T) {
	t.Parallel()
	reg, err := def.Bundled()
	if err != nil {
		t.Fatalf("Bundled: %v", err)
	}

	envs, err := healthdb.Scan(context.Background(), fixture(t), "matt")
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	// 5 group-periods: sleep×2 days, body×1 month, heart×1 day, stress×1 day.
	if len(envs) != 5 {
		t.Fatalf("Scan composed %d envelopes, want 5", len(envs))
	}

	types := map[facet.Type]int{}
	for _, env := range envs {
		types[env.Type]++

		if err := env.Validate(); err != nil {
			t.Errorf("%s: %v", env.SourceRef, err)
		}
		d, ok := reg.Lookup(env.Type)
		if !ok {
			t.Errorf("%s: no bundled definition for %s", env.SourceRef, env.Type)
			continue
		}
		doc, err := def.Decode(env.Payload)
		if err != nil {
			t.Fatalf("%s: decode: %v", env.SourceRef, err)
		}
		if err := d.Check(doc); err != nil {
			t.Errorf("%s: not compliant: %v", env.SourceRef, err)
		}
		if _, _, err := d.Times(doc); err != nil {
			t.Errorf("%s: times: %v", env.SourceRef, err)
		}
		ref, err := d.SourceRef(env.Source, doc)
		if err != nil {
			t.Errorf("%s: compose ref: %v", env.SourceRef, err)
		} else if ref != env.SourceRef {
			t.Errorf("identity disagreement: worker says %q, definition composes %q", env.SourceRef, ref)
		}
	}

	want := map[facet.Type]int{
		"health.sleep": 2, "health.body-composition": 1,
		"health.heart-rate": 1, "health.metric-group": 1,
	}
	for typ, n := range want {
		if types[typ] != n {
			t.Errorf("%s: %d envelopes, want %d", typ, types[typ], n)
		}
	}

	// The bp_* aggregates must not have ridden along inside health.heart-rate.
	for _, env := range envs {
		if env.Type == "health.heart-rate" && bytes.Contains(env.Payload, []byte("bp_systolic")) {
			t.Error("bp_* aggregate leaked into health.heart-rate (MVP.md §5 excludes it)")
		}
	}
}

// Two scans of an unchanged source must compose byte-identical payloads — the
// worker's half of the idempotency promise (the gate's half hashes them).
func TestScan_IsDeterministic(t *testing.T) {
	t.Parallel()
	path := fixture(t)

	a, err := healthdb.Scan(context.Background(), path, "matt")
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	b, err := healthdb.Scan(context.Background(), path, "matt")
	if err != nil {
		t.Fatalf("re-Scan: %v", err)
	}
	if len(a) != len(b) {
		t.Fatalf("scans disagree on count: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i].SourceRef != b[i].SourceRef || !bytes.Equal(a[i].Payload, b[i].Payload) {
			t.Errorf("scan %d not deterministic: %s", i, a[i].SourceRef)
		}
	}
}
