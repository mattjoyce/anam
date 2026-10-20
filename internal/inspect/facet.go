package inspect

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/fold"
	"github.com/mattjoyce/anam/internal/store/ledger"
)

// The facet page: one facet at a version, its fields read through the Definition it
// was admitted under, what changed from the version before, and its events.

type facetData struct {
	// RowsNote says why Rows is empty when the Definition could not be read (card 100).
	RowsNote string
	Facet    facet.Facet
	Status   facet.Status
	FactTime string

	// DocumentURL is set for a reference facet whose path the library can serve:
	// the inspector's own route to the bytes. Empty otherwise.
	DocumentURL string

	// BytesNotHeld is set for a record.link (card 165): the facet names a file whose
	// bytes Anam does not hold, so the page says so in place of a download.
	BytesNotHeld bool

	// AnamTime is read off this artifact's own admitted event, not off the Facet: a
	// Facet carries one clock, the world's. When Anam learned of it is the ledger's
	// business, and the ledger is where the page has to go to ask.
	AnamTime string

	Bucket  string
	Payload []jsonToken
	Events  []facet.AnamEvent

	// Track is a place.track facet's hour drawn (card 133); nil otherwise.
	Track *trackView

	// Links is the live edges touching the facet, read from the edge index through
	// the same code as the API's links part (card 104).
	Links linksData

	// DefinitionURL is the Definition the facet was admitted under; CurrentDefinition
	// is the version in force now, so the page can say when they differ.
	DefinitionURL     string
	CurrentDefinition int

	// Rows is the payload read through its admitted Definition; Changes what this
	// artifact version changed from the one before; Versions every artifact version,
	// for links, and Latest the newest.
	Rows     []payloadRow
	Changes  []changeRow
	Versions []int
	Latest   int
	Prior    int

	// CanEdit and CanRetract are set on the newest version of a live facet that the
	// Workbench entered, and on no other (card 188).
	CanEdit    bool
	CanRetract bool
}

// facetAt resolves the version the page shows: the newest, or the one ?v= names,
// with the fields it changed from the version before. A missing facet or version is
// 404 and a bad ?v= is 400; on either it has answered, and ok is false.
func (s *Server) facetAt(ctx context.Context, w http.ResponseWriter, r *http.Request, id string,
) (latest, f facet.Facet, changes []changeRow, ok bool) {
	latest, err := s.ledger.CurrentVersion(ctx, id)
	if err != nil {
		notFoundOr500(w, err)
		return latest, f, nil, false
	}
	// ?v= shows any artifact version, so "why did this supersede" has an answer: the
	// version and what it changed from the one before.
	f = latest
	if q := r.URL.Query().Get("v"); q != "" {
		n, err := strconv.Atoi(q)
		if err != nil || n < 1 || n > latest.ArtifactVersion {
			http.Error(w, fmt.Sprintf("?v=%q: want 1 to %d", q, latest.ArtifactVersion), http.StatusBadRequest)
			return latest, f, nil, false
		}
		if f, err = s.ledger.Version(ctx, id, n); err != nil {
			notFoundOr500(w, err)
			return latest, f, nil, false
		}
	}
	if f.ArtifactVersion > 1 {
		prior, err := s.ledger.Version(ctx, id, f.ArtifactVersion-1)
		if err != nil {
			notFoundOr500(w, err)
			return latest, f, nil, false
		}
		changes = changedFields(prior.Payload.Bytes(), f.Payload.Bytes())
	}
	return latest, f, changes, true
}

// facetEvents is one facet's anam-events and its folded status at the head.
func (s *Server) facetEvents(ctx context.Context, id string) ([]facet.AnamEvent, facet.Status, error) {
	_, events, err := s.headAndEvents(ctx)
	if err != nil {
		return nil, "", err
	}
	var mine []facet.AnamEvent
	for _, ev := range events {
		if ev.FacetID == id {
			mine = append(mine, ev)
		}
	}
	states, err := fold.Fold(events)
	if err != nil {
		return nil, "", err
	}
	return mine, states[id].Status, nil
}

// notFoundOr500 answers a ledger read that failed: a facet or version the ledger does
// not hold is the caller's 404, anything else is Anam's problem.
func notFoundOr500(w http.ResponseWriter, err error) {
	if errors.Is(err, ledger.ErrNotFound) {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	httpError(w, err)
}

func (s *Server) facetPage(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), s.timeout)
	defer cancel()

	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "missing ?id=<facet id>", http.StatusBadRequest)
		return
	}

	latest, f, changes, ok := s.facetAt(ctx, w, r, id)
	if !ok {
		return
	}
	versions := make([]int, latest.ArtifactVersion)
	for i := range versions {
		versions[i] = i + 1
	}

	mine, status, err := s.facetEvents(ctx, id)
	if err != nil {
		httpError(w, err)
		return
	}

	// The facet links to the Definition it was admitted under, and says so when a
	// newer one is current (card 71: the admitted words lead).
	defURL := fmt.Sprintf("/definition?type=%s&v=%d", url.QueryEscape(string(f.Type)), f.DefinitionVersion)
	var currentDef int
	if reg, err := s.definitions(); err == nil {
		if d, ok := reg.Lookup(f.Type); ok {
			currentDef = d.Version
		}
	}

	rows, rowsNote := s.payloadRows(ctx, f)
	links, err := s.facetLinks(ctx, id)
	if err != nil {
		httpError(w, err)
		return
	}
	canEdit, canRetract := s.workbenchActions(f, latest.ArtifactVersion, status)
	render(w, "facet.html", facetData{
		Links:             links,
		CanEdit:           canEdit,
		CanRetract:        canRetract,
		DefinitionURL:     defURL,
		CurrentDefinition: currentDef,
		Rows:              rows,
		RowsNote:          rowsNote,
		Changes:           changes,
		Versions:          versions,
		Latest:            latest.ArtifactVersion,
		Prior:             f.ArtifactVersion - 1,
		Facet:             f,
		Status:            status,
		FactTime:          f.FactTime.Format(time.RFC3339),
		DocumentURL:       s.documentURL(f),
		BytesNotHeld:      bytesNotHeld(f),
		AnamTime:          anamTimeOf(mine, f.ArtifactVersion),
		Bucket:            s.buckets.Of(f.FactTime).ID(),
		Payload:           highlightJSON(f.Payload.Bytes()),
		Events:            mine,
		Track:             facetTrack(f),
	})
}
