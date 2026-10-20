package inspect

import (
	"encoding/json"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"

	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/library"
)

// Library documents: the bytes a record.document points at, found by their sha256,
// served confined to the library root and isolated from the inspector's origin
// (cards 106, 150).

// linkType is the facet type for a file Anam remembers but does not hold (card 165):
// an observation of a file's name, hash and date, registered by the links importer.
// Its sha256 names bytes outside Anam, so the page says so and offers no download.
// Not configurable: it names a type.
const linkType facet.Type = "record.link"

// bytesNotHeld says whether f names a file whose bytes Anam, by design, does not
// hold. A record.link is verbatim (the payload is the whole fact, not a pointer the
// inspector can follow), so documentURL already gives it no link; this is the page's
// word for why.
func bytesNotHeld(f facet.Facet) bool { return f.Type == linkType }

// documentURL is the inspector's link to a reference facet's bytes, when it has a
// library to serve them from. The sha256 is read from the payload, which is where a
// reference says what it points at (journal/notes/reference-payloads.md); the path,
// the document's name, rides along only to name the download.
func (s *Server) documentURL(f facet.Facet) string {
	if s.library == "" || f.Descriptor.PayloadKind != facet.PayloadReference {
		return ""
	}
	var ref struct {
		Path   string `json:"path"`
		SHA256 string `json:"sha256"`
	}
	if err := json.Unmarshal(f.Payload.Bytes(), &ref); err != nil || !library.IsSHA256(ref.SHA256) {
		return ""
	}
	q := url.Values{"sha256": {ref.SHA256}}
	if ref.Path != "" {
		q.Set("name", path.Base(ref.Path))
	}
	return "/document?" + q.Encode()
}

// document serves one file from the default library by its sha256, as the reference
// facet recorded it. The library finds the file by that name; the open is confined
// to the library root (os.Root), so a symlink that points out is refused with the
// same answer as a file that is not there. The bytes are served as they are, with
// the media type the stored file's extension names; the inspector shows and never
// interprets. The name parameter only names the download.
//
// A document is untrusted material the person inspects, and the inspector shares
// its origin with the API (card 106). So every document is served in a CSP sandbox
// with neither scripts nor the origin, which keeps an HTML or SVG file from running
// as the operator and reading the record, and the active kinds (HTML, XHTML, SVG,
// XML) are downloads rather than previews. Images, PDFs and text stay inline.
func (s *Server) document(w http.ResponseWriter, r *http.Request) {
	sum := r.URL.Query().Get("sha256")
	if s.library == "" || !library.IsSHA256(sum) {
		http.NotFound(w, r)
		return
	}
	rel, err := library.Find(s.library, sum)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	root, err := os.OpenRoot(s.library)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer func() { _ = root.Close() }()
	f, err := root.Open(rel)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		http.NotFound(w, r)
		return
	}
	mt := mime.TypeByExtension(path.Ext(rel))
	if mt == "" {
		mt = "application/octet-stream"
	}
	name := path.Base(r.URL.Query().Get("name"))
	if name == "." || name == "/" {
		name = path.Base(rel)
	}
	w.Header().Set("Content-Type", mt)
	disposition := "inline"
	if activeDocument(mt) {
		disposition = "attachment"
	}
	w.Header().Set("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": name}))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'")
	w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
	http.ServeContent(w, r, path.Base(rel), info.ModTime(), f)
}

// activeDocument reports whether a media type is one a browser would execute or
// let script into when shown inline: a page, not a picture.
func activeDocument(mediaType string) bool {
	mt, _, err := mime.ParseMediaType(mediaType)
	if err != nil {
		return true
	}
	switch mt {
	case "text/html", "application/xhtml+xml", "image/svg+xml", "text/xml", "application/xml":
		return true
	}
	return strings.HasSuffix(mt, "+xml")
}
