// Package inspect serves a read-only web view over the ledger — the diagnostic
// harness for looking at what Anam holds (MVP.md §3: a harness for proving the model,
// not a shipped UI).
//
// It renders the ledger's own two surfaces and nothing more: facets at their current
// artifact version, and the record-event stream. There is no fold here — current
// status is the fold's to derive (R4), so the facet listing shows the *last event*
// against each facet, which is a fact, not a status.
package inspect

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"sort"
	"time"

	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/span"
	"github.com/mattjoyce/anam/internal/store/ledger"
)

// Ledger is the narrow read surface the inspector needs (interfaces at the point of
// use; *ledger.Ledger satisfies it).
type Ledger interface {
	Head(ctx context.Context) (ledger.Watermark, error)
	Replay(ctx context.Context, sinceW, uptoW ledger.Watermark) ([]facet.RecordEvent, error)
	CurrentVersion(ctx context.Context, facetID string) (facet.Facet, error)
}

//go:embed templates/*.html
var templateFS embed.FS

var tmpl = template.Must(template.ParseFS(templateFS, "templates/*.html"))

// Server renders the inspector pages. Timeout bounds each request's ledger reads.
type Server struct {
	ledger  Ledger
	timeout time.Duration
}

// Handler returns the inspector's routes on a fresh mux.
func Handler(l Ledger, timeout time.Duration) http.Handler {
	s := &Server{ledger: l, timeout: timeout}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.index)
	mux.HandleFunc("GET /facet", s.facetPage)
	mux.HandleFunc("GET /events", s.events)
	return mux
}

// Serve runs the inspector at addr until ctx is cancelled.
func Serve(ctx context.Context, l Ledger, addr string, timeout time.Duration) error {
	srv := &http.Server{
		Addr:              addr,
		Handler:           Handler(l, timeout),
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

// facetRow is one line of the index: a logical facet at its current version.
type facetRow struct {
	ID        string
	Type      facet.Type
	EventTime string
	Bucket    string
	Version   int
	LastEvent facet.RecordEventKind
	Hash      string
}

type indexData struct {
	W      ledger.Watermark
	Events int
	Facets []facetRow
}

func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), s.timeout)
	defer cancel()

	head, events, err := s.headAndEvents(ctx)
	if err != nil {
		httpError(w, err)
		return
	}

	lastEvent := map[string]facet.RecordEventKind{}
	for _, ev := range events {
		lastEvent[ev.FacetID] = ev.Kind
	}

	rows := make([]facetRow, 0, len(lastEvent))
	for id, kind := range lastEvent {
		f, err := s.ledger.CurrentVersion(ctx, id)
		if err != nil {
			httpError(w, err)
			return
		}
		rows = append(rows, facetRow{
			ID:        f.ID,
			Type:      f.Type,
			EventTime: f.EventTime.Format(time.RFC3339),
			Bucket:    span.Of(f.EventTime).ID(),
			Version:   f.ArtifactVersion,
			LastEvent: kind,
			Hash:      f.Descriptor.ContentHash,
		})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].EventTime < rows[j].EventTime })

	render(w, "index.html", indexData{W: head, Events: len(events), Facets: rows})
}

type facetData struct {
	Facet     facet.Facet
	EventTime string
	Recorded  string
	Ingested  string
	Bucket    string
	Payload   string
	Events    []facet.RecordEvent
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
	var mine []facet.RecordEvent
	for _, ev := range events {
		if ev.FacetID == id {
			mine = append(mine, ev)
		}
	}

	var pretty bytes.Buffer
	if err := json.Indent(&pretty, f.Payload.Bytes(), "", "  "); err != nil {
		pretty.Reset()
		pretty.Write(f.Payload.Bytes()) // verbatim beats absent
	}

	render(w, "facet.html", facetData{
		Facet:     f,
		EventTime: f.EventTime.Format(time.RFC3339),
		Recorded:  f.RecordedTime.Format(time.RFC3339),
		Ingested:  f.IngestedTime.Format(time.RFC3339),
		Bucket:    span.Of(f.EventTime).ID(),
		Payload:   pretty.String(),
		Events:    mine,
	})
}

type eventsData struct {
	W      ledger.Watermark
	Events []eventRow
}

type eventRow struct {
	facet.RecordEvent
	RecordTimeText string
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
		rows[i] = eventRow{RecordEvent: ev, RecordTimeText: ev.RecordTime.Format(time.RFC3339)}
	}
	render(w, "events.html", eventsData{W: head, Events: rows})
}

// headAndEvents reads the watermark and the full record-event stream — the inspector
// re-reads rather than caches, so what it shows is what the ledger holds right now.
func (s *Server) headAndEvents(ctx context.Context) (ledger.Watermark, []facet.RecordEvent, error) {
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
