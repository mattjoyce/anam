package app

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/config"
	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/fold"
	"github.com/mattjoyce/anam/internal/gate"
	"github.com/mattjoyce/anam/internal/migrate"
	"github.com/mattjoyce/anam/internal/span"
	"github.com/mattjoyce/anam/internal/store/ledger"
	"github.com/mattjoyce/anam/internal/store/ops"
)

// Card 200: a migration carries the live facets of one ledger into a fresh one through
// the gate. The old files are only read, the new ones must not exist, the dry run
// writes nothing, and the new ledger holds the selection facet for facet, each facet
// under the Definition version it was admitted under.

// readingV1 and readingV2 are one registered type at two versions: v2 adds an optional
// note, and keys and places facets as v1 does.
const readingV1 = `{
  "meta": {
    "facet_type": "test.reading", "definition_version": 1, "classification": "Normal",
    "validation_mode": "strict", "source_schema": "test/reading",
    "time_semantics": {"fact_time": {"pointer": "/measured_at", "layout": "2006-01-02T15:04:05Z07:00", "zone": "UTC"}},
    "identity_key": ["/measured_at"]
  },
  "schema": {
    "$schema": "https://json-schema.org/draft/2020-12/schema",
    "type": "object", "required": ["measured_at", "level"],
    "properties": {"measured_at": {"type": "string", "minLength": 1}, "level": {"type": "number"}},
    "additionalProperties": false
  }
}`

var readingV2 = strings.NewReplacer(`"definition_version": 1`, `"definition_version": 2`,
	`"level": {"type": "number"}}`, `"level": {"type": "number"}, "note": {"type": "string"}}`).Replace(readingV1)

// heartTemplate is a registered projection, so the carry has one to hold.
const heartTemplate = `{"name":"heart","version":1,"description":"heart","types":["health.heart-rate-day"],` +
	`"statuses":["admitted","endorsed"],"max_classification":"Sensitive","from":"","to":"","shape":"log"}`

func readingPayload(at string, level int, note string) string {
	if note == "" {
		return fmt.Sprintf(`{"measured_at": %q, "level": %d}`, at, level)
	}
	return fmt.Sprintf(`{"measured_at": %q, "level": %d, "note": %q}`, at, level, note)
}

// registryAt is the bundled Definitions with test.reading registered at the versions
// given, as the daemon would load them.
func registryAt(t *testing.T, bodies ...string) *def.Registry {
	t.Helper()
	bundled, err := def.Bundled()
	if err != nil {
		t.Fatal(err)
	}
	var stored []def.Stored
	for i, b := range bodies {
		stored = append(stored, def.Stored{Type: "test.reading", Version: i + 1, Body: []byte(b)})
	}
	reg, err := def.WithRegistered(bundled, stored)
	if err != nil {
		t.Fatal(err)
	}
	return reg
}

