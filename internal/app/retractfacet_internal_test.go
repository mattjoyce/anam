package app

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mattjoyce/anam/internal/config"
	"github.com/mattjoyce/anam/internal/fold"
	"github.com/mattjoyce/anam/internal/span"
	"github.com/mattjoyce/anam/internal/store/ledger"
	"github.com/mattjoyce/anam/internal/store/ops"
)

// firstFacet is the id of one live facet of source, by folding the ledger.
func firstFacet(t *testing.T, path, source string) string {
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
	for _, e := range res.Value.Entries() {
		if e.Source == source {
			return e.FacetID
		}
	}
	t.Fatalf("no live facet of %s", source)
	return ""
}

// Card 188 slice 3: one facet of a source nobody resends is retracted by name. A dry
// run writes nothing, the three refusals each name their rule, and the tombstone leaves
// the facet dead now and live as of before it.
func TestRetractFacet_OneFacetByHand(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newLedgerFixture(t)
	before := f.admit(t, bpReadings)
	// The store exists once the daemon or init has run; a dry run only reads it.
	buckets, err := config.Config{BucketWidth: span.Default}.Buckets()
	if err != nil {
		t.Fatal(err)
	}
	db, err := ops.Open(ctx, filepath.Join(f.dir, "tracking.db"), buckets)
	if err != nil {
		t.Fatal(err)
	}
	id := firstFacet(t, f.ledgerPath, "records")
	retract := func(source string, more ...string) (string, error) {
		out, _, err := cli(t, f.cfgPath, append([]string{"retract", source, "--facet", id}, more...)...)
		return out, err
	}

	// A dry run shows the facet and what the tombstone would do, and writes nothing.
	out, err := retract("records")
	if err != nil || !strings.Contains(out, id) || !strings.Contains(out, "health.blood-pressure") ||
		!strings.Contains(out, "v1") || !strings.Contains(out, "status admitted") || !strings.Contains(out, "dry run") {
		t.Fatalf("dry run: %v\n%s", err, out)
	}
	if w := headOf(t, f.ledgerPath); w != before {
		t.Fatalf("dry run moved W from %d to %d", before, w)
	}

	// Another source's name is refused, and writes nothing.
	if _, err := retract("workbench", "--yes"); err == nil || !strings.Contains(err.Error(), "another source") {
		t.Fatalf("wrong source: %v", err)
	}
	if w := headOf(t, f.ledgerPath); w != before {
		t.Fatalf("refused retraction moved W to %d", w)
	}

	// A source an importer declares and has a complete snapshot for is refused; one
	// that is declared and has none is not.
	importers := filepath.Join(f.dir, "importers.yaml")
	writeDoc(t, importers, "importers:\n  - source: records\n    token_sha256: "+strings.Repeat("ab", 32)+
		"\n    types: [health.blood-pressure]\n")
	appendFile(t, f.cfgPath, "importers_file: "+importers+"\n")
	if out, err = retract("records"); err != nil {
		t.Fatalf("declared, no snapshot: %v\n%s", err, out)
	}
	if _, err := db.RecordSnapshotPart(ctx, ops.SnapshotPart{ID: "run", Source: "records",
		Types: []string{"health.blood-pressure"}, Part: 1, Parts: 1, FacetIDs: []string{id}}, time.Now(), int64(before),
		ops.SnapshotRetention{Incomplete: config.DefaultSnapshotTTL, Complete: config.DefaultSnapshotKeep}); err != nil {
		t.Fatal(err)
	}
	if _, err := retract("records", "--yes"); err == nil || !strings.Contains(err.Error(), "reconciled by an importer's snapshot") {
		t.Fatalf("declared with a snapshot: %v", err)
	}
	if w := headOf(t, f.ledgerPath); w != before {
		t.Fatalf("refused retraction moved W to %d", w)
	}
	_ = db.Close()
	writeDoc(t, importers, "importers:\n  - source: garmin\n    token_sha256: "+strings.Repeat("ab", 32)+
		"\n    types: [health.blood-pressure]\n")

	// The tombstone: one anam-event, the facet dead now and live as of before.
	if out, err = retract("records", "--yes"); err != nil || !strings.Contains(out, "tombstoned") {
		t.Fatalf("retract: %v\n%s", err, out)
	}
	if w := headOf(t, f.ledgerPath); w != before+1 {
		t.Errorf("W = %d, want %d", w, before+1)
	}
	if n := liveOf(t, f.ledgerPath, "records", 0); n != 2 {
		t.Errorf("%d records facets live after retracting one of 3, want 2", n)
	}
	if n := liveOf(t, f.ledgerPath, "records", before); n != 3 {
		t.Errorf("as of W=%d, %d records facets live, want 3", before, n)
	}

	// Not live any more: refused, and nothing more is written.
	if _, err := retract("records", "--yes"); err == nil || !strings.Contains(err.Error(), "not live") {
		t.Fatalf("retracting a tombstoned facet: %v", err)
	}
	if w := headOf(t, f.ledgerPath); w != before+1 {
		t.Errorf("refused retraction moved W to %d", w)
	}

	// The two retractions do not mix.
	if _, _, err := cli(t, f.cfgPath, "retract", "records", "--facet", id, "--retired"); err == nil {
		t.Error("--facet with --retired was accepted")
	}
}
