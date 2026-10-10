package app

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/fold"
	"github.com/mattjoyce/anam/internal/store/ledger"
)

// liveOf counts the live facets of source at the watermark w, or at the head when w
// is 0, by folding the ledger.
func liveOf(t *testing.T, path, source string, w ledger.Watermark) int {
	t.Helper()
	ctx := context.Background()
	l, err := ledger.OpenReadOnly(ctx, path, "matt")
	if err != nil {
		t.Fatalf("open ledger: %v", err)
	}
	defer func() { _ = l.Close() }()
	var res fold.Result
	if w == 0 {
		res, err = fold.Read(ctx, l)
	} else {
		res, err = fold.ReadAt(ctx, l, w)
	}
	if err != nil {
		t.Fatalf("fold: %v", err)
	}
	n := 0
	for _, e := range res.Value.Entries() {
		if e.Source == source {
			n++
		}
	}
	return n
}

func headOf(t *testing.T, path string) ledger.Watermark {
	t.Helper()
	ctx := context.Background()
	l, err := ledger.OpenReadOnly(ctx, path, "matt")
	if err != nil {
		t.Fatalf("open ledger: %v", err)
	}
	defer func() { _ = l.Close() }()
	w, err := l.Head(ctx)
	if err != nil {
		t.Fatalf("head: %v", err)
	}
	return w
}

// Card 120: a retired source is tombstoned whole, in one deliberate step. It is
// refused while an import or an importer still declares it, refused without the whole
// fraction, reported by type on a dry run, and with --yes every live facet goes through
// the gate as one anam-event each; a read as of before the retirement still shows them.
func TestRetract_RetiredSourceGoesWhole(t *testing.T) {
	t.Parallel()
	f := newIngestFixture(t)
	declared := fmt.Sprintf("imports:\n  - source: records\n    path: %s\n", f.recordsDir)
	if _, _, err := f.ingest(t, declared); err != nil {
		t.Fatalf("seed the ledger: %v", err)
	}
	before := headOf(t, f.ledgerPath)
	live := liveOf(t, f.ledgerPath, "records", 0)
	if live != 3 {
		t.Fatalf("seeded %d live records facets, want 3", live)
	}
	stillImported := filepath.Join(f.dir, "imports.yaml") // written by ingest
	other := filepath.Join(f.dir, "imports-other.yaml")
	writeDoc(t, other, "imports:\n  - source: documents\n    path: "+f.recordsDir+"\n")
	retire := func(imports string, more ...string) (string, error) {
		args := append([]string{"retract", "records", "--retired", "--imports", imports}, more...)
		out, _, err := cli(t, f.cfgPath, args...)
		return out, err
	}
	unchanged := func(step string) {
		t.Helper()
		if w := headOf(t, f.ledgerPath); w != before {
			t.Fatalf("%s: W moved from %d to %d", step, before, w)
		}
	}

	// Still an import: refused, whatever else is said.
	if _, err := retire(stillImported, "--yes", "--max-fraction", "1"); err == nil || !strings.Contains(err.Error(), "still declared as an import") {
		t.Fatalf("declared import: %v", err)
	}
	unchanged("declared import")

	// Still an importer: refused.
	importers := filepath.Join(f.dir, "importers.yaml")
	writeDoc(t, importers, "importers:\n  - source: records\n    token_sha256: "+strings.Repeat("ab", 32)+
		"\n    types: [health.blood-pressure]\n")
	appendFile(t, f.cfgPath, "importers_file: "+importers+"\n")
	if _, err := retire(other, "--yes", "--max-fraction", "1"); err == nil || !strings.Contains(err.Error(), "still declared as an importer") {
		t.Fatalf("declared importer: %v", err)
	}
	unchanged("declared importer")
	writeDoc(t, importers, "importers:\n  - source: garmin\n    token_sha256: "+strings.Repeat("ab", 32)+
		"\n    types: [health.blood-pressure]\n")

	// Undeclared, but the default fraction does not cover the whole source: refused,
	// after the report.
	out, err := retire(other, "--yes")
	if err == nil || !strings.Contains(err.Error(), "--max-fraction 1") {
		t.Fatalf("default fraction: %v", err)
	}
	if !strings.Contains(out, "3 live facets") || !strings.Contains(out, "health.blood-pressure\t3") &&
		!strings.Contains(out, "health.blood-pressure  3") {
		t.Fatalf("report by type missing:\n%s", out)
	}
	unchanged("default fraction")

	// A dry run says so and writes nothing.
	if out, err = retire(other, "--max-fraction", "1"); err != nil || !strings.Contains(out, "dry run") {
		t.Fatalf("dry run: %v\n%s", err, out)
	}
	unchanged("dry run")

	// Asked for in full: every live facet is tombstoned, one event each.
	if out, err = retire(other, "--yes", "--max-fraction", "1"); err != nil || !strings.Contains(out, "tombstoned 3 facets") {
		t.Fatalf("retire: %v\n%s", err, out)
	}
	if w := headOf(t, f.ledgerPath); w != before+3 {
		t.Errorf("W = %d after retiring 3 facets from %d", w, before)
	}
	if n := liveOf(t, f.ledgerPath, "records", 0); n != 0 {
		t.Errorf("%d records facets still live", n)
	}
	// The artifacts stay: a read as of before the retirement still folds every one.
	if n := liveOf(t, f.ledgerPath, "records", before); n != 3 {
		t.Errorf("as of W=%d, %d records facets live, want 3", before, n)
	}

	// Done is done: a second run has nothing to plan.
	if out, err = retire(other, "--yes", "--max-fraction", "1"); err != nil || !strings.Contains(out, "0 live facets") {
		t.Fatalf("second run: %v\n%s", err, out)
	}
}
