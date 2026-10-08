// Package artifact is the filesystem half of Anam's store: the published-artifact
// tier (ARCHITECTURE.md §8).
//
// It holds what Anam has *published* — served projection values and their receipts
// now, minted checkpoint values from R8 — each addressed by the SHA-256 of its own
// bytes. Content addressing is doing real work here rather than decorating: a served
// value's name is the hash the receipt already had to record, so the receipt is
// simultaneously the pointer and the integrity check, and the two cannot drift.
//
// Every path is resolved through an os.Root scoped to the artifact directory, so a
// name that tried to reach outside it is refused by the kernel rather than by a check
// this package remembered to run. Digests arrive from a command line, and the
// alternative — validate the string, join it, and vouch for the result in a comment —
// is the shape of every path-traversal bug ever written. The syntactic check stays as
// well, because it produces a better error and it costs nothing; it is defence in
// depth, not the defence.
//
// This tier is *authored*: it comes back from a backup or from nowhere. Unlike the
// operational store's derived tables it cannot be rebuilt by replaying the ledger,
// because what Anam showed is not something the ledger records (§9). Back it up.
package artifact

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path"
	"regexp"
	"sort"
	"strings"
	"sync"
)

// ErrNotFound is returned by Get for a digest with no file.
var ErrNotFound = errors.New("artifact: not found")

// ErrInvalid is returned for a kind or digest that cannot name an artifact.
var ErrInvalid = errors.New("artifact: invalid name")

// ErrCorrupt is returned when a stored file's bytes do not hash to the digest that
// names it. Content addressing makes this detectable, so it is detected rather than
// assumed away.
var ErrCorrupt = errors.New("artifact: stored bytes do not match their content address")

// The file and directory modes. These are one person's published health facts, so the
// files are owner-only and the directories owner-plus-group, matching what the
// operational store already creates.
const (
	fileMode = 0o600
	dirMode  = 0o750
)

// digestPattern matches the canonical "sha256:<64 hex>" form.
var digestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// kindPattern matches a subdirectory name. Kinds are compile-time constants in this
// codebase, but the same reasoning applies — nothing becomes a path element unchecked.
var kindPattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// Store is a handle on a content-addressed directory tree.
type Store struct {
	dir string

	// root is opened on first write and reused. It is lazy because `anam version`
	// should not conjure a data directory as a side effect, and because the directory
	// legitimately does not exist until something is published.
	mu   sync.Mutex
	root *os.Root
}

// New returns a Store rooted at dir.
func New(dir string) (*Store, error) {
	if dir == "" {
		return nil, fmt.Errorf("artifact: root directory is required")
	}
	return &Store{dir: dir}, nil
}

// Root reports the directory the store is rooted at.
func (s *Store) Root() string { return s.dir }

// Close releases the root handle, if one was opened.
func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.root == nil {
		return nil
	}
	err := s.root.Close()
	s.root = nil
	return err
}

// Digest is the content address of b: "sha256:<hex>", the same spelling the fold uses
// for a canonical value's hash, so the two are directly comparable.
func Digest(b []byte) string {
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// Put writes b under kind, addressed by its own digest, and returns that digest.
//
// It is idempotent by construction: identical bytes have an identical name, so a
// re-put finds the file already there and writes nothing. A file that exists holding
// *different* bytes is a corruption of the tier, not an update, and is refused —
// content addressing means the name could only match if the bytes did.
func (s *Store) Put(kind string, b []byte) (string, error) {
	digest := Digest(b)
	name, err := entryName(kind, digest)
	if err != nil {
		return "", err
	}
	root, err := s.openRoot(true)
	if err != nil {
		return "", err
	}

	switch existing, err := root.ReadFile(name); {
	case err == nil && bytes.Equal(existing, b):
		return digest, nil
	case err == nil:
		return "", fmt.Errorf("%w: %s/%s holds bytes that hash elsewhere", ErrCorrupt, s.dir, name)
	case !errors.Is(err, os.ErrNotExist):
		return "", fmt.Errorf("artifact: read %s: %w", name, err)
	}

	if err := root.MkdirAll(kind, dirMode); err != nil {
		return "", fmt.Errorf("artifact: create %s dir: %w", kind, err)
	}
	if err := root.WriteFile(name, b, fileMode); err != nil {
		return "", fmt.Errorf("artifact: write %s: %w", name, err)
	}
	return digest, nil
}

// Get returns the bytes stored under kind at digest, verifying on the way out that
// they still hash to the name they are filed under. There is no caller who wanted the
// bytes regardless: every ordinary read of this tier is a read of something whose
// integrity is the point.
func (s *Store) Get(kind, digest string) ([]byte, error) {
	b, err := s.Raw(kind, digest)
	if err != nil {
		return nil, err
	}
	if got := Digest(b); got != digest {
		return nil, fmt.Errorf("%w: %s/%s is filed as %s but hashes to %s", ErrCorrupt, s.dir, kind, digest, got)
	}
	return b, nil
}

// Raw returns the stored bytes *without* the integrity check, so a caller can report
// precisely how a tampered artifact differs instead of only that it does. Get is the
// normal read; this exists for the verifier.
func (s *Store) Raw(kind, digest string) ([]byte, error) {
	name, err := entryName(kind, digest)
	if err != nil {
		return nil, err
	}
	root, err := s.openRoot(false)
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("%w: %s %s", ErrNotFound, kind, digest)
	}
	if err != nil {
		return nil, err
	}

	b, err := root.ReadFile(name)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return nil, fmt.Errorf("%w: %s %s", ErrNotFound, kind, digest)
	case err != nil:
		return nil, fmt.Errorf("artifact: read %s: %w", name, err)
	}
	return b, nil
}

