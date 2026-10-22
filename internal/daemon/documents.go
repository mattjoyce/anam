package daemon

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"

	"github.com/mattjoyce/anam/internal/api"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/gate"
	"github.com/mattjoyce/anam/internal/library"
	"github.com/mattjoyce/anam/internal/policy"
)

// docType is the facet type a stored document is referenced by; an importer may post
// documents only if it owns the references to them (policy.MayStoreDocument). It names
// a bundled type, so it is not configurable.
const docType = policy.DocumentType

// The ways a document post or a reference to one is refused.
var (
	// ErrDocumentExt: the ext parameter is not a plain file extension.
	ErrDocumentExt = errors.New("daemon: not a plain file extension")
	// ErrDocumentMissing: a record.document names a sha256 the library holds no bytes for.
	ErrDocumentMissing = errors.New("daemon: the library does not hold the document this reference names")
)

// postDocument stores one document's bytes in the library (cards 102, 150). Only
// Anam writes the library: an importer hands it the bytes and is answered with the
// sha256 a record.document names. The file is named by that sha256, so the same bytes
// again are unchanged, and other bytes are another file: a stored document is never
// overwritten. Where in the library it sits is the library's own business.
func (d *Daemon) postDocument(w http.ResponseWriter, r *http.Request) {
	imp := principalOf(r)
	if !policy.MayStoreDocument(imp) {
		writeError(w, http.StatusForbidden, fmt.Errorf("%s does not own %s, so may not store documents", imp.Source, docType))
		return
	}
	if d.cfg.LibraryDir == "" {
		writeError(w, http.StatusServiceUnavailable, errors.New("daemon: no library configured"))
		return
	}
	ext, err := documentExt(r.URL.Query().Get("ext"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, int64(d.cfg.MaxDocumentBytes)))
	if err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, err)
		return
	}
	sum := sha256.Sum256(body)
	stored := api.StoredDocument{
		SHA256: hex.EncodeToString(sum[:]), Bytes: int64(len(body)), MediaType: documentMediaType(ext),
	}
	// Through the write queue, so one writer counts and places, and a backup copying
	// the library never sees half a file.
	err = d.tryWriteWithin(r.Context(), d.cfg.ImporterWait, d.timeout(r), func(context.Context, Stores) error {
		var err error
		stored.New, err = library.Store(d.cfg.LibraryDir, d.cfg.LibraryFolderLimit, stored.SHA256, ext, body)
		return err
	})
	switch {
	case errors.Is(err, ErrBusy), errors.Is(err, ErrStopped):
		w.Header().Set("Retry-After", d.retryAfter())
		writeError(w, http.StatusServiceUnavailable, err)
	case err != nil:
		writeError(w, http.StatusInternalServerError, err)
	case stored.New:
		writeJSON(w, http.StatusCreated, stored)
	default:
		writeJSON(w, http.StatusOK, stored)
	}
}

// documentExt accepts a bare file extension, letters and digits, and gives it back
// lower-cased with its dot. The extension labels the stored file for a person
// browsing the library; empty is none.
func documentExt(e string) (string, error) {
	e = strings.ToLower(strings.TrimPrefix(e, "."))
	if len(e) > 16 {
		return "", fmt.Errorf("%w: %q", ErrDocumentExt, e)
	}
	for _, c := range e {
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') {
			return "", fmt.Errorf("%w: %q", ErrDocumentExt, e)
		}
	}
	if e == "" {
		return "", nil
	}
	return "." + e, nil
}

// documentMediaType is by extension: a label on the pointer, not a verdict on the
// bytes.
func documentMediaType(ext string) string {
	if t := mime.TypeByExtension(ext); t != "" {
		if i := strings.IndexByte(t, ';'); i >= 0 {
			t = t[:i]
		}
		return t
	}
	return "application/octet-stream"
}

// checkReferences refuses, before anything is admitted, every record.document in a
// batch whose sha256 the library holds no bytes for (cards 102, 150). The reference
// is a pointer, and a pointer to nothing is not a fact worth admitting. Its path is
// the document's name and identity, and plays no part in finding the bytes.
func checkReferences(dir string, envs []facet.Envelope) error {
	var refused gate.Refused
	var held map[string]string
	var why string
	for i, env := range envs {
		if env.Type != docType {
			continue
		}
		var ref struct {
			SHA256 string `json:"sha256"`
		}
		if err := json.Unmarshal(env.Payload, &ref); err != nil {
			continue // the gate refuses a malformed payload with its own reason
		}
		if held == nil && why == "" {
			held, why = heldDocuments(dir)
		}
		problem := why
		if _, ok := held[ref.SHA256]; problem == "" && !ok {
			problem = "the library holds no bytes with sha256 " + ref.SHA256
		}
		if problem != "" {
			refused.Envelopes = append(refused.Envelopes, gate.RefusedEnvelope{
				Index: i, Type: env.Type, Err: fmt.Errorf("%w: %s; post the bytes to /v1/documents first", ErrDocumentMissing, problem),
			})
		}
	}
	if len(refused.Envelopes) > 0 {
		return &refused
	}
	return nil
}

// heldDocuments indexes the library once for a batch, or says why it cannot.
func heldDocuments(dir string) (map[string]string, string) {
	if dir == "" {
		return nil, "no library is configured"
	}
	held, err := library.Index(dir)
	if err != nil {
		return nil, err.Error()
	}
	return held, ""
}
