package def

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"sort"
)

// SetVersionFS is the content address of a catalog root's current manifests: the
// SHA-256 over every *.json file's name and bytes in name order. Earlier versions
// under history/ do not contribute.
func SetVersionFS(fsys fs.FS) (string, error) {
	names, err := readJSONFiles(fsys, ".")
	if err != nil {
		return "", fmt.Errorf("def: read bundled manifests: %w", err)
	}

	files := make(map[string][]byte, len(names))
	for _, name := range names {
		raw, err := fs.ReadFile(fsys, name)
		if err != nil {
			return "", fmt.Errorf("def: read bundled manifest %s: %w", name, err)
		}
		files[name] = raw
	}
	return setVersionOf(files), nil
}

// SetVersion is the content address of the whole bundled definition set: the SHA-256
// over every embedded manifest's name and bytes, in name order.
//
// A receipt records it (§9) so that a served value can be read back years later
// alongside the interpretation that was in force when its facets were captured. It is
// a hash rather than a number because there is no release process to hand out numbers
// and no place to keep a counter that would not itself need versioning — the manifests
// are the version, and hashing them says so without a second source of truth.
//
// Note what it does *not* gate. Definitions govern capture, so a definition set that
// has moved on cannot change bytes already in the ledger; a mismatch at verification
// time is provenance worth reporting, never a reason to call a reproduction failed.
func SetVersion() (string, error) {
	return SetVersionFS(embedded())
}

// setVersionOf hashes a manifest set. Each entry contributes its name and its bytes,
// both length-prefixed, so that renaming a manifest — or moving bytes from one file to
// the next — cannot leave the digest unchanged.
func setVersionOf(files map[string][]byte) string {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)

	h := sha256.New()
	for _, name := range names {
		_, _ = fmt.Fprintf(h, "%d:%s\n%d:", len(name), name, len(files[name]))
		_, _ = h.Write(files[name])
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}