// admitAs commits recs from source through the gate under defs, in one batch.
func admitAs(t *testing.T, path, source string, defs *def.Registry, recs []record) {
	t.Helper()
	ctx := context.Background()
	l, err := ledger.Open(ctx, path, "matt")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()
	types := make([]facet.Type, len(recs))
	envs := make([]facet.Envelope, len(recs))
	bindings := make([]facet.Binding, len(recs))
	for i, r := range recs {
		types[i] = r.typ
		envs[i] = facet.Envelope{Type: r.typ, Source: source, Injector: source, Payload: []byte(r.payload), AssertedPerson: "matt"}
	}
	g := gate.New(l, defs, gate.StaticScopes{source: types}, clock.Real{}, "matt",
		gate.Limits{MaxPayloadBytes: config.DefaultMaxPayloadBytes, MaxRetries: config.DefaultCommitRetries})
	for i, env := range envs {
		if bindings[i], err = g.Resolve(env); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := g.CommitBatch(ctx, envs, bindings); err != nil {
		t.Fatalf("admit %d records from %s: %v", len(recs), source, err)
	}
}

// tombstone retracts the facet whose source ref the source and ref name.
func tombstone(t *testing.T, path string, id string) {
	t.Helper()
	ctx := context.Background()
	l, err := ledger.Open(ctx, path, "matt")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()
	reg, err := def.Bundled()
	if err != nil {
		t.Fatal(err)
	}
	g := gate.New(l, reg, gate.StaticScopes{}, clock.Real{}, "matt",
		gate.Limits{MaxPayloadBytes: config.DefaultMaxPayloadBytes, MaxRetries: config.DefaultCommitRetries})
	if _, _, err := g.Retract(ctx, id); err != nil {
		t.Fatal(err)
	}
}

// registerInStore registers the test type's versions and the heart template in the
// fixture's operational store, as the importers would have.
func registerInStore(t *testing.T, f ledgerFixture, bodies ...string) {
	t.Helper()
	ctx := context.Background()
	buckets, err := config.Config{BucketWidth: span.Default}.Buckets()
	if err != nil {
		t.Fatal(err)
	}
	db, err := ops.Open(ctx, filepath.Join(f.dir, "tracking.db"), buckets)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	for i, b := range bodies {
		row := ops.DefinitionRow{Type: "test.reading", Version: i + 1, Body: b}
		if err := db.RegisterDefinition(ctx, row, fmt.Sprintf("2026-10-2%d%s", i+1, "T00:00:00.000000000Z")); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.RegisterProjection(ctx, ops.ProjectionRow{Name: "heart", Version: 1, Body: heartTemplate}, "2026-10-23T00:00:00.000000000Z"); err != nil {
		t.Fatal(err)
	}
}

// migrationFixture is an old ledger with churn in it, and the ids of what it holds.
type migrationFixture struct {
	ledgerFixture
	store                     string
	dead, kept, changed       string // a tombstoned reading, one left alone, one corrected
	atV1, movedToV2, atV2     string // test.reading facets: left at v1, v1 then v2, v2
	edge                      string
	newLedger, newStore       string
	oldLedgerSum, oldStoreSum [32]byte
}

func idOf(source string, values ...string) string {
	return facet.NewID(facet.SourceRef(source, values...))
}

func newMigrationFixture(t *testing.T) migrationFixture {
	t.Helper()
	f := migrationFixture{ledgerFixture: newLedgerFixture(t)}
	f.store = filepath.Join(f.dir, "tracking.db")
	f.newLedger, f.newStore = filepath.Join(f.dir, "new", "ledger.db"), filepath.Join(f.dir, "new", "tracking.db")

	// Two sources of blood pressure; one reading of "records" is corrected, so it has
	// two versions, and one is retracted.
	f.admit(t, bpReadings)
	f.admit(t, corrected(bpReadings, "114.0", "115.0")[:1])
	bundled, err := def.Bundled()
	if err != nil {
		t.Fatal(err)
	}
	admitAs(t, f.ledgerPath, "bp-csv", bundled, []record{
		{"health.blood-pressure", `{"id": "csv:1", "date_excel": "2026-3-1 07:00:00", "reading": "1", "systolic_blood_pressure": {"value": 120.0, "unit": "mmHg"}, "diastolic_blood_pressure": {"value": 80.0, "unit": "mmHg"}, "effective_time_frame": {"date_time": "2026-03-01T07:00:00+11:00"}, "pulse": "60.0"}`},
	})
	f.changed, f.dead, f.kept = idOf("records", "2026-1-24 06:36:00", "1"), idOf("records", "2026-2-10 05:04:00", "1"), idOf("records", "2026-2-11 05:10:00", "1")
	tombstone(t, f.ledgerPath, f.dead)

	// A registered type at two versions: one reading is left at v1, one is admitted at
	// v1 and corrected under v2, one is admitted at v2.
	v1, v2 := registryAt(t, readingV1), registryAt(t, readingV1, readingV2)
	admitAs(t, f.ledgerPath, "records", v1, []record{
		{"test.reading", readingPayload("2026-04-01T08:00:00Z", 1, "")},
		{"test.reading", readingPayload("2026-04-02T08:00:00Z", 2, "")},
	})
	admitAs(t, f.ledgerPath, "records", v2, []record{
		{"test.reading", readingPayload("2026-04-02T08:00:00Z", 3, "corrected")},
		{"test.reading", readingPayload("2026-04-03T08:00:00Z", 4, "new")},
	})
	f.atV1, f.movedToV2, f.atV2 = idOf("records", "2026-04-01T08:00:00Z"), idOf("records", "2026-04-02T08:00:00Z"), idOf("records", "2026-04-03T08:00:00Z")
	registerInStore(t, f.ledgerFixture, readingV1, readingV2)

	// An edge naming a reading that is live and one that is retracted.
	f.edge = idOf("records", "note:t1:1")
	admitAs(t, f.ledgerPath, "records", bundled, []record{{"record.edge", fmt.Sprintf(
		`{"id": "note:t1:1", "at": "2026-03-01 10:00", "source_time": {"value": "2026-03-01 10:00", "zone": "Australia/Sydney", "offset": "+11:00", "basis": "source"}, "kind": "about", "to": [%q, %q], "text": "two readings"}`,
		f.kept, f.dead)}})

	f.oldLedgerSum, f.oldStoreSum = sumOf(t, f.ledgerPath), sumOf(t, f.store)
	return f
}

func sumOf(t *testing.T, path string) [32]byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return sha256.Sum256(b)
}

func (f migrationFixture) migrate(t *testing.T, extra ...string) (string, error) {
	t.Helper()
	args := append([]string{"migrate", "--from", f.ledgerPath, "--from-store", f.store, "--to", f.newLedger, "--to-store", f.newStore}, extra...)
	out, _, err := cli(t, f.cfgPath, args...)
	return out, err
}

// oldIsUntouched fails the test if the old files changed by a byte.
func (f migrationFixture) oldIsUntouched(t *testing.T) {
	t.Helper()
	if sumOf(t, f.ledgerPath) != f.oldLedgerSum || sumOf(t, f.store) != f.oldStoreSum {
		t.Error("the old ledger or operational store changed: a migration only reads them")
	}
}

func foldOf(t *testing.T, path string) fold.Result {
	t.Helper()
	ctx := context.Background()
	l, err := ledger.OpenReadOnly(ctx, path, "matt")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()
	res, err := fold.Read(ctx, l)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestMigrate_DryRunReportsAndWritesNothing(t *testing.T) {
	t.Parallel()
	f := newMigrationFixture(t)

	out, err := f.migrate(t)
	if err != nil {
		t.Fatalf("dry run: %v\n%s", err, out)
	}
	for _, want := range []string{
		"health.blood-pressure  bp-csv  1  0", "health.blood-pressure  records  2  0", "test.reading  records  3  0",
		"record.edge  records  1  0", "total  7  0", "dry run: nothing written",
		"link to a facet not carried: records:9:note:t1:1", f.dead + ", tombstoned",
	} {
		if !strings.Contains(squash(out), squash(want)) {
			t.Errorf("the dry run lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, f.kept+", ") {
		t.Errorf("a link to a facet that is carried was listed:\n%s", out)
	}
	for _, p := range []string{f.newLedger, f.newStore, filepath.Dir(f.newLedger)} {
		if _, err := os.Stat(p); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("the dry run made %s (%v)", p, err)
		}
	}
	f.oldIsUntouched(t)
}

// squash collapses runs of spaces, so a tabwriter row reads as the fields it holds.
func squash(s string) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(s, "\t", " ")), " ")
}

