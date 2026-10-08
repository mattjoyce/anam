package artifact_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/store/artifact"
)

func newStore(t *testing.T) (*artifact.Store, string) {
	t.Helper()
	dir := t.TempDir()
	s, err := artifact.New(dir)
	if err != nil {
		t.Fatalf("artifact.New: %v", err)
	}
	return s, dir
}

// The path shape is part of the contract: a receipt names a served value by digest,
// and a person restoring a backup has to be able to find the file by hand.
func TestPut_WritesContentAddressedPath(t *testing.T) {
	t.Parallel()
	s, dir := newStore(t)

	body := []byte(`{"hello":"world"}`)
	digest, err := s.Put("served", body)
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if digest != artifact.Digest(body) {
		t.Errorf("Put returned %s, want %s", digest, artifact.Digest(body))
	}

	want := filepath.Join(dir, "served", strings.Replace(digest, "sha256:", "sha256-", 1)+".json")
	got, err := os.ReadFile(want) //nolint:gosec // test-owned temp path
	if err != nil {
		t.Fatalf("read %s: %v", want, err)
	}
	if string(got) != string(body) {
		t.Errorf("stored %q, want %q", got, body)
	}
}

// Identical bytes have an identical name, so a re-put is a no-op rather than a second
// file. Serving the same projection twice must not litter the tier.
func TestPut_Idempotent(t *testing.T) {
	t.Parallel()
	s, dir := newStore(t)

	body := []byte(`{"a":1}`)
	first, err := s.Put("served", body)
	if err != nil {
		t.Fatalf("first Put: %v", err)
	}
	second, err := s.Put("served", body)
	if err != nil {
		t.Fatalf("second Put: %v", err)
	}
	if first != second {
		t.Errorf("Put twice returned %s then %s", first, second)
	}

	entries, err := os.ReadDir(filepath.Join(dir, "served"))
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("re-putting identical bytes left %d files, want 1", len(entries))
	}
}

// A file that exists holding different bytes is a corruption of the tier, not an
// update: content addressing means the name could only match if the bytes did.
func TestPut_RefusesDifferentBytesAtSameAddress(t *testing.T) {
	t.Parallel()
	s, dir := newStore(t)

	body := []byte(`{"a":1}`)
	digest, err := s.Put("served", body)
	if err != nil {
		t.Fatalf("Put: %v", err)
	}

	path := filepath.Join(dir, "served", strings.Replace(digest, "sha256:", "sha256-", 1)+".json")
	if err := os.WriteFile(path, []byte(`{"a":2}`), 0o600); err != nil {
		t.Fatalf("tamper: %v", err)
	}

	if _, err := s.Put("served", body); !errors.Is(err, artifact.ErrCorrupt) {
		t.Errorf("Put over tampered file: err = %v, want ErrCorrupt", err)
	}
}

// Published artifacts are one person's health facts: owner-only files, owner-plus-group
// directories, matching what the operational store already creates.
func TestPut_Modes(t *testing.T) {
	t.Parallel()
	s, dir := newStore(t)

	digest, err := s.Put("receipt", []byte(`{}`))
	if err != nil {
		t.Fatalf("Put: %v", err)
	}

	fi, err := os.Stat(filepath.Join(dir, "receipt", strings.Replace(digest, "sha256:", "sha256-", 1)+".json"))
	if err != nil {
		t.Fatalf("Stat file: %v", err)
	}
	if got := fi.Mode().Perm(); got != 0o600 {
		t.Errorf("file mode %o, want 600", got)
	}

	di, err := os.Stat(filepath.Join(dir, "receipt"))
	if err != nil {
		t.Fatalf("Stat dir: %v", err)
	}
	if got := di.Mode().Perm(); got != 0o750 {
		t.Errorf("dir mode %o, want 750", got)
	}
}

