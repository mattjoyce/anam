// Package inspect serves the web view over the ledger, the Anam Workbench: pages that
// read what Anam holds (MVP.md §3), and, when the daemon hands it a Workbench, forms
// that enter, correct and retract a fact by hand through the gate (card 188). The
// read pages never write; the entry routes are their own handlers (workbench.go).
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
	"errors"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/fold"
	"github.com/mattjoyce/anam/internal/projection"
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
	Versions(ctx context.Context, refs []ledger.VersionRef) ([]facet.Facet, error)
}

//go:embed templates/*.html
var templateFS embed.FS

var tmpl = template.Must(template.ParseFS(templateFS, "templates/*.html"))

// Server renders the inspector pages. Timeout bounds each request's ledger reads.
type Server struct {
	ledger  Ledger
	timeout time.Duration

	// library is the default library's root: the folder that holds the bytes a
	// record.document names by sha256. The inspector serves those bytes itself, confined
	// to the root, so a reference facet opens as the document it points at. Empty
	// means no library.
	library string

	// buckets is the operator's bucketing, so the id shown beside a facet is the one
	// the operational index holds for it rather than whatever this package assumed.
	buckets span.Buckets

	// defs answers the Definitions in force; nil means the bundled ones.
	defs func() *def.Registry

	// heldDefs answers every Definition version held, for the Definitions pages and a
	// facet's link to the version it was admitted under; nil means the bundled ones.
	heldDefs HeldDefinitions

	// checkpoints is the checkpoint log, read-only, for the bucket strip under the
	// timeline (card 21); nil means there is none to show.
	checkpoints Checkpoints

	// projections answers the templates held, bundled and registered (card 125), for
	// the projection page; nil means the bundled ones.
	projections func() *projection.Registry

	// bench is the Workbench the daemon supplies for entering facets by hand; nil means
	// the inspector only reads, and the entry routes do not exist (card 188).
	bench Workbench

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
	// cachedReads bounds the ring, so a person clicking through a year of timestamps
	// cannot grow the process without bound.
	cachedReads int
}

// Deps is what the inspector reads from. Ledger and Timeout are required; the rest
// widen what the pages can show. Library is the default library's root, or "" for
// none. Definitions answers the Definitions the record's facets were admitted
// against, asked at each page so a registration shows at once; nil means the bundled
// ones alone. Held answers every version held of each, for the Definitions pages; nil
// means the bundled manifests. Checkpoints is the checkpoint log for the bucket strip
// under the timeline; nil draws no strip. Projections answers the templates held; nil
// means the bundled ones.
type Deps struct {
	Ledger      Ledger
	Timeout     time.Duration
	Buckets     span.Buckets
	Library     string
	Definitions func() *def.Registry
	Held        HeldDefinitions
	Checkpoints Checkpoints
	Projections func() *projection.Registry
	// Workbench lets the pages enter, correct and retract facets by hand, through the
	// daemon's write; nil leaves the inspector read-only (a standalone `anam inspect`).
	Workbench Workbench
	// CachedReads is how many historical folds the inspector keeps
	// (inspect_cached_reads); zero keeps none.
	CachedReads int
}

// Handler returns the inspector's routes on a fresh mux.
func Handler(d Deps) http.Handler {
	s := &Server{
		ledger: d.Ledger, timeout: d.Timeout, buckets: d.Buckets, library: d.Library,
		defs: d.Definitions, heldDefs: d.Held, checkpoints: d.Checkpoints, projections: d.Projections,
		cachedReads: d.CachedReads, bench: d.Workbench,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.timeline)
	mux.HandleFunc("POST /view", s.setDefaultView)
	mux.HandleFunc("GET /facets", s.index)
	mux.HandleFunc("GET /facet", s.facetPage)
	mux.HandleFunc("GET /location", s.location)
	mux.HandleFunc("GET /events", s.events)
	mux.HandleFunc("GET /projection", s.projectionPage)
	mux.HandleFunc("GET /document", s.document)
	mux.HandleFunc("GET /definitions", s.definitionsIndex)
	mux.HandleFunc("GET /definition", s.definitionPage)
	if s.bench != nil {
		mux.HandleFunc("GET /entry", s.entryPage)
		mux.HandleFunc("POST /entry", s.entrySubmit)
		mux.HandleFunc("POST /retract", s.retractSubmit)
	}
	return mux
}

// definitions is the Definitions in force, or the bundled ones when none are given.
func (s *Server) definitions() (*def.Registry, error) {
	if s.defs != nil {
		return s.defs(), nil
	}
	return def.Bundled()
}

// templates is the projections held, or the bundled ones when none are given.
func (s *Server) templates() (*projection.Registry, error) {
	if s.projections != nil {
		return s.projections(), nil
	}
	return projection.Bundled()
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
	if s.cachedReads < 1 {
		return res, nil
	}
	if _, seen := s.asOf[w]; !seen {
		s.asOfRing = append(s.asOfRing, w)
		for len(s.asOfRing) > s.cachedReads {
			delete(s.asOf, s.asOfRing[0])
			s.asOfRing = s.asOfRing[1:]
		}
	}
	s.asOf[w] = res
	return res, nil
}

// Serve runs the inspector over d at addr until ctx is cancelled. readHeaderTimeout
// and shutdownGrace are the daemon's (http_read_header_timeout, shutdown_grace).
func Serve(ctx context.Context, addr string, d Deps, readHeaderTimeout, shutdownGrace time.Duration) error {
	srv := &http.Server{
		Addr:              addr,
		Handler:           Handler(d),
		ReadHeaderTimeout: readHeaderTimeout,
		BaseContext:       func(net.Listener) context.Context { return ctx },
	}

	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe() }()

	select {
	case <-ctx.Done():
		shutCtx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
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
	Source   string
	FactTime string
	Bucket   string
	Version  int
	Status   facet.Status
	Hash     string
}

type indexData struct {
	W ledger.Watermark
	facetList
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

	render(w, "index.html", indexData{W: res.W, facetList: pageFacets(rows, facetFilterOf(r), skipOf(r))})
}

func (s *Server) rowOf(f facet.Facet, st facet.Status) facetRow {
	return facetRow{
		ID:       f.ID,
		Type:     f.Type,
		Source:   f.Provenance.Source,
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
