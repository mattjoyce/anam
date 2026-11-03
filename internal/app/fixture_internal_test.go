package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mattjoyce/anam/internal/catalog/catalogtest"
	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/config"
	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/gate"
	"github.com/mattjoyce/anam/internal/store/ledger"
)

// testTimeout is the timeout every fixture in this package gives the stores and the
// daemon it opens (card 227). The default of 5 s fits a request to a store on local
// disk, but opening a daemon guards every bundled Definition, and under the race
// detector, on a machine running other work, that alone can pass 5 s. A test that
// asserts timeout behaviour sets its own short value.
const testTimeout = 30 * time.Second

// record is one fact as the records importer sends it: a type and its payload, from
// source "records". The tests seed their ledgers with records through the real gate,
// as the importer's batches reach it.
type record struct {
	typ     facet.Type
	payload string
}

// bpReadings is a blood pressure diary as records: three readings, ids in the diary's
// form (date_excel and reading joined by a colon). "114.0" appears once, so a test can
// correct one reading by replacing it.
var bpReadings = []record{
	{"health.blood-pressure", `{"id": "2026-1-24 06:36:00:1", "source_time": {"value": "2026-1-24 06:36:00", "zone": "Australia/Sydney", "basis": "source"}, "date_excel": "2026-1-24 06:36:00", "reading": "1", "systolic_blood_pressure": {"value": 114.0, "unit": "mmHg"}, "diastolic_blood_pressure": {"value": 80.0, "unit": "mmHg"}, "effective_time_frame": {"date_time": "2026-01-24T06:36:00+11:00"}, "pulse": "70.0"}`},
	{"health.blood-pressure", `{"id": "2026-2-10 05:04:00:1", "source_time": {"value": "2026-2-10 05:04:00", "zone": "Australia/Sydney", "basis": "source"}, "date_excel": "2026-2-10 05:04:00", "reading": "1", "systolic_blood_pressure": {"value": 102.0, "unit": "mmHg"}, "diastolic_blood_pressure": {"value": 72.0, "unit": "mmHg"}, "effective_time_frame": {"date_time": "2026-02-10T05:04:00+11:00"}, "pulse": "68.0"}`},
	{"health.blood-pressure", `{"id": "2026-2-11 05:10:00:1", "source_time": {"value": "2026-2-11 05:10:00", "zone": "Australia/Sydney", "basis": "source"}, "date_excel": "2026-2-11 05:10:00", "reading": "1", "systolic_blood_pressure": {"value": 101.0, "unit": "mmHg"}, "diastolic_blood_pressure": {"value": 72.0, "unit": "mmHg"}, "effective_time_frame": {"date_time": "2026-02-11T05:10:00+11:00"}, "pulse": "66.0"}`},
}

// corrected is recs with old replaced by new in the first payload that holds it.
func corrected(recs []record, old, new string) []record {
	out := append([]record(nil), recs...)
	for i, r := range out {
		if strings.Contains(r.payload, old) {
			out[i].payload = strings.Replace(r.payload, old, new, 1)
			break
		}
	}
	return out
}

// admitUnder commits recs from source "records" through the gate under the Definitions
// in defs, in one batch, and returns the watermark it left. A test admits under
// catalogtest.Definitions, or under a registry holding a type the catalog no longer
// carries, as a ledger written before a rename holds it.
func admitUnder(ctx context.Context, l *ledger.Ledger, defs *def.Registry, recs []record) (ledger.Watermark, error) {
	var err error
	types := make([]facet.Type, len(recs))
	envs := make([]facet.Envelope, len(recs))
	bindings := make([]facet.Binding, len(recs))
	for i, r := range recs {
		types[i] = r.typ
		envs[i] = facet.Envelope{
			Type: r.typ, Source: "records", Injector: "records",
			Payload: []byte(r.payload), AssertedPerson: "matt",
		}
	}
	g := gate.New(l, defs, gate.StaticScopes{"records": types}, clock.Real{}, "matt", gate.Limits{MaxPayloadBytes: config.DefaultMaxPayloadBytes, MaxRetries: config.DefaultCommitRetries})
	for i, env := range envs {
		if bindings[i], err = g.Resolve(env); err != nil {
			return 0, fmt.Errorf("resolve record %d (%s): %w", i, env.Type, err)
		}
	}
	_, w, err := g.CommitBatch(ctx, envs, bindings)
	return w, err
}

