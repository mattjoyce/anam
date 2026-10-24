package inspect

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/consent"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/fold"
	"github.com/mattjoyce/anam/internal/projection"
	"github.com/mattjoyce/anam/internal/store/ledger"
)

// The projection page makes governance something you click rather than something the
// documentation asserts. Two controls, and each answers a different question:
//
//	projection   which governed variant — canonical, bp, sleep. Switching redraws the
//	             verdict on every facet, so "prune, never reshape" is watchable.
//	as-of        an anam-time. Winding it back shows what the projection stood on
//	             when Anam knew less: before the BP export arrived, `bp` serves nothing,
//	             and the page says so rather than showing today's answer.
//
// Every row carries a verdict from projection.Explain — the same decision procedure
// Apply uses, so the page cannot describe a filter the publisher does not apply.

type projVerdictRow struct {
	FactTime  string
	Type      facet.Type
	Version   int
	Standing  string
	Why       string
	SourceRef string
	ID        string
}

type projectionData struct {
	W        ledger.Watermark
	Selected string
	Names    []string
	AsOf     string
	AsOfSet  bool

	CanonicalHash string
	ServedHash    string
	Live          int
	Served        int

	Template *projection.Template

	// series is the Definitions' word on which types close a series, for a
	// snapshot-shaped template.
	series projection.Series

	// Summary is the default view: one line per type. Rows is the rows view, and
	// only that view: a type's rows of one standing, at most rowLimit of them.
	Summary     []typeSummary
	Rows        []projVerdictRow
	NServed     int
	NExcluded   int
	NSuperseded int

	// The rows view's request. Type is empty in the summary view. Zone is the zone
	// From and To are read in (card 236).
	Type, Show, From, To string
	Zone                 readZone
	// RowsTotal is how many rows match, whether or not all are drawn.
	RowsTotal int
}

func (s *Server) projectionPage(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), s.timeout)
	defer cancel()

	templates, err := s.templates()
	if err != nil {
		httpError(w, err)
		return
	}
	defs, err := s.definitions()
	if err != nil {
		httpError(w, err)
		return
	}

	z, ok := s.zoneOrFail(w, r.URL.Query())
	if !ok {
		return
	}
	vw, err := viewOf(r.URL.Query(), z.Loc)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	data := projectionData{
		series:   defs,
		Names:    append([]string{"canonical"}, templates.Names()...),
		Selected: r.URL.Query().Get("p"),
		Type:     vw.typ, Show: vw.show, From: vw.rawFrom, To: vw.rawTo, Zone: z,
	}
	if data.Selected == "" {
		data.Selected = "canonical"
	}

	res, err := s.readForQuery(ctx, r.URL.Query().Get("asof"), &data)
	switch {
	case errors.Is(err, errBadAsOf):
		// The operator's fault, and only this is.
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	case err != nil:
		// A failed ledger read is Anam's problem, not the caller's. Reporting it as
		// 400 would tell someone their perfectly good request was malformed.
		httpError(w, err)
		return
	}

	data.W = res.W
	data.Live = res.Value.Len()
	data.CanonicalHash = res.Value.Hash()

	if data.Selected != "canonical" {
		t, err := templates.Lookup(data.Selected)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		data.Template = &t
	}
	if err := data.fill(ctx, s.ledger, res, vw); err != nil {
		httpError(w, err)
		return
	}
	s.render(w, "projection.html", data)
}

// errBadAsOf marks the one failure in this handler that is the caller's fault. Every
// other error here is a failed read, which is Anam's problem and must not be reported
// as a malformed request.
var errBadAsOf = errors.New("bad as-of")

// parseLocalAsOf reads a datetime-local value, with or without seconds, as UTC.
func parseLocalAsOf(asof string) (time.Time, error) {
	t, err := time.ParseInLocation("2006-01-02T15:04:05", asof, time.UTC)
	if err == nil {
		return t, nil
	}
	return time.ParseInLocation("2006-01-02T15:04", asof, time.UTC)
}

// readForQuery reads at head, or at the watermark in force at an as-of anam-time.
func (s *Server) readForQuery(ctx context.Context, asof string, data *projectionData) (fold.Result, error) {
	if asof == "" {
		return s.read(ctx)
	}
	t, err := time.Parse(time.RFC3339, asof)
	if err != nil {
		// The form submits datetime-local, which omits the zone and carries seconds
		// when the browser shows them (the input's step is one second); read it as
		// UTC, because Anam's own clock is UTC.
		t, err = parseLocalAsOf(asof)
		if err != nil {
			return fold.Result{}, fmt.Errorf("%w: %q is neither RFC 3339 nor YYYY-MM-DDTHH:MM[:SS]", errBadAsOf, asof)
		}
	}
	data.AsOf, data.AsOfSet = asof, true

	w, err := fold.WatermarkAt(ctx, s.ledger, t)
	if err != nil {
		return fold.Result{}, err
	}
	return s.readAt(ctx, w)
}

// judgeLive puts every live facet through the template, or marks them all served when
// no template is selected, since the canonical is the un-narrowed read. It returns the
// verdict of each, and sets the served counts and hash.
func (d *projectionData) judgeLive(res fold.Result) ([]projVerdictRow, error) {
	var rows []projVerdictRow
	if d.Template == nil {
		d.ServedHash, d.Served = res.Value.Hash(), res.Value.Len()
		for _, e := range res.Value.Entries() {
			d.NServed++
			rows = append(rows, projVerdictRow{
				FactTime: e.FactTime, Type: e.Type,
				Version: res.States[e.FacetID].Version, Standing: "served",
				SourceRef: e.SourceRef, ID: e.FacetID,
			})
		}
		return rows, nil
	}

	verdicts, err := projection.Explain(res.Value, *d.Template, consent.Open{}, d.series)
	if err != nil {
		return nil, err
	}
	served, err := projection.Apply(res.Value, *d.Template, consent.Open{}, d.series)
	if err != nil {
		return nil, err
	}
	d.ServedHash, d.Served = served.Hash(), served.Len()

	for _, x := range verdicts {
		standing, why := "served", ""
		if !x.Reason.Served() {
			standing, why = "excluded", string(x.Reason)
			if x.Detail != "" {
				why += ": " + x.Detail
			}
			d.NExcluded++
		} else {
			d.NServed++
		}
		rows = append(rows, projVerdictRow{
			FactTime: x.Entry.FactTime, Type: x.Entry.Type,
			Version:  res.States[x.Entry.FacetID].Version,
			Standing: standing, Why: why,
			SourceRef: x.Entry.SourceRef, ID: x.Entry.FacetID,
		})
	}
	return rows, nil
}

func displacedRow(d fold.Displacement) projVerdictRow {
	return projVerdictRow{
		FactTime:  clock.Format(d.Facet.FactTime),
		Type:      d.Facet.Type,
		Version:   d.Version,
		Standing:  "superseded",
		Why:       d.Reason,
		SourceRef: d.Facet.Provenance.SourceRef,
		ID:        d.Facet.ID,
	}
}
