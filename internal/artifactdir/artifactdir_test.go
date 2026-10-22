package artifactdir_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/artifactdir"
	"github.com/mattjoyce/anam/internal/span"
	"github.com/mattjoyce/anam/internal/store/ops"
)

// The move is tested over a folder written the way the artifact store wrote it, and a
// real operational store file, each under the test's own directory.

func store(t *testing.T) *ops.DB {
	t.Helper()
	buckets, err := span.New(span.Default)
	if err != nil {
		t.Fatal(err)
	}
	db, err := ops.Open(context.Background(), filepath.Join(t.TempDir(), "tracking.db"), buckets)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// put writes body into the folder as <kind>/sha256-<hex>.json and returns its digest.
func put(t *testing.T, dir, kind, body string) string {
	t.Helper()
	digest := ops.Digest([]byte(body))
	writeFile(t, filepath.Join(dir, kind, "sha256-"+strings.TrimPrefix(digest, "sha256:")+".json"), body)
	return digest
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// oldFolder is a folder of every kind: two checkpoint values (one the log will name),
// a receipt, a served value, a saved selection and the code of one.
type oldFolder struct {
	dir                        string
	named, unnamed             string
	receipt, served, selection string
	code                       string
}

func newOldFolder(t *testing.T) oldFolder {
	t.Helper()
	dir := t.TempDir()
	return oldFolder{
		dir:       dir,
		named:     put(t, dir, ops.KindCheckpoint, `[{"facet_id":"a"}]`),
		unnamed:   put(t, dir, ops.KindCheckpoint, `[{"facet_id":"b"}]`),
		receipt:   put(t, dir, ops.KindReceipt, `{"watermark":3,"served_hash":"sha256:x"}`),
		served:    put(t, dir, ops.KindServed, `[]`),
		selection: put(t, dir, ops.KindSelection, `pick(E) :- entry(E, _, _, _, _, _, _).`),
		code:      put(t, dir, ops.KindSelectionCode, `{"code":"0123456789","hash":"sha256:0123456789"}`),
	}
}

func counts(res artifactdir.Result) map[string]artifactdir.Counts {
	m := map[string]artifactdir.Counts{}
	for _, c := range res.Counts {
		m[c.Kind] = c
	}
	return m
}

func open(t *testing.T, dir string) *artifactdir.Folder {
	t.Helper()
	f, err := artifactdir.Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f
}

// The move loads every receipt, served value, selection and code, and the checkpoint
// values the log names; it reports counts by kind and checks each file as it goes.
func TestMove_LoadsEveryKindAndReportsCounts(t *testing.T) {
	ctx := context.Background()
	old := newOldFolder(t)
	db := store(t)
	if err := db.AppendCheckpoint(ctx, ops.CheckpointRow{SpanID: "bucket:2026", V: 1, CoversW: 1, ValueHash: old.named, MintedAt: "t"}); err != nil {
		t.Fatal(err)
	}

	f := open(t, old.dir)
	res, err := f.Verify(ctx)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	c := counts(res)
	if c[ops.KindCheckpoint].Files != 2 || c[ops.KindReceipt].Files != 1 || c[ops.KindServed].Files != 1 ||
		c[ops.KindSelection].Files != 1 || c[ops.KindSelectionCode].Files != 1 {
		t.Fatalf("Verify counted %+v", c)
	}
	if res.Total().Files != 6 || res.Total().Bytes == 0 || res.Total().Loaded != 0 {
		t.Fatalf("Verify's total %+v: six files, some bytes, none loaded", res.Total())
	}
	if digests, _ := db.List(ctx, ops.KindReceipt); len(digests) != 0 {
		t.Fatal("Verify wrote a row")
	}

	if err := f.Load(ctx, db, &res); err != nil {
		t.Fatalf("Load: %v", err)
	}
	c = counts(res)
	if c[ops.KindCheckpoint].Loaded != 1 || c[ops.KindCheckpoint].Unused != 1 {
		t.Errorf("checkpoints %+v, want the named one loaded and the other left", c[ops.KindCheckpoint])
	}
	for _, kind := range []string{ops.KindReceipt, ops.KindServed, ops.KindSelection, ops.KindSelectionCode} {
		if c[kind].Loaded != 1 {
			t.Errorf("%s %+v, want its file loaded", kind, c[kind])
		}
	}
	for kind, digest := range map[string]string{
		ops.KindCheckpoint: old.named, ops.KindReceipt: old.receipt, ops.KindServed: old.served,
		ops.KindSelection: old.selection, ops.KindSelectionCode: old.code,
	} {
		if _, err := db.Get(ctx, kind, digest); err != nil {
			t.Errorf("%s %s is not in the store, or does not check: %v", kind, digest, err)
		}
	}
	if _, err := db.Get(ctx, ops.KindCheckpoint, old.unnamed); !errors.Is(err, ops.ErrNotFound) {
		t.Errorf("a checkpoint value the log does not name was loaded: %v", err)
	}
}

// Running it again loads nothing twice.
func TestMove_AgainFindsTheRowsThere(t *testing.T) {
	ctx := context.Background()
	old := newOldFolder(t)
	db := store(t)
	for pass := 0; pass < 2; pass++ {
		f := open(t, old.dir)
		res, err := f.Verify(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if err := f.Load(ctx, db, &res); err != nil {
			t.Fatalf("pass %d: %v", pass, err)
		}
		c := counts(res)[ops.KindReceipt]
		want := artifactdir.Counts{Kind: ops.KindReceipt, Files: 1, Bytes: c.Bytes}
		if pass == 0 {
			want.Loaded = 1
		} else {
			want.Present = 1
		}
		if c != want {
			t.Errorf("pass %d: receipts %+v, want %+v", pass, c, want)
		}
	}
}

// A file whose bytes do not hash to its name refuses the whole move, names the file,
// and leaves the store with nothing: the others are not carried in around it.
func TestMove_RefusesAFileWhoseBytesDoNotMatchItsName(t *testing.T) {
	for _, kind := range []string{ops.KindCheckpoint, ops.KindReceipt, ops.KindServed, ops.KindSelection, ops.KindSelectionCode} {
		t.Run(kind, func(t *testing.T) {
			ctx := context.Background()
			old := newOldFolder(t)
			digest := map[string]string{
				ops.KindCheckpoint: old.unnamed, ops.KindReceipt: old.receipt, ops.KindServed: old.served,
				ops.KindSelection: old.selection, ops.KindSelectionCode: old.code,
			}[kind]
			name := filepath.Join(old.dir, kind, "sha256-"+strings.TrimPrefix(digest, "sha256:")+".json")
			writeFile(t, name, "altered after it was named")

			f := open(t, old.dir)
			_, err := f.Verify(ctx)
			if !errors.Is(err, artifactdir.ErrMismatch) {
				t.Fatalf("Verify = %v, want ErrMismatch", err)
			}
			if !strings.Contains(err.Error(), filepath.Join(kind, "sha256-"+strings.TrimPrefix(digest, "sha256:"))) ||
				!strings.Contains(err.Error(), "nothing was moved") {
				t.Errorf("the error does not name the file and say nothing moved: %v", err)
			}
			db := store(t)
			for _, k := range ops.Kinds() {
				if got, _ := db.List(ctx, k); len(got) != 0 {
					t.Errorf("%s holds %d rows after a refused move", k, len(got))
				}
			}
		})
	}
}

// Anything in the folder that is not a kind's directory or a digest-named file stops
// the move, since it cannot say that what it skipped is not worth keeping.
func TestOpen_RefusesWhatItDoesNotRecognise(t *testing.T) {
	for name, path := range map[string]string{
		"a stray file":       "notes.txt",
		"an unknown kind":    "projection/sha256-" + strings.Repeat("a", 64) + ".json",
		"a misnamed file":    "receipt/latest.json",
		"a short digest":     "receipt/sha256-abc.json",
		"a file at the top":  "sha256-" + strings.Repeat("a", 64) + ".json",
		"a nested directory": "receipt/old/sha256-" + strings.Repeat("a", 64) + ".json",
	} {
		t.Run(name, func(t *testing.T) {
			old := newOldFolder(t)
			writeFile(t, filepath.Join(old.dir, path), "x")
			if _, err := artifactdir.Open(old.dir); !errors.Is(err, artifactdir.ErrUnexpected) {
				t.Fatalf("Open = %v, want ErrUnexpected", err)
			}
		})
	}
}

// A write that died leaves <name>.<16 hex>.tmp, which was never a whole file.
func TestOpen_IgnoresTemporaryFilesFromAWriteThatDied(t *testing.T) {
	ctx := context.Background()
	old := newOldFolder(t)
	writeFile(t, filepath.Join(old.dir, ops.KindReceipt, "sha256-"+strings.Repeat("b", 64)+".json.0123456789abcdef.tmp"), "short")
	f := open(t, old.dir)
	res, err := f.Verify(ctx)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if res.Leftovers != 1 || res.Total().Files != 6 {
		t.Errorf("result %+v: want one leftover ignored and the six files", res)
	}
}

func TestOpen_AFolderThatIsNotThereIsAnError(t *testing.T) {
	if _, err := artifactdir.Open(filepath.Join(t.TempDir(), "not-here")); err == nil {
		t.Fatal("Open of a missing folder succeeded")
	}
}

func TestMove_AnEmptyFolderMovesNothing(t *testing.T) {
	ctx := context.Background()
	f := open(t, t.TempDir())
	res, err := f.Verify(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Load(ctx, store(t), &res); err != nil {
		t.Fatal(err)
	}
	if res.Total().Files != 0 {
		t.Errorf("an empty folder reports %+v", res.Total())
	}
}
