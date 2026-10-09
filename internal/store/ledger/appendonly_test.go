package ledger

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/mattjoyce/anam/internal/facet"
)

// Append-only is held by the database, so these tests do not go through the package
// at all. They open the file the way a sqlite3 shell or a hurried script would, on a
// plain connection with none of this package's settings, and issue the statement
// directly. The threat is not an adversary. It is us, mid-refactor, reaching for the
// convenient thing.

const internalPerson = "test-person"

// seeded returns an initialised ledger holding one artifact and its event, and the
// path it lives at.
func seeded(t *testing.T) (*Ledger, string, facet.Facet) {
	t.Helper()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "ledger.db")

	l, err := Create(ctx, path, internalPerson)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() { _ = l.Close() })

	canonical, err := facet.Canonicalize([]byte(`{"systolic": 118}`))
	if err != nil {
		t.Fatalf("canonicalize: %v", err)
	}
	f := facet.Facet{
		ID: facet.NewID("bp-csv:seed"), Type: "health.blood-pressure", Confidence: 1,
		Classification: facet.ClassificationNormal, ArtifactVersion: 1, DefinitionVersion: 1,
		FactTime:   time.Date(2026, 7, 5, 6, 30, 0, 0, time.UTC),
		Binding:    facet.Binding{PersonID: internalPerson, Assurance: facet.AssuranceHigh},
		Provenance: facet.Provenance{Source: "bp-csv", Injector: "bp-csv", SourceRef: "bp-csv:seed"},
		Descriptor: facet.Descriptor{
			SourceSchema: "bp-csv/reading", SchemaVersion: 1,
			ContentHash: facet.ContentHash(canonical),
			PayloadKind: facet.PayloadVerbatim, MediaType: "application/json",
		},
		Payload: canonical,
	}
	ev := facet.AnamEvent{
		Kind: facet.EventAdmitted, FacetID: f.ID, Seq: 1, ArtifactVersion: 1,
		AnamTime: time.Date(2026, 7, 10, 9, 0, 0, 0, time.UTC),
	}
	if err := l.Admit(ctx, 0, f, ev); err != nil {
		t.Fatalf("Admit: %v", err)
	}
	return l, path, f
}

