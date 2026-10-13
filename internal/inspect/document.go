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
)

// Library documents: the bytes a doc.reference points at, served confined to the
// library root and isolated from the inspector's origin (card 106).

// documentURL is the inspector's link to a reference facet's bytes, when it has a
// library to serve them from. The path is read from the payload, which is where a
// reference says what it points at (journal/notes/reference-payloads.md).
func (s *Server) documentURL(f facet.Facet) string {
	if s.library == "" || f.Descriptor.PayloadKind != facet.PayloadReference {
		return ""
	}
	var ref struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(f.Payload.Bytes(), &ref); err != nil || ref.Path == "" {
		return ""
	}
	return "/document?path=" + url.QueryEscape(ref.Path)
}

// document serves one file from the default library by its library-relative path,
// as the reference facet recorded it. The open is confined to the library root
// (os.Root), so a path that climbs out, or a symlink that points out, is refused
// with the same answer as a file that is not there. The bytes are served as they
// are, with the media type the extension names; the inspector shows and never
// interprets.
//
// A document is untrusted material the person inspects, and the inspector shares
// its origin with the API (card 106). So every document is served in a CSP sandbox
// with neither scripts nor the origin, which keeps an HTML or SVG file from running
// as the operator and reading the record, and the active kinds (HTML, XHTML, SVG,
// XML) are downloads rather than previews. Images, PDFs and text stay inline.
func (s *Server) document(w http.ResponseWriter, r *http.Request) {
	rel := r.URL.Query().Get("path")
	if s.library == "" || rel == "" {
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
	w.Header().Set("Content-Type", mt)
	disposition := "inline"
	if activeDocument(mt) {
		disposition = "attachment"
	}
	w.Header().Set("Content-Disposition", disposition+"; filename=\""+path.Base(rel)+"\"")
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