func TestMigrate_CarriesLiveCurrentVersionsUnderTheirDefinitionVersion(t *testing.T) {
	t.Parallel()
	f := newMigrationFixture(t)

	out, err := f.migrate(t, "--yes")
	if err != nil {
		t.Fatalf("--yes: %v\n%s", err, out)
	}
	for _, want := range []string{"carried 7 facets", "W old", "W new 7", "proof: the new ledger's 7 live facets equal the selection's 7", "idempotent"} {
		if !strings.Contains(out, want) {
			t.Errorf("--yes did not say %q:\n%s", want, out)
		}
	}
	f.oldIsUntouched(t)

	old, now := foldOf(t, f.ledgerPath), foldOf(t, f.newLedger)
	if now.W != 7 || len(now.Live) != len(old.Live) {
		t.Fatalf("new ledger at W %d with %d live facets; old has %d live: one event per live facet", now.W, len(now.Live), len(old.Live))
	}
	byID := map[string]facet.Facet{}
	for _, nf := range now.Live {
		byID[nf.ID] = nf
	}
	for _, of := range old.Live {
		nf, ok := byID[of.ID]
		switch {
		case !ok:
			t.Errorf("%s (%s) is live in the old ledger and missing from the new", of.ID, of.Type)
		case nf.Descriptor.ContentHash != of.Descriptor.ContentHash || !nf.FactTime.Equal(of.FactTime):
			t.Errorf("%s: hash %s fact time %s, was %s %s", of.ID, nf.Descriptor.ContentHash, nf.FactTime, of.Descriptor.ContentHash, of.FactTime)
		case nf.DefinitionVersion != of.DefinitionVersion || nf.Type != of.Type || nf.Provenance != of.Provenance || nf.Binding != of.Binding:
			t.Errorf("%s: carried as %s v%d from %+v, was %s v%d from %+v", of.ID, nf.Type, nf.DefinitionVersion, nf.Provenance, of.Type, of.DefinitionVersion, of.Provenance)
		case nf.ArtifactVersion != 1:
			t.Errorf("%s is at artifact version %d, want the one version carried", of.ID, nf.ArtifactVersion)
		}
	}
	if _, dead := byID[f.dead]; dead {
		t.Error("the retracted reading was carried")
	}
	if got := byID[f.changed].Descriptor.ContentHash; got != currentHash(t, old, f.changed) {
		t.Errorf("the corrected reading arrived with hash %s, not its current version's", got)
	}
	// The reading left at v1 arrives under v1, though the gate would admit its payload
	// under v2 today; the one corrected under v2 and the one admitted at v2 arrive at v2.
	for id, want := range map[string]int{f.atV1: 1, f.movedToV2: 2, f.atV2: 2} {
		if got := byID[id].DefinitionVersion; got != want {
			t.Errorf("%s arrived under definition v%d, want v%d", id, got, want)
		}
	}
	assertRegisteredCarried(t, f)
}

