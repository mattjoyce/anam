// Package inspect serves a read-only web view over the ledger — the diagnostic
// harness for looking at what Anam holds (MVP.md §3: a harness for proving the model,
// not a shipped UI).
//
// Since R4 the fold exists, so the inspector renders both of Anam's surfaces under
// their own names (§7): the landing page is the capture→compose picture — the
// ledger band (every facet, marked at its folded status) over the canonical strip
// (the live-only value at W) — and the tables show folded status rather than R3's
// last-event stand-in.
package inspect

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"sort"
	"sync"
	"time"

	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/fold"
	"github.com/mattjoyce/anam/internal/span"
	"github.com/mattjoyce/anam/internal/store/ledger"
)

// Ledger is the narrow read surface the inspector needs (interfaces at the point of
// use; *ledger.Ledger satisfies it, and it is exactly fold.Ledger).
type Ledger interface {
	Head(ctx context.Context) (ledger.Watermark, error)
	Replay(ctx context.Context, sinceW, uptoW ledger.Watermark) ([]facet.AnamEvent, error)
	CurrentVersion(ctx context.Context, facetID string) (facet.Facet, error)
	Version(ctx context.Context, facetID string, version int) (facet.Facet, error)
}

//go:embed templates/*.html
var templateFS embed.FS

var tmpl = template.Must(template.ParseFS(templateFS, "templates/*.html"))

// Server renders the inspector pages. Timeout bounds each request's ledger reads.
type Server struct {
	ledger  Ledger
	timeout time.Duration

	// library is the default library's root: the folder a doc.reference's path is
	// relative to. The inspector serves those bytes itself, confined to the root, so
	// a reference facet opens as the document it points at. Empty means no library.
	library string

	// buckets is the operator's bucketing, so the id shown beside a facet is the one
	// the operational index holds for it rather than whatever this package assumed.
	buckets span.Buckets

	// The fold cache, keyed by watermark: a whole-person read is one replay plus
	// one fetch per live facet, too dear to repeat per page view. Every request
	// still reads the head (one cheap call), so a moved W is never served stale —
	// the cache holds a *value*, and values don't go stale, they get replaced.
	mu     sync.Mutex
	cached fold.Result
	warm   bool

	// asOfCache holds recent historical reads. The head cache above is keyed on a
	// moving W; these are keyed on a watermark the operator chose, which does not
	// move, so they can be kept until the ring wraps. Small on purpose: an as-of
	// read is a deliberate act, not a page refresh.
	asOf     map[ledger.Watermark]fold.Result
	asOfRing []ledger.Watermark
}

// asOfCacheSize bounds the historical-read cache. Four is enough to flick between a
// couple of instants without re-folding, and small enough that a person clicking
// through a year of timestamps cannot grow the process without bound.
const asOfCacheSize = 4

// Handler returns the inspector's routes on a fresh mux. library is the default
// library's root, or "" for none.
func Handler(l Ledger, timeout time.Duration, buckets span.Buckets, library string) http.Handler {
	s := &Server{ledger: l, timeout: timeout, buckets: buckets, library: library}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.timeline)
	mux.HandleFunc("GET /facets", s.index)
	mux.HandleFunc("GET /facet", s.facetPage)
	mux.HandleFunc("GET /events", s.events)
	mux.HandleFunc("GET /projection", s.projectionPage)
	mux.HandleFunc("GET /document", s.document)
	return mux
}

// read returns the whole person at the current head, from cache when the
// watermark has not moved.
func (s *Server) read(ctx context.Context) (fold.Result, error) {
	head, err := s.ledger.Head(ctx)
	if err != nil {
		return fold.Result{}, err
	}

	s.mu.Lock()
	if s.warm && s.cached.W == head {
		res := s.cached
		s.mu.Unlock()
		return res, nil
	}
	s.mu.Unlock()

	res, err := fold.Read(ctx, s.ledger)
	if err != nil {
		return fold.Result{}, err
	}
	s.mu.Lock()
	s.cached, s.warm = res, true
	s.mu.Unlock()
	return res, nil
}

