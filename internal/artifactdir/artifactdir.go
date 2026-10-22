// Package artifactdir reads the folder Anam used to keep checkpoint values, receipts,
// served values and saved selections in, so that a store which now keeps them as rows
// can take them in (card 215).
//
// The folder was <kind>/sha256-<hex>.json, each file named by the SHA-256 of its own
// bytes. Moving it changes where the bytes live and nothing about them, so the move
// proves that before it writes: every file is read and hashed, a file whose bytes do
// not hash to its name refuses the whole move, and only then are rows written. A move
// that is refused, or crashes, leaves the folder as it was and the store holding only
// whole rows, and running it again finishes the job, because a row is inserted once.
//
// Every path is resolved through an os.Root scoped to the folder, so a name in it that
// tried to reach outside is refused by the kernel and not by a check remembered here.
package artifactdir

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/mattjoyce/anam/internal/store/ops"
)

// ErrMismatch is returned when a file's bytes do not hash to the digest its name gives.
// Nothing is moved: a file that is not what its name says is a fault to look at, not
// one to carry into a store.
var ErrMismatch = errors.New("artifactdir: file bytes do not match their name")

// ErrUnexpected is returned for an entry in the folder that is no kind's directory or
// no digest-named file. Nothing is moved, since the move cannot say that what it
// skipped is not worth keeping.
var ErrUnexpected = errors.New("artifactdir: unexpected entry")

// Store is what the move needs of the operational store.
type Store interface {
	Put(ctx context.Context, kind string, b []byte) (string, error)
	Size(ctx context.Context, kind, digest string) (int64, error)
	Checkpoints(ctx context.Context) ([]ops.CheckpointRow, error)
}

// Counts is what the folder held of one kind and what the move did with it.
type Counts struct {
	Kind  string
	Files int
	Bytes int64
	// Loaded files became rows; Present were rows already; Unused were not loaded
	// because nothing wants them.
	Loaded, Present, Unused int
}

// Result is a move or the report of one that was not made.
type Result struct {
	Dir    string
	Counts []Counts
	// Leftovers are the temporary files a write that died left behind, which were
	// never a complete file and are ignored.
	Leftovers int
}

// Total sums the counts.
func (r Result) Total() Counts {
	var t Counts
	t.Kind = "total"
	for _, c := range r.Counts {
		t.Files += c.Files
		t.Bytes += c.Bytes
		t.Loaded += c.Loaded
		t.Present += c.Present
		t.Unused += c.Unused
	}
	return t
}

// namePattern matches a file in a kind's directory: sha256-<64 hex>.json.
var namePattern = regexp.MustCompile(`^sha256-([0-9a-f]{64})\.json$`)

// tmpPattern matches the temporary name a write that died left: <name>.<16 hex>.tmp.
var tmpPattern = regexp.MustCompile(`\.[0-9a-f]{16}\.tmp$`)

// Folder is an artifact folder opened for reading. Verify it, then Load it.
type Folder struct {
	dir       string
	root      *os.Root
	files     []file
	leftovers int
}

// Open lists the folder at dir and refuses anything in it that is neither a kind's
// directory nor a digest-named file. It reads no file's bytes.
func Open(dir string) (*Folder, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, fmt.Errorf("artifactdir: open %s: %w", dir, err)
	}
	files, leftovers, err := scan(root)
	if err != nil {
		_ = root.Close()
		return nil, err
	}
	return &Folder{dir: dir, root: root, files: files, leftovers: leftovers}, nil
}

// Close releases the folder.
func (f *Folder) Close() error { return f.root.Close() }

// Verify reads every file and checks its bytes against its name, and reports what the
// folder holds by kind. A file that does not match refuses the move with ErrMismatch.
func (f *Folder) Verify(ctx context.Context) (Result, error) {
	res := Result{Dir: f.dir, Leftovers: f.leftovers}
	counts := map[string]*Counts{}
	for _, k := range ops.Kinds() {
		counts[k] = &Counts{Kind: k}
	}
	var bad []string
	for _, fl := range f.files {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		b, err := f.root.ReadFile(fl.name())
		if err != nil {
			return Result{}, fmt.Errorf("artifactdir: read %s: %w", fl.name(), err)
		}
		if got := ops.Digest(b); got != fl.digest {
			bad = append(bad, fmt.Sprintf("%s hashes to %s", fl.name(), got))
			continue
		}
		c := counts[fl.kind]
		c.Files++
		c.Bytes += int64(len(b))
	}
	if len(bad) > 0 {
		return Result{}, fmt.Errorf("%w: %d of %d files: %s; nothing was moved",
			ErrMismatch, len(bad), len(f.files), listed(bad))
	}
	for _, k := range ops.Kinds() {
		res.Counts = append(res.Counts, *counts[k])
	}
	return res, nil
}

