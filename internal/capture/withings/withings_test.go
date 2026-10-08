package withings_test

import (
	"bytes"
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/mattjoyce/anam/internal/facet"

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
// must not reach the payload. Their groups are as the account holds them: the
// blood pressure entered by hand (attrib 2, no device, no timezone), the weight
// from the scale but ambiguous (attrib 1), and a scale reading with no group row.
func fixture(t *testing.T) string {
	t.Helper()
	path := measurementsOnly(t)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer func() { _ = db.Close() }()

	stmts := `
	CREATE TABLE measure_groups (
	    withings_id INTEGER PRIMARY KEY,
	    attrib INTEGER, category INTEGER, deviceid TEXT, model TEXT, modelid INTEGER,
	    timezone TEXT, created INTEGER, modified INTEGER);

	INSERT INTO measure_groups VALUES
	  (6314440651, 2, 1, NULL, NULL, NULL, NULL, 1724355780, 1724355780),
	  (6314440652, 2, 1, NULL, NULL, NULL, NULL, 1724355780, 1724355780),
	  (5204716098, 1, 1, 'da158fc939db68c8', 'Body Smart', 16, 'Australia/Sydney', 1706304979, 1788579025);
	INSERT INTO measurements (withings_id, date, type, value, unit, created_at) VALUES
	  (7974153600,'2026-07-11 13:47:37', 1, 88.164, -3, '2026-07-11 19:33:04');`
	if _, err := db.Exec(stmts); err != nil {
		t.Fatalf("seed groups: %v", err)
	}
	return path
}

// measurementsOnly is a source from before the plugin stored measure groups.
func measurementsOnly(t *testing.T) string {
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
	if len(envs) != 6 {
		t.Fatalf("Scan composed %d envelopes, want 6 (one per measurement)", len(envs))
	}

	for _, env := range envs {
		if env.Type != withings.Type {
			t.Errorf("type %s, want %s", env.Type, withings.Type)
		}
		if err := env.Validate(); err != nil {
			t.Errorf("%s: %v", env.Type, err)
		}
		d, ok := reg.Lookup(env.Type)
		if !ok {
			t.Fatalf("%s: no bundled definition for %s", env.SourceRef, env.Type)
		}
		doc, err := def.Decode(env.Payload)
		if err != nil {
			t.Fatalf("%s: decode: %v", env.Type, err)
		}
		if err := d.Check(doc); err != nil {
			t.Errorf("%s: not compliant: %v", env.Type, err)
		}
		if _, err := d.FactTimeOf(doc); err != nil {
			t.Errorf("%s: fact time: %v", env.Type, err)
		}
		if env.SourceRef != "" {
			t.Errorf("%s: the worker composed source_ref %q; composing it is the gate's job", env.Type, env.SourceRef)
		}
		if _, err := d.SourceRef(env.Source, doc); err != nil {
			t.Errorf("%s: compose ref: %v", env.Type, err)
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
	envs = identified(t, envs)

	refs := map[string]bool{}
	for _, env := range envs {
		if refs[env.SourceRef] {
			t.Errorf("duplicate source_ref %q — two measurements collapsed into one facet", env.SourceRef)
		}
		refs[env.SourceRef] = true
	}
	for _, want := range []string{
		facet.SourceRef("withings", "6314440651", "9"), facet.SourceRef("withings", "6314440651", "10"),
		facet.SourceRef("withings", "6314440652", "9"), facet.SourceRef("withings", "6314440652", "10"),
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
	envs = identified(t, envs)
	var weight []byte
	for _, env := range envs {
		if env.SourceRef == facet.SourceRef("withings", "5204716098", "1") {
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
	envs = identified(t, envs)
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
	a = identified(t, a)
	b, err := withings.Scan(context.Background(), path, "matt")
	if err != nil {
		t.Fatalf("re-Scan: %v", err)
	}
	b = identified(t, b)
	if len(a) != len(b) {
		t.Fatalf("scans disagree on count: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i].SourceRef != b[i].SourceRef || !bytes.Equal(a[i].Payload, b[i].Payload) {
			t.Errorf("scan %d not deterministic: %s", i, a[i].SourceRef)
		}
	}
}

// Withings' own attribution decides the binding. A reading the scale could not place
// asserts no one, so the gate binds it at medium. A reading typed in by hand is the
// account holder's own entry and keeps the assertion: manual is how it was taken,
// not whose it is. A row with no group row is as it was before attribution existed.
func TestScan_AttributionDecidesTheAssertion(t *testing.T) {
	t.Parallel()

	envs, err := withings.Scan(context.Background(), fixture(t), "matt")
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	envs = identified(t, envs)
	want := map[string]string{
		facet.SourceRef("withings", "5204716098", "1"):  "",     // attrib 1, ambiguous
		facet.SourceRef("withings", "6314440651", "9"):  "matt", // attrib 2, entered by hand
		facet.SourceRef("withings", "6314440652", "10"): "matt",
		facet.SourceRef("withings", "7974153600", "1"):  "matt", // no group row
	}
	for _, env := range envs {
		if w, ok := want[env.SourceRef]; ok && env.AssertedPerson != w {
			t.Errorf("%s: asserted %q, want %q", env.SourceRef, env.AssertedPerson, w)
		}
	}
}

// The group's fields reach the payload as the source wrote them, and a NULL is an
// absent field: the hand-entered reading has no device and no timezone, and must not
// be given an empty or invented one.
func TestScan_CarriesTheGroupAsWritten(t *testing.T) {
	t.Parallel()

	envs, err := withings.Scan(context.Background(), fixture(t), "matt")
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	envs = identified(t, envs)
	byRef := map[string][]byte{}
	for _, env := range envs {
		byRef[env.SourceRef] = env.Payload
	}
	scale := byRef[facet.SourceRef("withings", "5204716098", "1")]
	for _, want := range []string{`"attrib":"1"`, `"model":"Body Smart"`, `"deviceid":"da158fc939db68c8"`,
		`"timezone":"Australia/Sydney"`} {
		if !bytes.Contains(scale, []byte(want)) {
			t.Errorf("scale payload lacks %s:\n%s", want, scale)
		}
	}
	manual := byRef[facet.SourceRef("withings", "6314440651", "9")]
	if !bytes.Contains(manual, []byte(`"attrib":"2"`)) {
		t.Errorf("manual payload lacks its attrib:\n%s", manual)
	}
	for _, absent := range []string{"model", "deviceid", "timezone", "null"} {
		if bytes.Contains(manual, []byte(absent)) {
			t.Errorf("manual payload carries %s:\n%s", absent, manual)
		}
	}
	for _, absent := range []string{"modified", "1788579025", "category"} {
		if bytes.Contains(scale, []byte(absent)) {
			t.Errorf("scale payload carries Withings' bookkeeping %s:\n%s", absent, scale)
		}
	}
}

// A source without measure_groups cannot say who stood on the scale. Scan refuses
// rather than bind every reading high on attribution it cannot see.
func TestScan_RefusesASourceWithoutGroups(t *testing.T) {
	t.Parallel()

	if _, err := withings.Scan(context.Background(), measurementsOnly(t), "matt"); err == nil {
		t.Fatal("Scan accepted a source with no measure_groups")
	}
}

// identified gives each envelope the source_ref the gate admits it under. The
// worker leaves it empty; the Definition's identity_key composes it.
func identified(t *testing.T, envs []facet.Envelope) []facet.Envelope {
	t.Helper()
	reg, err := def.Bundled()
	if err != nil {
		t.Fatalf("Bundled: %v", err)
	}
	out := make([]facet.Envelope, len(envs))
	for i, env := range envs {
		ref, err := reg.Identify(env)
		if err != nil {
			t.Fatalf("envelope %d (%s): %v", i, env.Type, err)
		}
		env.SourceRef = ref
		out[i] = env
	}
	return out
}
