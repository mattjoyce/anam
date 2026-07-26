package withings_test

import (
	"bytes"
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/mattjoyce/anam/internal/capture/withings"
	"github.com/mattjoyce/anam/internal/def"
)

// fixture builds a real source database in the test's temp dir — the source's own
// schema, including the UNIQUE(withings_id, type) constraint that is the identity.
//
// The rows are drawn from the shape the real export has: two measurement groups
// sharing one timestamp (a repeated blood-pressure reading), a REAL that would
// lose its ".0" if the driver were allowed to re-utter it, and a created_at that
// must not reach the payload.
func fixture(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "withings.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer func() { _ = db.Close() }()

	stmts := `
	CREATE TABLE measurements (
	    id INTEGER PRIMARY KEY AUTOINCREMENT,
	    withings_id INTEGER,
	    date TIMESTAMP NOT NULL,
	    type INTEGER NOT NULL,
	    value REAL NOT NULL,
	    unit INTEGER,
	    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	    UNIQUE(withings_id, type));

	INSERT INTO measurements (withings_id, date, type, value, unit, created_at) VALUES
	  (6314440651,'2024-08-22 19:43:00', 9,  93.0, -2, '2025-02-24 18:40:12'),
	  (6314440651,'2024-08-22 19:43:00', 10,151.0, -2, '2025-02-24 18:40:12'),
	  (6314440652,'2024-08-22 19:43:00', 9,  90.0, -2, '2025-02-24 18:40:12'),
	  (6314440652,'2024-08-22 19:43:00', 10,137.0, -2, '2025-02-24 18:40:12'),
	  (5204716098,'2024-01-26 21:36:19', 1,  99.0, -3, '2025-02-24 18:40:12');`
	if _, err := db.Exec(stmts); err != nil {
		t.Fatalf("seed fixture: %v", err)
	}
	return path
}

// Every envelope the worker composes must clear the same bundled contract the gate
// will hold it to: definition present, payload compliant, fact time parseable, and
// — the strong one — the worker's source_ref byte-identical to the one the
// definition composes from the payload. Identity is definition-owned (§6a).
func TestScan_ComposesGateCompliantEnvelopes(t *testing.T) {
	t.Parallel()
	reg, err := def.Bundled()
	if err != nil {
		t.Fatalf("Bundled: %v", err)
	}

	envs, err := withings.Scan(context.Background(), fixture(t), "matt")
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(envs) != 5 {
		t.Fatalf("Scan composed %d envelopes, want 5 (one per measurement)", len(envs))
	}

	for _, env := range envs {
		if env.Type != withings.Type {
			t.Errorf("%s: type %s, want %s", env.SourceRef, env.Type, withings.Type)
		}
		if err := env.Validate(); err != nil {
			t.Errorf("%s: %v", env.SourceRef, err)
		}
		d, ok := reg.Lookup(env.Type)
		if !ok {
			t.Fatalf("%s: no bundled definition for %s", env.SourceRef, env.Type)
		}
		doc, err := def.Decode(env.Payload)
		if err != nil {
			t.Fatalf("%s: decode: %v", env.SourceRef, err)
		}
		if err := d.Check(doc); err != nil {
			t.Errorf("%s: not compliant: %v", env.SourceRef, err)
		}
		if _, err := d.FactTimeOf(doc); err != nil {
			t.Errorf("%s: fact time: %v", env.SourceRef, err)
		}
		ref, err := d.SourceRef(env.Source, doc)
		if err != nil {
			t.Errorf("%s: compose ref: %v", env.SourceRef, err)
		} else if ref != env.SourceRef {
			t.Errorf("identity disagreement: worker says %q, definition composes %q", env.SourceRef, ref)
		}
	}
}

// The two measurement groups at 19:43:00 are different facts, and only
// withings_id separates them. Keying on the timestamp — or on the timestamp and
// the metric — would collapse the pair into one facet and silently supersede the
// first reading with the second.
func TestScan_SameInstantDifferentGroupsAreDistinctFacets(t *testing.T) {
	t.Parallel()

	envs, err := withings.Scan(context.Background(), fixture(t), "matt")
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	refs := map[string]bool{}
	for _, env := range envs {
		if refs[env.SourceRef] {
			t.Errorf("duplicate source_ref %q — two measurements collapsed into one facet", env.SourceRef)
		}
		refs[env.SourceRef] = true
	}
	for _, want := range []string{
		"withings:6314440651:9", "withings:6314440651:10",
		"withings:6314440652:9", "withings:6314440652:10",
	} {
		if !refs[want] {
			t.Errorf("missing source_ref %q", want)
		}
	}
}

// The source's own lexeme must reach the payload. A REAL 99.0 read through the
// driver as a float64 marshals to "99", and a TIMESTAMP comes back re-formatted —
// either would put a value in the ledger that the source never wrote.
func TestScan_KeepsTheSourceLexeme(t *testing.T) {
	t.Parallel()

	envs, err := withings.Scan(context.Background(), fixture(t), "matt")
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	var weight []byte
	for _, env := range envs {
		if env.SourceRef == "withings:5204716098:1" {
			weight = env.Payload
		}
	}
	if weight == nil {
		t.Fatal("the weight measurement is missing")
	}
	for _, want := range []string{`"value":"99.0"`, `"date":"2024-01-26 21:36:19"`} {
		if !bytes.Contains(weight, []byte(want)) {
			t.Errorf("payload lost the source lexeme %s:\n%s", want, weight)
		}
	}
}

// created_at is the sync pipeline's clock and id is this copy's surrogate key.
// Neither is a fact about the person, and carrying created_at in a hashed payload
// would turn every re-sync into a spurious supersession.
func TestScan_ExcludesThePipelinesOwnColumns(t *testing.T) {
	t.Parallel()

	envs, err := withings.Scan(context.Background(), fixture(t), "matt")
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	for _, env := range envs {
		for _, forbidden := range []string{"created_at", `"id"`, "2025-02-24"} {
			if bytes.Contains(env.Payload, []byte(forbidden)) {
				t.Errorf("%s: payload carries %s:\n%s", env.SourceRef, forbidden, env.Payload)
			}
		}
	}
}

// Two scans of an unchanged source must compose byte-identical payloads — the
// worker's half of the idempotency promise (the gate's half hashes them).
func TestScan_IsDeterministic(t *testing.T) {
	t.Parallel()
	path := fixture(t)

	a, err := withings.Scan(context.Background(), path, "matt")
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	b, err := withings.Scan(context.Background(), path, "matt")
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
