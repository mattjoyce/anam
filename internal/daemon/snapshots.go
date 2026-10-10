package daemon

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"time"

	"github.com/mattjoyce/anam/internal/api"
	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/config"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/gate"
	"github.com/mattjoyce/anam/internal/read"
	"github.com/mattjoyce/anam/internal/store/ledger"
	"github.com/mattjoyce/anam/internal/store/ops"
)

// A complete resend (card 78) labels its batches as the parts of one snapshot. The
// daemon admits each part as any batch, records which parts and facet ids arrived,
// and reports what a complete snapshot leaves absent. It tombstones nothing: Matt
// decided that retraction stays an operator's anam retract --yes.

// maxSnapshotParts bounds a run: at the gate's batch size it is far beyond any source.
const maxSnapshotParts = 100000

var snapshotID = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,128}$`)

// validSnapshot checks a part's label against the importer and the batch it labels.
func validSnapshot(s *api.SnapshotPart, imp config.Importer, envs []api.SubmittedEnvelope) error {
	if s == nil {
		return nil
	}
	switch {
	case !snapshotID.MatchString(s.ID):
		return fmt.Errorf("snapshot id %q: 1 to 128 of letters, digits and . _ : -", s.ID)
	case s.Parts < 1 || s.Parts > maxSnapshotParts:
		return fmt.Errorf("snapshot parts %d: want 1 to %d", s.Parts, maxSnapshotParts)
	case s.Part < 1 || s.Part > s.Parts:
		return fmt.Errorf("snapshot part %d of %d", s.Part, s.Parts)
	case len(s.Types) == 0:
		return errors.New("snapshot names no types")
	}
	speaks := map[string]bool{}
	for _, t := range s.Types {
		if !mayWrite(imp, facet.Type(t)) {
			return fmt.Errorf("snapshot type %s is not one %s may submit", t, imp.Source)
		}
		speaks[t] = true
	}
	for i, e := range envs {
		if !speaks[e.Type] {
			return fmt.Errorf("envelope %d is %s, which the snapshot does not speak for", i, e.Type)
		}
	}
	return nil
}

// checkSnapshot refuses a part whose id names a run begun with another source, other
// types or another part count.
func checkSnapshot(ctx context.Context, db *ops.DB, source string, s *api.SnapshotPart) error {
	if s == nil {
		return nil
	}
	held, ok, err := db.Snapshot(ctx, s.ID)
	if err != nil || !ok {
		return err
	}
	if held.Source != source || held.Parts != s.Parts || !sameTypes(held.Types, s.Types) {
		return fmt.Errorf("%w: %s", ops.ErrSnapshotMismatch, s.ID)
	}
	return nil
}

func sameTypes(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	set := map[string]bool{}
	for _, t := range a {
		set[t] = true
	}
	for _, t := range b {
		if !set[t] {
			return false
		}
	}
	return true
}

// recordSnapshotPart records an admitted part and the facet ids its results carried,
// every outcome counted, no-op included: a no-op is a facet the source still holds.
func recordSnapshotPart(ctx context.Context, db *ops.DB, source string, s *api.SnapshotPart,
	results []gate.Result, head ledger.Watermark, at time.Time,
) (*api.SnapshotProgress, error) {
	ids := make([]string, len(results))
	for i, r := range results {
		ids[i] = r.FacetID
	}
	rec, err := db.RecordSnapshotPart(ctx, ops.SnapshotPart{
		ID: s.ID, Source: source, Types: s.Types, Part: s.Part, Parts: s.Parts, FacetIDs: ids,
	}, at, int64(head))
	if err != nil {
		return nil, fmt.Errorf("batch admitted, snapshot part not recorded; resend the part: %w", err)
	}
	return &api.SnapshotProgress{ID: rec.ID, Parts: rec.Parts, PartsAdmitted: rec.PartsAdmitted, Complete: rec.Complete()}, nil
}

// getSnapshot reports a snapshot. An importer sees only its own source's; the
// operator sees any. Once complete, the report says what the source's live facets of
// its types are and which of them it left absent.
func (d *Daemon) getSnapshot(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), d.timeout(r))
	defer cancel()
	s, ok, err := d.readOps.Snapshot(ctx, r.PathValue("id"))
	if imp, isImporter := d.importers.of(r); ok && isImporter && imp.Source != s.Source {
		ok = false // another importer's run is not this one's to see
	}
	switch {
	case err != nil:
		writeError(w, http.StatusInternalServerError, err)
		return
	case !ok:
		writeError(w, http.StatusNotFound, fmt.Errorf("no snapshot %s", r.PathValue("id")))
		return
	}
	rep := api.SnapshotReport{
		SnapshotProgress: api.SnapshotProgress{ID: s.ID, Parts: s.Parts, PartsAdmitted: s.PartsAdmitted, Complete: s.Complete()},
		Source:           s.Source, Types: s.Types, Carried: s.Carried, StartedAt: clock.Format(s.StartedAt),
	}
	if s.Complete() {
		rep.CompletedAt, rep.CompletedW = clock.Format(s.CompletedAt), s.CompletedW
		carried, err := d.readOps.SnapshotFacets(ctx, s.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		src := read.Source{Ledger: d.read, Checkpoints: d.readOps, Artifacts: d.artifacts, Definitions: d.defs.current.Load()}
		absent, live, err := read.Absent(ctx, src, d.read, read.SnapshotScope{
			Source: s.Source, Types: s.Types, Carried: carried, CompletedW: ledger.Watermark(s.CompletedW),
		})
		if err != nil {
			writeReadError(w, err)
			return
		}
		n := len(absent)
		rep.Live, rep.Absent = &live, &n
		for _, e := range absent[:min(n, api.AbsentIDsMax)] {
			rep.AbsentIDs = append(rep.AbsentIDs, e.FacetID)
		}
	}
	writeJSON(w, http.StatusOK, rep)
}
