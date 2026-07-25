package def

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
)

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
	entries, err := manifestFS.ReadDir("manifests")
	if err != nil {
		return "", fmt.Errorf("def: read bundled manifests: %w", err)
	}

	files := make(map[string][]byte, len(entries))
	for _, e := range entries {
		raw, err := manifestFS.ReadFile("manifests/" + e.Name())
		if err != nil {
			return "", fmt.Errorf("def: read bundled manifest %s: %w", e.Name(), err)
		}
		files[e.Name()] = raw
	}
	return setVersionOf(files), nil
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