// Load writes the rows into st, and fills in what it did on res, which Verify returned.
// A checkpoint value is loaded only if the checkpoint log names it: values are derived,
// and the log is the judge of which are read. A folder moved into a store whose log was
// rebuilt loads none, and minting writes the ones that are wanted.
//
// Files are read again rather than held, since a folder of checkpoints is far larger
// than the store they come down to. Each is checked against its name once more as it is
// stored, by the store, which keys a row by the hash of the bytes it is given.
func (f *Folder) Load(ctx context.Context, st Store, res *Result) error {
	wanted, err := namedCheckpoints(ctx, st)
	if err != nil {
		return err
	}
	byKind := map[string]*Counts{}
	for i := range res.Counts {
		byKind[res.Counts[i].Kind] = &res.Counts[i]
	}
	for _, fl := range f.files {
		if err := ctx.Err(); err != nil {
			return err
		}
		c := byKind[fl.kind]
		if fl.kind == ops.KindCheckpoint && !wanted[fl.digest] {
			c.Unused++
			continue
		}
		switch _, err := st.Size(ctx, fl.kind, fl.digest); {
		case err == nil:
			c.Present++
			continue
		case !errors.Is(err, ops.ErrNotFound):
			return err
		}
		b, err := f.root.ReadFile(fl.name())
		if err != nil {
			return fmt.Errorf("artifactdir: read %s: %w", fl.name(), err)
		}
		if got, err := st.Put(ctx, fl.kind, b); err != nil {
			return err
		} else if got != fl.digest {
			return fmt.Errorf("%w: %s was read as %s and stored as %s", ErrMismatch, fl.name(), fl.digest, got)
		}
		c.Loaded++
	}
	return nil
}

// file is one file of the folder: its kind and the digest its name gives.
type file struct {
	kind   string
	digest string
}

func (f file) name() string {
	return path.Join(f.kind, "sha256-"+strings.TrimPrefix(f.digest, "sha256:")+".json")
}

// scan lists the folder: every file of every kind, and the count of leftovers. An entry
// that is neither is refused.
func scan(root *os.Root) (files []file, leftovers int, err error) {
	top, err := fs.ReadDir(root.FS(), ".")
	if err != nil {
		return nil, 0, fmt.Errorf("artifactdir: list the folder: %w", err)
	}
	known := map[string]bool{}
	for _, k := range ops.Kinds() {
		known[k] = true
	}
	var unexpected []string
	for _, e := range top {
		if !e.IsDir() || !known[e.Name()] {
			unexpected = append(unexpected, e.Name())
		}
	}
	for _, kind := range ops.Kinds() {
		entries, err := fs.ReadDir(root.FS(), kind)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, 0, fmt.Errorf("artifactdir: list %s: %w", kind, err)
		}
		for _, e := range entries {
			switch m := namePattern.FindStringSubmatch(e.Name()); {
			case e.IsDir():
				unexpected = append(unexpected, path.Join(kind, e.Name()))
			case m != nil:
				files = append(files, file{kind: kind, digest: "sha256:" + m[1]})
			case tmpPattern.MatchString(e.Name()):
				leftovers++
			default:
				unexpected = append(unexpected, path.Join(kind, e.Name()))
			}
		}
	}
	if len(unexpected) > 0 {
		sort.Strings(unexpected)
		return nil, 0, fmt.Errorf("%w: %s; nothing was moved. Remove or move them aside and run again",
			ErrUnexpected, listed(unexpected))
	}
	return files, leftovers, nil
}

// maxListed is how many names an error shows before it says how many more.
const maxListed = 8

func listed(names []string) string {
	if len(names) <= maxListed {
		return strings.Join(names, ", ")
	}
	return fmt.Sprintf("%s and %d more", strings.Join(names[:maxListed], ", "), len(names)-maxListed)
}

// namedCheckpoints is the set of checkpoint values the log names.
func namedCheckpoints(ctx context.Context, st Store) (map[string]bool, error) {
	rows, err := st.Checkpoints(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(rows))
	for _, r := range rows {
		out[r.ValueHash] = true
	}
	return out, nil
}
