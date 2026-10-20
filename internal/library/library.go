// Package library is Anam's store of document bytes (cards 102, 150): files named by
// their sha256, written only by Anam, found by that name. A record.document facet names
// a document's bytes by sha256; where in the library the file sits is this package's
// own business, decided when it is written and never changed.
package library

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"strings"
)

// maxDepth bounds how deep placement goes: a sha256 has 64 characters, so no folder
// name along its path is longer. It follows from the name, so it is not a choice
// (card 151); library_folder_limit is.
const maxDepth = 64

// Store writes body as <sum><ext> in the library, confined to it (os.Root),
// through a temporary file renamed into place, so the name holds the whole file or
// nothing. It reports whether the bytes are new; bytes the library already holds,
// under any extension, are not written again.
//
// A new file goes in the root until the root holds limit files, then into the
// subfolder named by the first character of its sha256, then the first two inside
// that, and so on: each folder along the way named sum[:depth]. A stored file never
// moves, so where it went depends on when it came; Find looks it up by name.
func Store(library string, limit int, sum, ext string, body []byte) (bool, error) {
	if err := os.MkdirAll(library, 0o750); err != nil {
		return false, fmt.Errorf("library: %w", err)
	}
	root, err := os.OpenRoot(library)
	if err != nil {
		return false, fmt.Errorf("library: %w", err)
	}
	defer func() { _ = root.Close() }()

	switch _, err := find(root, sum); {
	case err == nil:
		return false, nil
	case !errors.Is(err, fs.ErrNotExist):
		return false, err
	}
	dir, err := place(root, limit, sum)
	if err != nil {
		return false, err
	}
	var tag [8]byte
	if _, err := rand.Read(tag[:]); err != nil {
		return false, err
	}
	tmp := path.Join(dir, ".incoming-"+hex.EncodeToString(tag[:]))
	name := path.Join(dir, sum+ext)
	f, err := root.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o640)
	if err != nil {
		return false, fmt.Errorf("library: %w", err)
	}
	_, werr := f.Write(body)
	serr := f.Sync()
	cerr := f.Close()
	if err := errors.Join(werr, serr, cerr); err != nil {
		_ = root.Remove(tmp)
		return false, fmt.Errorf("library %s: %w", name, err)
	}
	if err := root.Rename(tmp, name); err != nil {
		_ = root.Remove(tmp)
		return false, fmt.Errorf("library %s: %w", name, err)
	}
	return true, nil
}

// place is the folder a new document named sum goes in: the first folder
// along root, sum[:1], sum[:1]/sum[:2], ... holding fewer than limit files, made if
// it is not there yet. Subfolders do not count against the limit.
func place(root *os.Root, limit int, sum string) (string, error) {
	dir := "."
	for depth := 1; depth <= maxDepth; depth++ {
		files, err := countFiles(root, dir)
		if err != nil {
			return "", fmt.Errorf("library %s: %w", dir, err)
		}
		if files < limit {
			break
		}
		dir = path.Join(dir, sum[:depth])
	}
	if dir != "." {
		if err := root.MkdirAll(dir, 0o750); err != nil {
			return "", fmt.Errorf("library %s: %w", dir, err)
		}
	}
	return dir, nil
}

// countFiles is how many documents dir holds directly: regular files, not
// subfolders and not dot-files. A folder not made yet holds none.
func countFiles(root *os.Root, dir string) (int, error) {
	entries, err := fs.ReadDir(root.FS(), dir)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	n := 0
	for _, e := range entries {
		if e.Type().IsRegular() && !strings.HasPrefix(e.Name(), ".") {
			n++
		}
	}
	return n, nil
}

// Find is the library-relative name of the file holding the bytes whose sha256 is
// sum: a regular file named sum, or sum and an extension, anywhere in the library.
// The sha256 is unique and Anam never renames a stored file, so the name is enough.
// fs.ErrNotExist when there is none.
func Find(library, sum string) (string, error) {
	root, err := os.OpenRoot(library)
	if err != nil {
		return "", fmt.Errorf("library: %w", err)
	}
	defer func() { _ = root.Close() }()
	return find(root, sum)
}

func find(root *os.Root, sum string) (string, error) {
	held, err := index(root)
	if err != nil {
		return "", err
	}
	if name, ok := held[sum]; ok {
		return name, nil
	}
	return "", fmt.Errorf("library: holds no %s: %w", sum, fs.ErrNotExist)
}

// Index maps each sha256 the library holds to the file holding it, by one walk. A
// file not named by a sha256 is not a stored document and is passed over, as are
// dot-files and dot-directories (temporaries on their way in).
func Index(library string) (map[string]string, error) {
	root, err := os.OpenRoot(library)
	if err != nil {
		return nil, fmt.Errorf("library: %w", err)
	}
	defer func() { _ = root.Close() }()
	return index(root)
}

func index(root *os.Root) (map[string]string, error) {
	held := map[string]string{}
	err := fs.WalkDir(root.FS(), ".", func(p string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p != "." && strings.HasPrefix(e.Name(), ".") {
			if e.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if !e.Type().IsRegular() {
			return nil
		}
		if sum, _, _ := strings.Cut(e.Name(), "."); IsSHA256(sum) {
			held[sum] = p
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("library: %w", err)
	}
	return held, nil
}

// IsSHA256 reports whether s is a sha256 as lower-case hex.
func IsSHA256(s string) bool {
	if len(s) != sha256.Size*2 {
		return false
	}
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}
