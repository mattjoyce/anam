package daemon

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path"
	"strings"

	"github.com/mattjoyce/anam/internal/api"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/gate"
)

// docType is the facet type a stored document is referenced by; an importer may post
// documents only if it may submit references to them.
const docType facet.Type = "doc.reference"

// maxDocument bounds one posted document. A scanned letter or a lab report is a few
// MB; a bound keeps one request from filling the disk.
const maxDocument = 64 << 20

// The ways a document post or a reference to one is refused.
var (
	// ErrDocumentPath: the path is not a plain library-relative path.
	ErrDocumentPath = errors.New("daemon: not a library-relative document path")
	// ErrDocumentHeld: the library holds other bytes at this path. A stored document
	// is a value: a changed document takes a new path, so the bytes an earlier
	// reference names are never overwritten.
	ErrDocumentHeld = errors.New("daemon: the library holds other bytes at this path")
	// ErrDocumentMissing: a doc.reference names a path the library does not hold, or
	// bytes other than the ones it holds there.
	ErrDocumentMissing = errors.New("daemon: the library does not hold the document this reference names")
)

// postDocument stores one document's bytes in the library (card 102). Only Anam
// writes the library: an importer hands it the bytes and is answered with what a
// doc.reference names. The same bytes at the same path again are unchanged.
func (d *Daemon) postDocument(w http.ResponseWriter, r *http.Request) {
	imp := importerOf(r)
	if !mayWrite(imp, docType) {
		writeError(w, http.StatusForbidden, fmt.Errorf("%s may not submit %s, so may not store documents", imp.Source, docType))
		return
	}
	if d.cfg.LibraryDir == "" {
		writeError(w, http.StatusServiceUnavailable, errors.New("daemon: no library configured"))
		return
	}
	rel, err := documentPath(r.URL.Query().Get("path"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxDocument))
	if err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, err)
		return
	}
	sum := sha256.Sum256(body)
	stored := api.StoredDocument{
		Path: rel, SHA256: hex.EncodeToString(sum[:]), Bytes: int64(len(body)), MediaType: documentMediaType(rel),
	}
	// Through the write queue, so a backup copying the library never sees half a file.
	err = d.tryWriteWithin(importerWait, d.timeout(r), func(context.Context, Stores) error {
		var err error
		stored.New, err = storeDocument(d.cfg.LibraryDir, rel, body, stored.SHA256)
		return err
	})
	switch {
	case errors.Is(err, ErrDocumentHeld):
		writeError(w, http.StatusConflict, err)
	case errors.Is(err, ErrBusy), errors.Is(err, ErrStopped):
		w.Header().Set("Retry-After", retryAfter)
		writeError(w, http.StatusServiceUnavailable, err)
	case err != nil:
		writeError(w, http.StatusInternalServerError, err)
	case stored.New:
		writeJSON(w, http.StatusCreated, stored)
	default:
		writeJSON(w, http.StatusOK, stored)
	}
}

// documentPath accepts a slash-separated, library-relative path to a file: no
// absolute path, no climbing out, no dot-file or dot-directory (the library walk
// skips those), and no sidecar name.
func documentPath(p string) (string, error) {
	if p == "" || strings.HasPrefix(p, "/") || strings.Contains(p, "\\") || path.Clean(p) != p {
		return "", fmt.Errorf("%w: %q", ErrDocumentPath, p)
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == "" || seg == "." || seg == ".." || strings.HasPrefix(seg, ".") {
			return "", fmt.Errorf("%w: %q", ErrDocumentPath, p)
		}
	}
	if strings.HasSuffix(p, ".meta.yaml") {
		return "", fmt.Errorf("%w: %q is a sidecar name", ErrDocumentPath, p)
	}
	return p, nil
}