// Get verifies on the way out; every read of this tier is a read of something whose
// integrity is the point.
func TestGet_DetectsTamperedBytes(t *testing.T) {
	t.Parallel()
	s, dir := newStore(t)

	body := []byte(`{"reading":"137.0"}`)
	digest, err := s.Put("served", body)
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if got, err := s.Get("served", digest); err != nil || string(got) != string(body) {
		t.Fatalf("Get before tamper: %q, %v", got, err)
	}

	path := filepath.Join(dir, "served", strings.Replace(digest, "sha256:", "sha256-", 1)+".json")
	if err := os.WriteFile(path, []byte(`{"reading":"138.0"}`), 0o600); err != nil {
		t.Fatalf("tamper: %v", err)
	}

	if _, err := s.Get("served", digest); !errors.Is(err, artifact.ErrCorrupt) {
		t.Errorf("Get after tamper: err = %v, want ErrCorrupt", err)
	}
	// Raw is the verifier's read: it hands back what is actually there so the caller
	// can report how it differs, rather than only that it does.
	raw, err := s.Raw("served", digest)
	if err != nil {
		t.Fatalf("Raw after tamper: %v", err)
	}
	if string(raw) != `{"reading":"138.0"}` {
		t.Errorf("Raw returned %q, want the tampered bytes", raw)
	}
}

func TestGet_NotFound(t *testing.T) {
	t.Parallel()
	s, _ := newStore(t)

	absent := artifact.Digest([]byte("never stored"))
	if _, err := s.Get("served", absent); !errors.Is(err, artifact.ErrNotFound) {
		t.Errorf("Get absent: err = %v, want ErrNotFound", err)
	}
}

// Size is the stored length, and a missing artifact is ErrNotFound as it is for Get.
func TestSize(t *testing.T) {
	t.Parallel()
	s, _ := newStore(t)

	b := []byte(`{"n": 1}`)
	digest, err := s.Put("served", b)
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if n, err := s.Size("served", digest); err != nil || n != int64(len(b)) {
		t.Errorf("Size = %d, %v; want %d", n, err, len(b))
	}
	if _, err := s.Size("served", artifact.Digest([]byte("never stored"))); !errors.Is(err, artifact.ErrNotFound) {
		t.Errorf("Size absent: err = %v, want ErrNotFound", err)
	}
}

// Digests reach this package from a command line. A name that cannot contain a
// separator cannot escape the store, so the shape is checked before any path is built.
func TestPath_RejectsMalformedNames(t *testing.T) {
	t.Parallel()
	s, _ := newStore(t)

	bad := []struct{ kind, digest string }{
		{"served", "../../etc/passwd"},
		{"served", "sha256:xyz"},
		{"served", "sha256:" + strings.Repeat("a", 63)},
		{"served", ""},
		{"../escape", artifact.Digest(nil)},
		{"Served", artifact.Digest(nil)},
		{"", artifact.Digest(nil)},
	}
	for _, c := range bad {
		if _, err := s.Get(c.kind, c.digest); err == nil {
			t.Errorf("Get(%q, %q) was accepted", c.kind, c.digest)
		} else if errors.Is(err, artifact.ErrNotFound) {
			t.Errorf("Get(%q, %q) reached the filesystem (got ErrNotFound); it should be refused by shape", c.kind, c.digest)
		}
	}
}

// Nothing published yet is a state, not a fault.
func TestList_AbsentKindIsEmpty(t *testing.T) {
	t.Parallel()
	s, _ := newStore(t)

	got, err := s.List("receipt")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("List of an unwritten kind = %v, want empty", got)
	}
}

func TestList_ReturnsStoredDigests(t *testing.T) {
	t.Parallel()
	s, _ := newStore(t)

	want := map[string]bool{}
	for _, body := range [][]byte{[]byte(`{"a":1}`), []byte(`{"b":2}`), []byte(`{"c":3}`)} {
		d, err := s.Put("receipt", body)
		if err != nil {
			t.Fatalf("Put: %v", err)
		}
		want[d] = true
	}

	got, err := s.List("receipt")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("List returned %d digests, want %d", len(got), len(want))
	}
	for _, d := range got {
		if !want[d] {
			t.Errorf("List returned unexpected digest %s", d)
		}
	}
	for i := 1; i < len(got); i++ {
		if got[i-1] >= got[i] {
			t.Errorf("List is not sorted: %s before %s", got[i-1], got[i])
		}
	}
}
