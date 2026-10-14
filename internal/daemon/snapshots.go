package daemon

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
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

// snapshotID is the shape of a snapshot's label. It is an invariant (card 151): the
// importers mint labels to it, and a looser one here would admit ids they cannot read
// back in a report's URL.
var snapshotID = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,128}$`)

// validSnapshot checks a part's label against the importer and the batch it labels,
// and its declared parts against maxParts (max_snapshot_parts).
func validSnapshot(s *api.SnapshotPart, imp config.Importer, envs []api.SubmittedEnvelope, maxParts int) error {
	if s == nil {
		return nil
	}
	switch {
	case !snapshotID.MatchString(s.ID):
		return fmt.Errorf("snapshot id %q: 1 to 128 of letters, digits and . _ : -", s.ID)
	case s.Parts < 1 || s.Parts > maxParts:
		return fmt.Errorf("snapshot parts %d: want 1 to %d", s.Parts, maxParts)
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

// partDigest is a part's content identity (card 107): a hash over its envelopes'
// canonical forms as a sorted set, so an exact retry matches in whatever order it
// sends them, and a part carrying other envelopes does not. It is a function of the
// request alone, needing no Definition and no gate result, so a changed part can be
// refused before the ledger is touched. A payload that will not canonicalise is hashed
// as sent: the gate refuses the batch, and the part is never recorded.
func partDigest(envs []facet.Envelope) string {
	lines := make([]string, len(envs))
	for i, e := range envs {
		payload := e.Payload
		if c, err := facet.Canonicalize(e.Payload); err == nil {
			payload = c.Bytes()
		}
		lines[i] = strings.Join([]string{string(e.Type), strconv.Itoa(e.DefinitionVersion), e.SourceRef,
			e.AssertedPerson, e.SpanHint, string(payload)}, "\x00")
	}
	sort.Strings(lines)
	h := sha256.New()
	for _, l := range lines {
		_, _ = fmt.Fprintf(h, "%d:%s\n", len(l), l)
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}

// checkSnapshot refuses a part whose id names a run begun with another source, other
// types or another part count, and a part number already recorded with other
// envelopes. It runs before admission, inside the write, so a refused part admits
// nothing; the store checks again when the part is recorded.
func checkSnapshot(ctx context.Context, db *ops.DB, source string, s *api.SnapshotPart, digest string) error {
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
	recorded, ok, err := db.SnapshotPartDigest(ctx, s.ID, s.Part)
	if err != nil {
		return err
	}
	if ok && recorded != digest {
		return fmt.Errorf("%w: %s part %d", ops.ErrSnapshotPartChanged, s.ID, s.Part)
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

// recordSnapshotPart records an admitted part, under its digest, and the facet ids its
// results carried, every outcome counted, no-op included: a no-op is a facet the
// source still holds.
func recordSnapshotPart(ctx context.Context, db *ops.DB, source string, s *api.SnapshotPart, digest string,
	results []gate.Result, head ledger.Watermark, at time.Time, keep ops.SnapshotRetention,
) (*api.SnapshotProgress, error) {
	ids := make([]string, len(results))
	for i, r := range results {
		ids[i] = r.FacetID
	}
	rec, err := db.RecordSnapshotPart(ctx, ops.SnapshotPart{
		ID: s.ID, Source: source, Types: s.Types, Part: s.Part, Parts: s.Parts, Digest: digest, FacetIDs: ids,
	}, at, int64(head), keep)
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
		Source:           s.Source, Types: s.Types, Carried: s.Carried, StartedAt: clock.Format(s.StartedAt), StartedW: s.StartedW,
	}
	if s.Complete() {
		rep.CompletedAt, rep.CompletedW = clock.Format(s.CompletedAt), s.CompletedW
		carried, err := d.readOps.SnapshotFacets(ctx, s.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		src := read.Source{Ledger: d.read, Checkpoints: d.readOps, Artifacts: d.artifacts, Definitions: d.defs.current.Load(), Projections: d.projections.current.Load()}
		absent, live, err := read.Absent(ctx, src, d.read, read.SnapshotScope{
			Source: s.Source, Types: s.Types, Carried: carried, StartedW: ledger.Watermark(s.StartedW),
			MaxJudged: d.cfg.MaxAbsentJudged,
		})
		if err != nil {
			writeReadError(w, err)
			return
		}
		n := len(absent)
		rep.Live, rep.Absent = &live, &n
		for _, e := range absent[:min(n, d.cfg.MaxAbsentIDs)] {
			rep.AbsentIDs = append(rep.AbsentIDs, e.FacetID)
		}
	}
	writeJSON(w, http.StatusOK, rep)
}