func currentHash(t *testing.T, res fold.Result, id string) string {
	t.Helper()
	for _, f := range res.Live {
		if f.ID == id {
			return f.Descriptor.ContentHash
		}
	}
	t.Fatalf("%s is not live", id)
	return ""
}

// assertRegisteredCarried holds the new operational store to the old one's registered
// Definitions and projection, row for row, registration moment included.
func assertRegisteredCarried(t *testing.T, f migrationFixture) {
	t.Helper()
	ctx := context.Background()
	read := func(path string) ops.Registered {
		db, err := ops.OpenReadOnly(ctx, path)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = db.Close() }()
		r, err := db.ReadRegistered(ctx)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	was, now := read(f.store), read(f.newStore)
	if len(now.Definitions) != 2 || len(now.Projections) != 1 {
		t.Fatalf("new store holds %d Definitions and %d projections, want 2 and 1", len(now.Definitions), len(now.Projections))
	}
	for i := range was.Definitions {
		if was.Definitions[i] != now.Definitions[i] {
			t.Errorf("Definition row %d: %+v, was %+v", i, now.Definitions[i], was.Definitions[i])
		}
	}
	if was.Projections[0] != now.Projections[0] {
		t.Errorf("projection row: %+v, was %+v", now.Projections[0], was.Projections[0])
	}
}