// storeDocument writes body at rel under the library root, confined to it (os.Root),
// through a temporary file renamed into place, so the path holds the whole file or
// nothing. It reports whether the bytes are new; the same bytes already there are not.
func storeDocument(library, rel string, body []byte, sum string) (bool, error) {
	if err := os.MkdirAll(library, 0o750); err != nil {
		return false, fmt.Errorf("daemon: library: %w", err)
	}
	root, err := os.OpenRoot(library)
	if err != nil {
		return false, fmt.Errorf("daemon: library: %w", err)
	}
	defer func() { _ = root.Close() }()

	switch held, err := heldSum(root, rel); {
	case err == nil && held == sum:
		return false, nil
	case err == nil:
		return false, fmt.Errorf("%w: %s holds sha256 %s; a changed document takes a new path", ErrDocumentHeld, rel, held)
	case !errors.Is(err, fs.ErrNotExist):
		return false, err
	}

	if dir := path.Dir(rel); dir != "." {
		if err := root.MkdirAll(dir, 0o750); err != nil {
			return false, fmt.Errorf("daemon: library %s: %w", dir, err)
		}
	}
	var tag [8]byte
	if _, err := rand.Read(tag[:]); err != nil {
		return false, err
	}
	tmp := path.Join(path.Dir(rel), ".incoming-"+hex.EncodeToString(tag[:]))
	f, err := root.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o640)
	if err != nil {
		return false, fmt.Errorf("daemon: library: %w", err)
	}
	_, werr := f.Write(body)
	serr := f.Sync()
	cerr := f.Close()
	if err := errors.Join(werr, serr, cerr); err != nil {
		_ = root.Remove(tmp)
		return false, fmt.Errorf("daemon: library %s: %w", rel, err)
	}
	if err := root.Rename(tmp, rel); err != nil {
		_ = root.Remove(tmp)
		return false, fmt.Errorf("daemon: library %s: %w", rel, err)
	}
	return true, nil
}

// heldSum is the sha256 of the regular file at rel, hex.
func heldSum(root *os.Root, rel string) (string, error) {
	f, err := root.Open(rel)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("%w: %s is not a regular file", ErrDocumentHeld, rel)
	}
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// documentMediaType is by extension, as the library worker names it: a label on the
// pointer, not a verdict on the bytes.
func documentMediaType(rel string) string {
	if t := mime.TypeByExtension(strings.ToLower(path.Ext(rel))); t != "" {
		if i := strings.IndexByte(t, ';'); i >= 0 {
			t = t[:i]
		}
		return t
	}
	return "application/octet-stream"
}

// checkReferences refuses, before anything is admitted, every doc.reference in a
// batch whose path the library does not hold or whose sha256 is not the bytes held
// there (card 102). The reference is a pointer, and a pointer to nothing is not a fact
// worth admitting.
func checkReferences(library string, envs []facet.Envelope) error {
	var refused gate.Refused
	var root *os.Root
	for i, env := range envs {
		if env.Type != docType {
			continue
		}
		var ref struct {
			Path   string `json:"path"`
			SHA256 string `json:"sha256"`
		}
		if err := json.Unmarshal(env.Payload, &ref); err != nil {
			continue // the gate refuses a malformed payload with its own reason
		}
		if root == nil && library != "" {
			r, err := os.OpenRoot(library)
			if err == nil {
				root = r
				defer func() { _ = root.Close() }()
			}
		}
		if why := referenceProblem(root, ref.Path, ref.SHA256); why != "" {
			refused.Envelopes = append(refused.Envelopes, gate.RefusedEnvelope{
				Index: i, Type: env.Type, Err: fmt.Errorf("%w: %s; post the bytes to /v1/documents first", ErrDocumentMissing, why),
			})
		}
	}
	if len(refused.Envelopes) > 0 {
		return &refused
	}
	return nil
}

// referenceProblem says why a reference to path with sum is not one the library can
// honour, or "" when it can.
func referenceProblem(root *os.Root, p, sum string) string {
	if root == nil {
		return "no library is configured or readable"
	}
	if _, err := documentPath(p); err != nil {
		return err.Error()
	}
	held, err := heldSum(root, p)
	if err != nil {
		return fmt.Sprintf("the library holds nothing at %s", p)
	}
	if held != sum {
		return fmt.Sprintf("%s holds sha256 %s, the reference names %s", p, held, sum)
	}
	return ""
}