// readAt is read's as-of counterpart: the canonical at a watermark the caller chose.
func (s *Server) readAt(ctx context.Context, w ledger.Watermark) (fold.Result, error) {
	s.mu.Lock()
	if res, ok := s.asOf[w]; ok {
		s.mu.Unlock()
		return res, nil
	}
	s.mu.Unlock()

	res, err := fold.ReadAt(ctx, s.ledger, w)
	if err != nil {
		return fold.Result{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.asOf == nil {
		s.asOf = map[ledger.Watermark]fold.Result{}
	}
	if _, seen := s.asOf[w]; !seen {
		s.asOfRing = append(s.asOfRing, w)
		for len(s.asOfRing) > asOfCacheSize {
			delete(s.asOf, s.asOfRing[0])
			s.asOfRing = s.asOfRing[1:]
		}
	}
	s.asOf[w] = res
	return res, nil
}

// Serve runs the inspector at addr until ctx is cancelled.
func Serve(ctx context.Context, l Ledger, addr string, timeout time.Duration, buckets span.Buckets, library string) error {
	srv := &http.Server{
		Addr:              addr,
		Handler:           Handler(l, timeout, buckets, library),
		ReadHeaderTimeout: 5 * time.Second,
		BaseContext:       func(net.Listener) context.Context { return ctx },
	}

	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe() }()

	select {
	case <-ctx.Done():
		shutCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		return srv.Shutdown(shutCtx)
	case err := <-errc:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("inspect: serve %s: %w", addr, err)
	}
}

// facetRow is one line of the index: a logical facet at its current version, at
// its folded status.
type facetRow struct {
	ID       string
	Type     facet.Type
	FactTime string
	Bucket   string
	Version  int
	Status   facet.Status
	Hash     string
}

type indexData struct {
	W      ledger.Watermark
	Facets []facetRow
}

func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), s.timeout)
	defer cancel()

	res, err := s.read(ctx)
	if err != nil {
		httpError(w, err)
		return
	}

	// The ledger surface lists everything, marked: the live set comes from the
	// read; the dead (tombstoned, rejected) are fetched individually — few, and
	// deliberately absent from the cached canonical.
	rows := make([]facetRow, 0, len(res.States))
	for _, f := range res.Live {
		rows = append(rows, s.rowOf(f, res.States[f.ID].Status))
	}
	for id, st := range res.States {
		if st.Live() {
			continue
		}
		// At the version the fold derived, not the latest: a facet that was
		// tombstoned and later re-captured is dead at v2 in this fold and live at
		// v3 in the ledger, and rendering v3's content under v2's status would
		// show a row that was never true.
		f, err := s.ledger.Version(ctx, id, st.Version)
		if err != nil {
			httpError(w, err)
			return
		}
		rows = append(rows, s.rowOf(f, st.Status))
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].FactTime != rows[j].FactTime {
			return rows[i].FactTime < rows[j].FactTime
		}
		return rows[i].ID < rows[j].ID
	})

	render(w, "index.html", indexData{W: res.W, Facets: rows})
}

func (s *Server) rowOf(f facet.Facet, st facet.Status) facetRow {
	return facetRow{
		ID:       f.ID,
		Type:     f.Type,
		FactTime: f.FactTime.Format(time.RFC3339),
		Bucket:   s.buckets.Of(f.FactTime).ID(),
		Version:  f.ArtifactVersion,
		Status:   st,
		Hash:     f.Descriptor.ContentHash,
	}
}

// anamTimeOf finds when Anam admitted this artifact version. A version always has an
// admitting event — that is what put it in the ledger — but the page renders whatever
// it is handed, so an absent one shows as unknown rather than as the zero instant.
func anamTimeOf(events []facet.AnamEvent, version int) string {
	for _, ev := range events {
		if ev.ArtifactVersion == version && (ev.Kind == facet.EventAdmitted || ev.Kind == facet.EventSuperseded) {
			return ev.AnamTime.Format(time.RFC3339)
		}
	}
	return "unknown"
}