// Size returns how many bytes are stored under kind at digest, from the file's
// metadata alone. It does not read or check the bytes: it answers how big an artifact
// is, and a caller that needs the bytes uses Get, which checks them.
func (s *Store) Size(kind, digest string) (int64, error) {
	name, err := entryName(kind, digest)
	if err != nil {
		return 0, err
	}
	root, err := s.openRoot(false)
	if errors.Is(err, os.ErrNotExist) {
		return 0, fmt.Errorf("%w: %s %s", ErrNotFound, kind, digest)
	}
	if err != nil {
		return 0, err
	}
	info, err := root.Stat(name)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return 0, fmt.Errorf("%w: %s %s", ErrNotFound, kind, digest)
	case err != nil:
		return 0, fmt.Errorf("artifact: stat %s: %w", name, err)
	}
	return info.Size(), nil
}

// List returns the digests stored under kind, sorted. An absent directory is not an
// error: nothing published yet is a state, not a fault.
func (s *Store) List(kind string) ([]string, error) {
	if !kindPattern.MatchString(kind) {
		return nil, fmt.Errorf("artifact: %q is not a valid artifact kind", kind)
	}
	root, err := s.openRoot(false)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	dir, err := root.Open(kind)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("artifact: open %s: %w", kind, err)
	}
	defer func() { _ = dir.Close() }()

	names, err := dir.Readdirnames(-1)
	if err != nil {
		return nil, fmt.Errorf("artifact: list %s: %w", kind, err)
	}

	digests := make([]string, 0, len(names))
	for _, n := range names {
		trimmed := strings.TrimSuffix(n, ".json")
		if trimmed == n {
			continue
		}
		if d := "sha256:" + strings.TrimPrefix(trimmed, "sha256-"); digestPattern.MatchString(d) {
			digests = append(digests, d)
		}
	}
	sort.Strings(digests)
	return digests, nil
}

// openRoot returns the store's os.Root, creating the directory when create is set.
// Callers that only read pass false, so a missing directory surfaces as os.ErrNotExist
// rather than being conjured into existence by a read.
func (s *Store) openRoot(create bool) (*os.Root, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.root != nil {
		return s.root, nil
	}
	if create {
		if err := os.MkdirAll(s.dir, dirMode); err != nil {
			return nil, fmt.Errorf("artifact: create %s: %w", s.dir, err)
		}
	}
	root, err := os.OpenRoot(s.dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, err // the caller distinguishes "nothing published" from a fault
		}
		return nil, fmt.Errorf("artifact: open %s: %w", s.dir, err)
	}
	s.root = root
	return root, nil
}

// entryName builds "<kind>/sha256-<hex>.json", the name relative to the store's root.
//
// The colon of the digest becomes a hyphen on disk: a colon is legal in a POSIX
// filename and illegal on Windows, and a published artifact should survive being
// copied to a backup drive formatted anywhere.
func entryName(kind, digest string) (string, error) {
	if !kindPattern.MatchString(kind) {
		return "", fmt.Errorf("%w: %q is not a valid artifact kind", ErrInvalid, kind)
	}
	if !digestPattern.MatchString(digest) {
		return "", fmt.Errorf("%w: %q is not a sha256:<64 hex> digest", ErrInvalid, digest)
	}
	return path.Join(kind, strings.Replace(digest, "sha256:", "sha256-", 1)+".json"), nil
}
