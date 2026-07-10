package safepath_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/safepath"
)

func TestExpand(t *testing.T) {
	t.Parallel()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name, in, want string
		wantErr        bool
	}{
		{"tilde slash", "~/data/x.db", filepath.Join(home, "data", "x.db"), false},
		{"bare tilde", "~", home, false},
		{"env var", "$HOME/data/x.db", filepath.Join(home, "data", "x.db"), false},
		{"cleans dots", home + "/a/../b/./x", filepath.Join(home, "b", "x"), false},
		{"empty", "", "", true},
		{"tilde user unsupported", "~alice/x", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := safepath.Expand(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("Expand(%q) = %q, want error", tc.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Expand(%q): %v", tc.in, err)
			}
			if got != tc.want {
				t.Errorf("Expand(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}

	// A relative path becomes absolute against the working directory.
	got, err := safepath.Expand("rel/file")
	if err != nil {
		t.Fatalf("Expand(rel): %v", err)
	}
	if !filepath.IsAbs(got) || !strings.HasSuffix(got, filepath.Join("rel", "file")) {
		t.Errorf("Expand(rel/file) = %q, want absolute ending in rel/file", got)
	}
}

func TestResolveFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	real := filepath.Join(dir, "export.csv")
	if err := os.WriteFile(real, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}

	// A symlink resolves to the file it points at — the returned path is the real one.
	link := filepath.Join(dir, "link.csv")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	got, err := safepath.ResolveFile(link)
	if err != nil {
		t.Fatalf("ResolveFile(symlink): %v", err)
	}
	if resolved, _ := filepath.EvalSymlinks(real); got != resolved {
		t.Errorf("ResolveFile(link) = %q, want %q", got, resolved)
	}

	// A directory is not a readable source file.
	if _, err := safepath.ResolveFile(dir); !errors.Is(err, safepath.ErrNotRegular) {
		t.Errorf("ResolveFile(dir) = %v, want ErrNotRegular", err)
	}

	// Absent files are errors at resolution, not at first read.
	if _, err := safepath.ResolveFile(filepath.Join(dir, "missing")); err == nil {
		t.Error("ResolveFile(missing) succeeded")
	}
}

func TestOpen(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "f.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}

	f, err := safepath.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = f.Close() }()

	buf := make([]byte, 5)
	if _, err := f.Read(buf); err != nil || string(buf) != "hello" {
		t.Errorf("read = %q, %v", buf, err)
	}
}
