package ledger_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/store/ledger"
)

// FacetEvents is one facet's history to a watermark, in seq order, and nothing of
// any other facet: the slice of Replay that concerns it.
func TestFacetEvents_IsTheFacetsSliceOfReplay(t *testing.T) {
	t.Parallel()
	l := newLedger(t)
	ctx := context.Background()

	a := testFacet(t, "bp-csv:a", `{"systolic": 120}`, 1)
	b := testFacet(t, "bp-csv:b", `{"systolic": 121}`, 1)
	if err := l.Admit(ctx, 0, a, admittedEvent(a, 1)); err != nil {
		t.Fatalf("Admit a: %v", err)
	}
	if err := l.Admit(ctx, 1, b, admittedEvent(b, 2)); err != nil {
		t.Fatalf("Admit b: %v", err)
	}
	tomb := facet.AnamEvent{
		Kind: facet.EventTombstoned, FacetID: a.ID, Seq: 3, ArtifactVersion: 1,
		AnamTime: time.Date(2026, 7, 10, 10, 0, 0, 0, time.UTC),
	}
	if err := l.Mark(ctx, 2, tomb); err != nil {
		t.Fatalf("Mark: %v", err)
	}

	for _, tc := range []struct {
		upto ledger.Watermark
		want []int64
	}{
		{0, nil},
		{1, []int64{1}},
		{2, []int64{1}},
		{3, []int64{1, 3}},
		{9, []int64{1, 3}},
	} {
		got, err := l.FacetEvents(ctx, a.ID, tc.upto)
		if err != nil {
			t.Fatalf("FacetEvents(a, %d): %v", tc.upto, err)
		}
		var seqs []int64
		for _, ev := range got {
			if ev.FacetID != a.ID {
				t.Errorf("FacetEvents(a, %d) returned an event of %s", tc.upto, ev.FacetID)
			}
			seqs = append(seqs, ev.Seq)
		}
		if len(seqs) != len(tc.want) {
			t.Errorf("FacetEvents(a, %d) seqs %v, want %v", tc.upto, seqs, tc.want)
			continue
		}
		for i := range seqs {
			if seqs[i] != tc.want[i] {
				t.Errorf("FacetEvents(a, %d) seqs %v, want %v", tc.upto, seqs, tc.want)
				break
			}
		}
	}
	none, err := l.FacetEvents(ctx, "never-admitted", 3)
	if err != nil || len(none) != 0 {
		t.Errorf("FacetEvents(unknown) = %v, %v; want none", none, err)
	}
}

// A ledger written before the index existed gains it on its next writable open, with
// no migration and no version change, and FacetEvents' plan uses it, so its cost is
// the facet's history and not the ledger's.
func TestOpen_AWriterAddsTheFacetIndexToAnOlderFile(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "ledger.db")
	l, err := ledger.Create(ctx, path, person)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	_ = l.Close()

	raw := rawDB(t, path)
	if _, err := raw.ExecContext(ctx, `DROP INDEX anam_event_by_facet`); err != nil {
		t.Fatalf("drop the index to stand in for an older file: %v", err)
	}
	if hasIndex(t, raw) {
		t.Fatal("the index survived DROP INDEX")
	}

	r, err := ledger.OpenReadOnly(ctx, path, person)
	if err != nil {
		t.Fatalf("OpenReadOnly without the index: %v", err)
	}
	if _, err := r.FacetEvents(ctx, "x", 1); err != nil {
		t.Errorf("FacetEvents without the index: %v", err)
	}
	_ = r.Close()
	if hasIndex(t, raw) {
		t.Error("a read-only open created the index")
	}

	w, err := ledger.Open(ctx, path, person)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = w.Close() }()
	if !hasIndex(t, raw) {
		t.Fatal("a writable open did not create the index")
	}

	rows, err := raw.QueryContext(ctx, `EXPLAIN QUERY PLAN SELECT seq FROM anam_event
		WHERE facet_id = ? AND seq <= ? ORDER BY seq`, "x", 1)
	if err != nil {
		t.Fatalf("explain: %v", err)
	}
	defer func() { _ = rows.Close() }()
	var plan []string
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatalf("scan plan: %v", err)
		}
		plan = append(plan, detail)
	}
	if joined := strings.Join(plan, "; "); !strings.Contains(joined, "anam_event_by_facet") {
		t.Errorf("FacetEvents' query does not use the index: %s", joined)
	}
}

// hasIndex reports whether the file holds the facet index.
func hasIndex(t *testing.T, raw *sql.DB) bool {
	t.Helper()
	var n int
	err := raw.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = 'anam_event_by_facet'`).Scan(&n)
	if err != nil {
		t.Fatalf("read sqlite_master: %v", err)
	}
	return n == 1
}
