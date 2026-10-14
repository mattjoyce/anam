package library

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"testing"
)

// A full folder sends a new document a level deeper, into sha256-prefix subfolders,
// and every stored document is still found by its name, wherever it went.
func TestStorePlacesByPrefixAndFindFindsByName(t *testing.T) {
	library := t.TempDir()
	const limit = 2
	var sums []string
	for i := range 12 {
		body := fmt.Sprintf("document %d", i)
		s := sha256.Sum256([]byte(body))
		sum := hex.EncodeToString(s[:])
		sums = append(sums, sum)
		if isNew, err := Store(library, limit, sum, ".txt", []byte(body)); err != nil || !isNew {
			t.Fatalf("store %d: new %v, %v", i, isNew, err)
		}
	}
	root, err := os.OpenRoot(library)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	deep := 0
	for i, sum := range sums {
		name, err := Find(library, sum)
		if err != nil {
			t.Fatalf("document %d not found: %v", i, err)
		}
		dirs := strings.Split(name, "/")
		for depth, dir := range dirs[:len(dirs)-1] {
			if dir != sum[:depth+1] {
				t.Errorf("document %d is at %s: folder %q is not its sha256's first %d", i, name, dir, depth+1)
			}
		}
		if len(dirs) > 1 {
			deep++
		}
		if isNew, err := Store(library, limit, sum, ".txt", []byte(fmt.Sprintf("document %d", i))); err != nil || isNew {
			t.Errorf("document %d again: new %v, %v", i, isNew, err)
		}
	}
	if deep == 0 {
		t.Error("no document went below the root, with the root full at 2")
	}
	err = fs.WalkDir(root.FS(), ".", func(p string, e fs.DirEntry, err error) error {
		if err == nil && e.IsDir() {
			if n, _ := countFiles(root, p); n > limit {
				t.Errorf("%s holds %d files, over the limit of %d", p, n, limit)
			}
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

// A document's bytes are stored once, whatever extension they arrive with, and a
// name that is not a sha256 is not a stored document.
func TestStoreKeepsOneCopyAndIndexSkipsOtherFiles(t *testing.T) {
	library := t.TempDir()
	body := []byte("%PDF-1.4 one letter")
	s := sha256.Sum256(body)
	sum := hex.EncodeToString(s[:])
	if isNew, err := Store(library, 1000, sum, ".pdf", body); err != nil || !isNew {
		t.Fatalf("store: new %v, %v", isNew, err)
	}
	if isNew, err := Store(library, 1000, sum, "", body); err != nil || isNew {
		t.Fatalf("the same bytes without an extension: new %v, %v", isNew, err)
	}
	for _, other := range []string{"letters/letter.pdf", "letters/letter.pdf.meta.yaml", ".incoming-0011"} {
		if err := os.MkdirAll(library+"/letters", 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(library+"/"+other, body, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	held, err := Index(library)
	if err != nil {
		t.Fatal(err)
	}
	if len(held) != 1 || held[sum] != sum+".pdf" {
		t.Errorf("index = %v, want only %s.pdf", held, sum)
	}
	if _, err := Find(library, strings.Repeat("0", 64)); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("find of bytes never stored: %v, want fs.ErrNotExist", err)
	}
}