// catalogKeys is the config file lines naming a copy of fixture.Mattjoyce under the
// test's own directory, for a config a test writes as YAML.
func catalogKeys(t *testing.T) string {
	t.Helper()
	var cfg config.Config
	catalogtest.Config(t, &cfg)
	return fmt.Sprintf("definitions_dir: %s\nprojections_dir: %s\n", cfg.DefinitionsDir, cfg.ProjectionsDir)
}

// ledgerFixture is a real ledger and a real config under the test's own directory. The
// config reads a copy of fixture.Mattjoyce (card 255), so every local command and every
// daemon opened on it loads that catalog.
type ledgerFixture struct {
	dir, cfgPath, ledgerPath string
}

func newLedgerFixture(t *testing.T) ledgerFixture {
	t.Helper()
	dir := t.TempDir()
	f := ledgerFixture{
		dir:        dir,
		cfgPath:    filepath.Join(dir, "config.yaml"),
		ledgerPath: filepath.Join(dir, "ledger.db"),
	}
	cfg := fmt.Sprintf("person_id: matt\nledger_path: %s\ndb_path: %s\ntimeout: %s\n",
		f.ledgerPath, filepath.Join(dir, "tracking.db"), testTimeout) + catalogKeys(t)
	if err := os.WriteFile(f.cfgPath, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	l, err := ledger.Create(context.Background(), f.ledgerPath, "matt")
	if err != nil {
		t.Fatalf("create ledger: %v", err)
	}
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	return f
}

// TestFixtures_ReadTheFixtureCatalog holds the package's fixtures to a fixture
// catalog: the config newLedgerFixture writes, and the config newRemoteFixture's
// daemon opens on, load their catalog from directories, not the embedded catalog.
// TestTests_ReadNoEmbeddedCatalog (catalogtest) sees a directory set to "" but not a
// YAML config that leaves the keys out, so this is the guard for every local command
// and daemon these fixtures run.
func TestFixtures_ReadTheFixtureCatalog(t *testing.T) {
	t.Parallel()
	check := func(t *testing.T, cfg config.Config) {
		t.Helper()
		cat, err := loadCatalog(context.Background(), cfg)
		if err != nil {
			t.Fatal(err)
		}
		if cat.DefinitionsSource() == "embedded" || cat.ProjectionsSource() == "embedded" {
			t.Errorf("the config loads definitions from %s and projections from %s, want a fixture copy for both",
				cat.DefinitionsSource(), cat.ProjectionsSource())
		}
	}
	t.Run("newLedgerFixture", func(t *testing.T) {
		t.Parallel()
		cfg, err := config.Load(newLedgerFixture(t).cfgPath)
		if err != nil {
			t.Fatal(err)
		}
		check(t, cfg)
	})
	t.Run("newRemoteFixture", func(t *testing.T) {
		t.Parallel()
		var opened config.Config
		newRemoteFixtureWith(t, "UTC", func(_ *testing.T, _ remoteFixture, cfg *config.Config) { opened = *cfg })
		check(t, opened)
	})
}

// admit commits recs to the fixture's ledger and returns the head after.
func (f ledgerFixture) admit(t *testing.T, recs []record) ledger.Watermark {
	t.Helper()
	ctx := context.Background()
	l, err := ledger.Open(ctx, f.ledgerPath, "matt")
	if err != nil {
		t.Fatalf("open ledger: %v", err)
	}
	defer func() { _ = l.Close() }()
	w, err := admitUnder(ctx, l, catalogtest.Definitions(t), recs)
	if err != nil {
		t.Fatalf("admit records: %v", err)
	}
	return w
}

// admitHeld is admit under the Definitions in defs, for a type the binary keeps only
// in its history.
func (f ledgerFixture) admitHeld(t *testing.T, defs *def.Registry, recs []record) ledger.Watermark {
	t.Helper()
	ctx := context.Background()
	l, err := ledger.Open(ctx, f.ledgerPath, "matt")
	if err != nil {
		t.Fatalf("open ledger: %v", err)
	}
	defer func() { _ = l.Close() }()
	w, err := admitUnder(ctx, l, defs, recs)
	if err != nil {
		t.Fatalf("admit records: %v", err)
	}
	return w
}

func writeDoc(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
