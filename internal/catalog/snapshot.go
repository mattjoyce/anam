package catalog

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"sort"
	"strings"
	"time"
)

// snapshot is a catalog root frozen in memory: the selected *.json files at the root
// and directly under history/, nothing else. Every parser and the set version read
// from the same snapshot, so a catalog's hash is always of the bytes it parsed, and
// later edits to the source change nothing already loaded.
type snapshot struct {
	root    map[string][]byte // file name -> bytes
	history map[string][]byte // file name -> bytes; nil when the source has no history/
}

// takeSnapshot reads a catalog root into memory. A missing history/ is an empty
// history; any other failure to read it, or a history that is not a directory, is
// an error. Entries that are not regular *.json files are ignored, as the embedded
// catalog's pattern ignores them.
func takeSnapshot(src fs.FS) (*snapshot, error) {
	root, err := readSelected(src, ".")
	if err != nil {
		return nil, err
	}
	s := &snapshot{root: root}
	info, err := fs.Stat(src, "history")
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return s, nil
	case err != nil:
		return nil, fmt.Errorf("stat history: %w", err)
	case !info.IsDir():
		return nil, fmt.Errorf("history is not a directory: %w", fs.ErrInvalid)
	}
	if s.history, err = readSelected(src, "history"); err != nil {
		return nil, err
	}
	return s, nil
}

// readSelected reads every regular *.json file directly in dir.
func readSelected(src fs.FS, dir string) (map[string][]byte, error) {
	entries, err := fs.ReadDir(src, dir)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", dir, err)
	}
	out := make(map[string][]byte, len(entries))
	for _, e := range entries {
		if !e.Type().IsRegular() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		name := e.Name()
		if dir != "." {
			name = dir + "/" + e.Name()
		}
		raw, err := fs.ReadFile(src, name)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", name, err)
		}
		out[e.Name()] = raw
	}
	return out, nil
}

// dirRoot opens a configured directory as a catalog source. It must exist and be a
// directory; it is never created and never falls back to the embedded catalog.
func dirRoot(dir string) (fs.FS, error) {
	info, err := os.Stat(dir)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("not a directory: %w", fs.ErrInvalid)
	}
	return os.DirFS(dir), nil
}

// The snapshot is an fs.FS with two directories, "." and "history", so the def and
// projection readers run over it unchanged.

func (s *snapshot) dir(name string) (map[string][]byte, bool) {
	switch name {
	case ".":
		return s.root, true
	case "history":
		return s.history, s.history != nil
	}
	return nil, false
}

func (s *snapshot) lookup(name string) ([]byte, bool) {
	d, base := path.Split(name)
	files, ok := s.dir(strings.TrimSuffix(d, "/"))
	if d == "" {
		files, ok = s.root, true
	}
	if !ok {
		return nil, false
	}
	raw, ok := files[base]
	return raw, ok
}

// Open implements fs.FS.
func (s *snapshot) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}
	if files, ok := s.dir(name); ok {
		return &snapDir{name: name, entries: entriesOf(files)}, nil
	}
	if raw, ok := s.lookup(name); ok {
		return &snapFile{name: path.Base(name), r: bytes.NewReader(raw), size: int64(len(raw))}, nil
	}
	return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
}

// ReadDir implements fs.ReadDirFS.
func (s *snapshot) ReadDir(name string) ([]fs.DirEntry, error) {
	files, ok := s.dir(name)
	if !ok {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrNotExist}
	}
	return entriesOf(files), nil
}

// ReadFile implements fs.ReadFileFS. It returns a copy, so no reader can change the
// snapshot.
func (s *snapshot) ReadFile(name string) ([]byte, error) {
	raw, ok := s.lookup(name)
	if !ok {
		return nil, &fs.PathError{Op: "read", Path: name, Err: fs.ErrNotExist}
	}
	return append([]byte(nil), raw...), nil
}

func entriesOf(files map[string][]byte) []fs.DirEntry {
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]fs.DirEntry, len(names))
	for i, n := range names {
		out[i] = fs.FileInfoToDirEntry(snapInfo{name: n, size: int64(len(files[n]))})
	}
	return out
}

type snapInfo struct {
	name string
	size int64
	dir  bool
}

func (i snapInfo) Name() string { return i.name }
func (i snapInfo) Size() int64  { return i.size }
func (i snapInfo) Mode() fs.FileMode {
	if i.dir {
		return fs.ModeDir | 0o555
	}
	return 0o444
}
func (i snapInfo) ModTime() time.Time { return time.Time{} }
func (i snapInfo) IsDir() bool        { return i.dir }
func (i snapInfo) Sys() any           { return nil }

type snapFile struct {
	name string
	r    *bytes.Reader
	size int64
}

func (f *snapFile) Stat() (fs.FileInfo, error) { return snapInfo{name: f.name, size: f.size}, nil }
func (f *snapFile) Read(p []byte) (int, error) { return f.r.Read(p) }
func (f *snapFile) Close() error               { return nil }

type snapDir struct {
	name    string
	entries []fs.DirEntry
	off     int
}

func (d *snapDir) Stat() (fs.FileInfo, error) {
	return snapInfo{name: path.Base(d.name), dir: true}, nil
}
func (d *snapDir) Read([]byte) (int, error) {
	return 0, &fs.PathError{Op: "read", Path: d.name, Err: errors.New("is a directory")}
}
func (d *snapDir) Close() error { return nil }

// ReadDir implements fs.ReadDirFile.
func (d *snapDir) ReadDir(n int) ([]fs.DirEntry, error) {
	rest := d.entries[d.off:]
	if n <= 0 {
		d.off = len(d.entries)
		return rest, nil
	}
	if len(rest) == 0 {
		return nil, io.EOF
	}
	if n > len(rest) {
		n = len(rest)
	}
	d.off += n
	return rest[:n], nil
}