type facetData struct {
	Facet    facet.Facet
	Status   facet.Status
	FactTime string

	// DocumentURL is set for a reference facet whose path the library can serve:
	// the inspector's own route to the bytes. Empty otherwise.
	DocumentURL string

	// AnamTime is read off this artifact's own admitted event, not off the Facet: a
	// Facet carries one clock, the world's. When Anam learned of it is the ledger's
	// business, and the ledger is where the page has to go to ask.
	AnamTime string

	Bucket  string
	Payload string
	Events  []facet.AnamEvent
}

func (s *Server) facetPage(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), s.timeout)
	defer cancel()

	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "missing ?id=<facet id>", http.StatusBadRequest)
		return
	}

	f, err := s.ledger.CurrentVersion(ctx, id)
	if errors.Is(err, ledger.ErrNotFound) {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if err != nil {
		httpError(w, err)
		return
	}

	_, events, err := s.headAndEvents(ctx)
	if err != nil {
		httpError(w, err)
		return
	}
	var mine []facet.AnamEvent
	for _, ev := range events {
		if ev.FacetID == id {
			mine = append(mine, ev)
		}
	}
	states, err := fold.Fold(events)
	if err != nil {
		httpError(w, err)
		return
	}

	var pretty bytes.Buffer
	if err := json.Indent(&pretty, f.Payload.Bytes(), "", "  "); err != nil {
		pretty.Reset()
		pretty.Write(f.Payload.Bytes()) // verbatim beats absent
	}

	render(w, "facet.html", facetData{
		Facet:       f,
		Status:      states[id].Status,
		FactTime:    f.FactTime.Format(time.RFC3339),
		DocumentURL: s.documentURL(f),
		AnamTime:    anamTimeOf(mine, f.ArtifactVersion),
		Bucket:      s.buckets.Of(f.FactTime).ID(),
		Payload:     pretty.String(),
		Events:      mine,
	})
}

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
	if mt := mime.TypeByExtension(path.Ext(rel)); mt != "" {
		w.Header().Set("Content-Type", mt)
	} else {
		w.Header().Set("Content-Type", "application/octet-stream")
	}
	w.Header().Set("Content-Disposition", "inline; filename=\""+path.Base(rel)+"\"")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, path.Base(rel), info.ModTime(), f)
}

type eventsData struct {
	W      ledger.Watermark
	Events []eventRow
}

type eventRow struct {
	facet.AnamEvent
	AnamTimeText string
}

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), s.timeout)
	defer cancel()

	head, events, err := s.headAndEvents(ctx)
	if err != nil {
		httpError(w, err)
		return
	}
	rows := make([]eventRow, len(events))
	for i, ev := range events {
		rows[i] = eventRow{AnamEvent: ev, AnamTimeText: ev.AnamTime.Format(time.RFC3339)}
	}
	render(w, "events.html", eventsData{W: head, Events: rows})
}

// headAndEvents reads the watermark and the full anam-event stream — the inspector
// re-reads rather than caches, so what it shows is what the ledger holds right now.
func (s *Server) headAndEvents(ctx context.Context) (ledger.Watermark, []facet.AnamEvent, error) {
	head, err := s.ledger.Head(ctx)
	if err != nil {
		return 0, nil, err
	}
	events, err := s.ledger.Replay(ctx, 0, head)
	if err != nil {
		return 0, nil, err
	}
	return head, events, nil
}

func render(w http.ResponseWriter, name string, data any) {
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, name, data); err != nil {
		httpError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = buf.WriteTo(w)
}

func httpError(w http.ResponseWriter, err error) {
	http.Error(w, err.Error(), http.StatusInternalServerError)
}