func TestMigrate_ProgramCarriesOnlyWhatItPicks(t *testing.T) {
	t.Parallel()
	f := newMigrationFixture(t)
	prog := filepath.Join(f.dir, "edges.dl")
	writeDoc(t, prog, "pick/1.\npick(E) :- entry(E, T, _, _, _, _, _), T = \"record.edge\".\n")

	out, err := f.migrate(t, "--program", prog)
	if err != nil {
		t.Fatalf("dry run: %v\n%s", err, out)
	}
	if !strings.Contains(squash(out), "record.edge records 1 0") || strings.Contains(out, "health.blood-pressure") {
		t.Errorf("the dry run is not the one type:\n%s", out)
	}
	if !strings.Contains(out, "link to a facet not carried: records:9:note:t1:1") ||
		!strings.Contains(out, f.kept+", live, not selected") || !strings.Contains(out, f.dead+", tombstoned") {
		t.Errorf("the edge's two ends outside the selection are not listed:\n%s", out)
	}

	if out, err = f.migrate(t, "--program", prog, "--yes"); err != nil {
		t.Fatalf("--yes: %v\n%s", err, out)
	}
	now := foldOf(t, f.newLedger)
	if len(now.Live) != 1 || now.Live[0].ID != f.edge || now.Live[0].Type != "record.edge" {
		t.Errorf("new ledger holds %d live facets, want the one edge", len(now.Live))
	}
	f.oldIsUntouched(t)
}

func TestMigrate_RefusesAnExistingTarget(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"ledger", "store"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newMigrationFixture(t)
			existing := f.newLedger
			if name == "store" {
				existing = f.newStore
			}
			if err := os.MkdirAll(filepath.Dir(existing), 0o750); err != nil {
				t.Fatal(err)
			}
			writeDoc(t, existing, "somebody's file")

			for _, extra := range [][]string{nil, {"--yes"}} {
				out, err := f.migrate(t, extra...)
				if !errors.Is(err, migrate.ErrExists) || !strings.Contains(err.Error(), existing) {
					t.Errorf("migrate %v: %v, want ErrExists naming %s\n%s", extra, err, existing, out)
				}
			}
			if b, err := os.ReadFile(existing); err != nil || string(b) != "somebody's file" {
				t.Errorf("the existing file was touched: %q, %v", b, err)
			}
			f.oldIsUntouched(t)
		})
	}
}

// A facet admitted under a Definition version the old store does not hold as a contract
// cannot be carried faithfully, so the dry run names it and a real run writes nothing.
func TestMigrate_RefusesAFacetWhoseDefinitionVersionIsNotHeld(t *testing.T) {
	t.Parallel()
	f := newLedgerFixture(t)
	admitAs(t, f.ledgerPath, "records", registryAt(t, readingV1), []record{{"test.reading", readingPayload("2026-04-01T08:00:00Z", 1, "")}})
	// The store registers v2 only: v1 is not held anywhere.
	ctx := context.Background()
	buckets, err := config.Config{BucketWidth: span.Default}.Buckets()
	if err != nil {
		t.Fatal(err)
	}
	db, err := ops.Open(ctx, filepath.Join(f.dir, "tracking.db"), buckets)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.RegisterDefinition(ctx, ops.DefinitionRow{Type: "test.reading", Version: 2, Body: readingV2}, "2026-10-22T00:00:00.000000000Z"); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	newLedger := filepath.Join(f.dir, "new", "ledger.db")
	args := []string{"migrate", "--from", f.ledgerPath, "--from-store", filepath.Join(f.dir, "tracking.db"),
		"--to", newLedger, "--to-store", filepath.Join(f.dir, "new", "tracking.db")}
	for _, extra := range [][]string{nil, {"--yes"}} {
		out, _, err := cli(t, f.cfgPath, append(args, extra...)...)
		if err == nil || !strings.Contains(err.Error(), "1 facets the gate would refuse") ||
			!strings.Contains(out, "refused 1 test.reading from records at v1") || !strings.Contains(out, "no Definition of test.reading at v1 is held") {
			t.Errorf("migrate %v: %v\n%s", extra, err, out)
		}
	}
	if _, err := os.Stat(newLedger); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a refused run made the new ledger (%v)", err)
	}
}
