package artifactdir_test

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/artifactdir"
	"github.com/mattjoyce/anam/internal/store/ops"
)

// Card 260: schema 10 names the template body kind template-body; a folder an older
// build wrote names its directory inline-projection. Both are read, checked against
// their names and filed under template-body, and the folder keeps its own names.

const (
	inlineBody  = `{"max_classification":"Normal","statuses":["admitted"],"types":["health.weight"]}`
	catalogBody = `{"name":"weights","version":1,"types":["health.weight"]}`
)

// snapshot is every file under dir with its bytes, so a test can show the folder was
// left as it was.
func snapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(p) //nolint:gosec // a path under the test's own directory
		out[p] = string(b)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func sameFiles(t *testing.T, before, after map[string]string) {
	t.Helper()
	if len(before) != len(after) {
		t.Fatalf("the folder held %d files and holds %d", len(before), len(after))
	}
	for p, b := range before {
		if after[p] != b {
			t.Errorf("%s changed", p)
		}
	}
}

func TestLegacy_AnOlderFoldersTemplateBodiesAreFiledUnderTheNewName(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dir := t.TempDir()
	inline := put(t, dir, "inline-projection", inlineBody)
	catalog := put(t, dir, ops.KindTemplateBody, catalogBody)
	before := snapshot(t, dir)

	f := open(t, dir)
	res, err := f.Verify(ctx)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if c := counts(res)[ops.KindTemplateBody]; c.Files != 2 {
		t.Fatalf("template-body counted %+v, want both folders' files", c)
	}
	if _, listed := counts(res)["inline-projection"]; listed || res.Legacy["inline-projection"] != 1 || res.Twins != 0 {
		t.Fatalf("the older name is reported as %+v, legacy %v, twins %d", counts(res), res.Legacy, res.Twins)
	}
	db := store(t)
	if err := f.Load(ctx, db, &res); err != nil {
		t.Fatalf("Load: %v", err)
	}
	for _, d := range []string{inline, catalog} {
		if b, err := db.Get(ctx, ops.KindTemplateBody, d); err != nil || ops.Digest(b) != d {
			t.Errorf("template body %s: %v", d, err)
		}
	}
	sameFiles(t, before, snapshot(t, dir))
}

// The same digest under both names is checked in both directories and stored once;
// the copy under the older name is found already there.
func TestLegacy_TheSameBodyUnderBothNamesIsStoredOnce(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dir := t.TempDir()
	d := put(t, dir, ops.KindTemplateBody, inlineBody)
	if got := put(t, dir, "inline-projection", inlineBody); got != d {
		t.Fatal("one body, two digests")
	}
	f := open(t, dir)
	res, err := f.Verify(ctx)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if res.Twins != 1 {
		t.Fatalf("twins = %d, want 1", res.Twins)
	}
	db := store(t)
	if err := f.Load(ctx, db, &res); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c := counts(res)[ops.KindTemplateBody]; c.Files != 2 || c.Loaded != 1 || c.Present != 1 {
		t.Errorf("template-body %+v, want two files, one loaded, one already there", c)
	}
	if got, _ := db.List(ctx, ops.KindTemplateBody); len(got) != 1 {
		t.Errorf("%d rows for one body", len(got))
	}
}

// A file under the older name whose bytes are not what its name says refuses the whole
// move, whether or not the same name under the new directory holds the right bytes.
func TestLegacy_AMismatchUnderEitherNameRefusesTheMove(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct{ good, bad string }{
		"older name bad":   {ops.KindTemplateBody, "inline-projection"},
		"current name bad": {"inline-projection", ops.KindTemplateBody},
		"older name alone": {"", "inline-projection"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			dir := t.TempDir()
			d := ops.Digest([]byte(inlineBody))
			file := "sha256-" + strings.TrimPrefix(d, "sha256:") + ".json"
			if tc.good != "" {
				writeFile(t, filepath.Join(dir, tc.good, file), inlineBody)
			}
			writeFile(t, filepath.Join(dir, tc.bad, file), catalogBody)
			before := snapshot(t, dir)

			f := open(t, dir)
			_, err := f.Verify(ctx)
			if !errors.Is(err, artifactdir.ErrMismatch) || !strings.Contains(err.Error(), tc.bad+"/"+file) {
				t.Fatalf("Verify = %v, want a mismatch naming %s/%s", err, tc.bad, file)
			}
			sameFiles(t, before, snapshot(t, dir))
		})
	}
}

// The older name is a folder's spelling, not a kind. The store refuses it, so nothing
// can file a row under it.
func TestLegacy_TheStoreKnowsOnlyTheCurrentName(t *testing.T) {
	t.Parallel()
	db := store(t)
	if _, err := db.Put(context.Background(), "inline-projection", []byte(inlineBody)); !errors.Is(err, ops.ErrInvalid) {
		t.Fatalf("Put under inline-projection = %v, want ErrInvalid", err)
	}
	for _, k := range ops.Kinds() {
		if k == "inline-projection" {
			t.Fatal("inline-projection is still a kind")
		}
	}
}

// A file that changes between Verify and Load is refused before anything is stored for
// it, under either name: neither its old digest nor its new one gets a row.
func TestLoad_AFileChangedSinceVerifyStoresNothing(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"inline-projection", ops.KindTemplateBody} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			dir := t.TempDir()
			named := put(t, dir, kind, inlineBody)
			f := open(t, dir)
			res, err := f.Verify(ctx)
			if err != nil {
				t.Fatalf("Verify: %v", err)
			}
			changed := []byte(catalogBody)
			file := filepath.Join(dir, kind, "sha256-"+strings.TrimPrefix(named, "sha256:")+".json")
			if err := os.WriteFile(file, changed, 0o600); err != nil {
				t.Fatal(err)
			}
			db := store(t)
			if err := f.Load(ctx, db, &res); !errors.Is(err, artifactdir.ErrMismatch) {
				t.Fatalf("Load = %v, want ErrMismatch", err)
			}
			for _, d := range []string{named, ops.Digest(changed)} {
				if _, err := db.Get(ctx, ops.KindTemplateBody, d); !errors.Is(err, ops.ErrNotFound) {
					t.Errorf("template body %s after the refused load: %v, want not found", d, err)
				}
			}
		})
	}
}