// outsider opens the file on a plain connection: no pragmas, no transaction mode,
// nothing this package would have set.
func outsider(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open outsider connection: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestAppendOnly_DatabaseRefusesEveryOverwrite(t *testing.T) {
	t.Parallel()
	l, path, f := seeded(t)
	ctx := context.Background()
	raw := outsider(t, path)

	statements := []struct {
		name string
		sql  string
		args []any
	}{
		{"update an artifact", `UPDATE facet SET payload = x'00'`, nil},
		{"delete an artifact", `DELETE FROM facet`, nil},
		{"replace an artifact", `INSERT OR REPLACE INTO facet (` + facetColumns + `)
			SELECT facet_id, artifact_version, type, confidence, classification,
			       definition_version, fact_time, binding_person_id, binding_assurance,
			       binding_span_hint, prov_source, prov_injector, prov_source_ref, prov_method,
			       prov_model, prov_watermark, source_schema, schema_version, content_hash,
			       payload_kind, media_type, x'00' FROM facet`, nil},
		{"update an event", `UPDATE anam_event SET kind = 'tombstoned'`, nil},
		{"delete an event", `DELETE FROM anam_event`, nil},
		{"replace an event", `INSERT OR REPLACE INTO anam_event (` + eventColumns + `)
			VALUES (1, 'tombstoned', 'x', 'x', 1, 0)`, nil},
		{"update a pin", `UPDATE ledger_meta SET value = 'someone-else' WHERE key = 'person_id'`, nil},
		{"delete a pin", `DELETE FROM ledger_meta`, nil},
		{"replace a pin", `INSERT OR REPLACE INTO ledger_meta (key, value) VALUES ('person_id', 'someone-else')`, nil},
	}
	for _, s := range statements {
		if _, err := raw.ExecContext(ctx, s.sql, s.args...); err == nil {
			t.Errorf("%s: the database accepted it", s.name)
		}
	}

	// And nothing moved: the artifact, the event and the pins read back as written.
	got, err := l.Version(ctx, f.ID, 1)
	if err != nil {
		t.Fatalf("Version after the attempts: %v", err)
	}
	if string(got.Payload.Bytes()) != string(f.Payload.Bytes()) {
		t.Errorf("payload changed: got %s, want %s", got.Payload.Bytes(), f.Payload.Bytes())
	}
	events, err := l.Replay(ctx, 0, 1)
	if err != nil || len(events) != 1 || events[0].Kind != facet.EventAdmitted {
		t.Errorf("events after the attempts = %+v (err %v), want the one admitted event", events, err)
	}
	if err := l.verify(ctx); err != nil {
		t.Errorf("pins after the attempts: %v", err)
	}
}

// W is MAX(seq), so gapless seqs are what make the watermark mean anything. The
// database holds that too: an event may only take the seq after the newest one.
func TestSeq_DatabaseRefusesAGapOrARepeat(t *testing.T) {
	t.Parallel()
	l, path, _ := seeded(t)
	ctx := context.Background()
	raw := outsider(t, path)

	for _, seq := range []int64{1, 3, 0, -1} {
		_, err := raw.ExecContext(ctx, insertEventSQL, seq, "admitted", "x", "x", 1, 0)
		if err == nil {
			t.Errorf("an event at seq %d was accepted while the head is 1", seq)
		}
	}
	if _, err := raw.ExecContext(ctx,
		`INSERT INTO anam_event (kind, facet_id, anam_time, artifact_version) VALUES ('admitted', 'x', 'x', 1)`); err == nil {
		t.Error("an event with no seq was accepted; the database must not assign one")
	}
	if w, err := l.Head(ctx); err != nil || w != 1 {
		t.Errorf("head = %d (err %v) after the refused inserts, want 1", w, err)
	}

	// Through the package, the same refusal is ErrCorrupt: the ledger's own checks
	// passed, the database disagreed, and no write can be trusted after that.
	ev := facet.AnamEvent{Kind: facet.EventTombstoned, FacetID: "x", Seq: 7, ArtifactVersion: 1,
		AnamTime: time.Date(2026, 7, 10, 9, 0, 0, 0, time.UTC)}
	if err := refused("anam-event", insertEvent(ctx, l.db, ev)); !errors.Is(err, ErrCorrupt) {
		t.Errorf("a refused insert mapped to %v, want ErrCorrupt", err)
	}
}

// KnownFacets reads a missing v1 as "never admitted". That is only safe if no artifact
// can exist above a missing version.
func TestVersion_DatabaseRefusesAVersionWithNothingBeneathIt(t *testing.T) {
	t.Parallel()
	l, _, f := seeded(t)
	ctx := context.Background()

	orphan := f
	orphan.ID = facet.NewID("bp-csv:never-had-a-v1")
	orphan.ArtifactVersion = 2
	if err := insertFacet(ctx, l.db, orphan); err == nil {
		t.Error("v2 of a facet with no v1 was accepted")
	}

	skipped := f
	skipped.ArtifactVersion = 3
	if err := insertFacet(ctx, l.db, skipped); err == nil {
		t.Error("v3 of a facet whose newest version is v1 was accepted")
	}
}

// Lock contention has its own sentinel, so an operator sees "another writer" and not
// a bare result code.
func TestClassify_LockContentionIsErrBusy(t *testing.T) {
	t.Parallel()
	_, path, _ := seeded(t)
	ctx := context.Background()

	holder, err := outsider(t, path).Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = holder.Close() }()
	if _, err := holder.ExecContext(ctx, `BEGIN IMMEDIATE`); err != nil {
		t.Fatalf("take the write lock: %v", err)
	}
	defer func() { _, _ = holder.ExecContext(ctx, `ROLLBACK`) }()

	// A plain connection has no busy timeout, so it reports the lock at once.
	_, err = outsider(t, path).ExecContext(ctx, `BEGIN IMMEDIATE`)
	if err == nil {
		t.Fatal("a second writer took the lock while the first held it")
	}
	if got := classify("begin write", err); !errors.Is(got, ErrBusy) {
		t.Errorf("classify = %v, want ErrBusy", got)
	}
}

// A backup carries the triggers: a restored ledger is append-only for every writer,
// as the one it was taken from.
func TestAppendOnly_ABackupKeepsTheTriggers(t *testing.T) {
	t.Parallel()
	l, _, _ := seeded(t)
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "ledger.db")
	if err := l.BackupTo(ctx, path); err != nil {
		t.Fatalf("BackupTo: %v", err)
	}
	raw := outsider(t, path)
	for _, stmt := range []string{`DELETE FROM facet`, `UPDATE anam_event SET kind = 'tombstoned'`, `DELETE FROM ledger_meta`} {
		if _, err := raw.ExecContext(ctx, stmt); err == nil {
			t.Errorf("%s: the backup accepted it", stmt)
		}
	}
}
