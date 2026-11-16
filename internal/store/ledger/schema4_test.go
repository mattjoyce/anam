package ledger_test

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/store/ledger"
)

func TestSchema4_CarryPreservesTheRecord(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := buildV3(t)
	db := rawDB(t, path)
	before := ledgerState(t, db)
	l, err := ledger.Create(ctx, path, person)
	if err != nil {
		t.Fatalf("Init schema 3: %v", err)
	}
	t.Cleanup(func() { _ = l.Close() })
	after := ledgerState(t, db)
	for _, part := range []string{"facets", "events", "W"} {
		t.Run(part, func(t *testing.T) {
			t.Parallel()
			if !reflect.DeepEqual(after[part], before[part]) {
				t.Errorf("carry changed %s by column position: got %v, want %v", part, after[part], before[part])
			}
		})
	}
	t.Run("pins", func(t *testing.T) {
		t.Parallel()
		for _, pin := range before["pins"] {
			got := queryValues(t, db, `SELECT key, value FROM ledger_meta WHERE key = ?`, pin[0])
			if !reflect.DeepEqual(got, [][]any{pin}) {
				t.Errorf("carry changed original pin %v: got %v", pin, got)
			}
		}
		want := [][]any{{"person_id", person}, {"subject_id", person}}
		if got := queryValues(t, db, `SELECT key, value FROM ledger_meta WHERE key IN ('person_id', 'subject_id') ORDER BY key`); !reflect.DeepEqual(got, want) {
			t.Errorf("subject pins = %v, want %v", got, want)
		}
		got := queryValues(t, db, `SELECT value FROM ledger_meta WHERE key = 'schema_migration:4'`)
		if len(got) != 1 || got[0][0] == "" {
			t.Errorf("missing migration record: %v", got)
		}
		if len(after["pins"]) != len(before["pins"])+2 {
			t.Errorf("carry must add exactly two pins: %v", after["pins"])
		}
	})
	if w, err := l.Head(ctx); err != nil || int64(w) != before["W"][0][0] {
		t.Errorf("Head after carry = %d, %v; want %v", w, err, before["W"])
	}
	if id, err := l.ID(ctx); err != nil || id != "v3-ledger" {
		t.Errorf("ledger id after carry = %q, %v", id, err)
	}
	if f, err := l.Version(ctx, "f1", 2); err != nil || f.Binding.SubjectID != person || f.Series != "arm:left" {
		t.Errorf("read carried facet = %+v, %v", f, err)
	}
}

func TestSchema4_FreshAndCarriedSchemaMatch(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	carried := buildV3(t)
	l, err := ledger.Create(ctx, carried, person)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	fresh, err := ledger.Create(ctx, filepath.Join(t.TempDir(), "fresh.db"), person)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fresh.Close() })
	got, want := ledgerState(t, rawDB(t, carried))["schema"], ledgerState(t, rawDB(t, fresh.Path()))["schema"]
	if !reflect.DeepEqual(got, want) {
		t.Errorf("sqlite_master SQL differs:\ncarried: %v\nfresh: %v", got, want)
	}
	pins := queryValues(t, rawDB(t, fresh.Path()), `SELECT key, value FROM ledger_meta WHERE key != 'ledger_id' ORDER BY key`)
	if want := [][]any{{"schema_version", "4"}, {"subject_id", person}}; !reflect.DeepEqual(pins, want) {
		t.Errorf("fresh pins = %v, want %v", pins, want)
	}
}

func TestSchema4_InitIsIdempotent(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"fresh", "carried"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "ledger.db")
			if kind == "carried" {
				path = buildV3(t)
			}
			l, err := ledger.Create(context.Background(), path, person)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = l.Close() })
			db := rawDB(t, path)
			before := ledgerState(t, db)
			if err := l.Init(context.Background()); err != nil {
				t.Fatalf("second Init: %v", err)
			}
			if after := ledgerState(t, db); !reflect.DeepEqual(after, before) {
				t.Errorf("second Init changed the file: got %v, want %v", after, before)
			}
		})
	}
}

func TestSchema4_PinInsertFailureRollsBackRename(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := buildV3(t)
	db := rawDB(t, path)
	// The equal pin gets past identity validation, then the append-only trigger
	// refuses step 4's INSERT, after its ALTER TABLE has executed.
	if _, err := db.ExecContext(ctx, `INSERT INTO ledger_meta VALUES ('subject_id', 'test-person')`); err != nil {
		t.Fatal(err)
	}
	before := ledgerState(t, db)
	l, err := ledger.Create(ctx, path, person)
	closeIfOpen(l)
	if err == nil || !strings.Contains(err.Error(), "migrate to schema 4") || !strings.Contains(err.Error(), "already pinned") {
		t.Fatalf("Init with an existing subject pin = %v, want step 4 insert failure", err)
	}
	if after := ledgerState(t, db); !reflect.DeepEqual(after, before) {
		t.Errorf("failed migration changed schema 3: got %v, want %v", after, before)
	}
	l, err = ledger.OpenReadOnly(ctx, path, person)
	closeIfOpen(l)
	if !errors.Is(err, ledger.ErrSchemaMismatch) || !strings.Contains(err.Error(), "holds schema 3, this build writes 4") {
		t.Errorf("after rollback open = %v, want schema 3 refusal", err)
	}
}
