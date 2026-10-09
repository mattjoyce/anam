package ledger

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/mattjoyce/anam/internal/facet"
)

// ErrNotFound is returned by CurrentVersion and Version for an artifact the ledger
// does not hold.
var ErrNotFound = errors.New("ledger: facet not found")

// The reads below feed the error kernel: CurrentVersion is what commit compares
// content hashes against, and Replay is what rebuilds every derived index. Each is one
// statement against one file, so each sees one committed state of the ledger.

// KnownFacets reports which of the given facet ids the ledger has ever admitted.
//
// It is a fast negative, and only that. Absent means the facet is unknown and its
// first artifact will be version 1. That is certain because versions start at 1, no
// row is ever deleted, and version n cannot be written without n-1 beneath it, all
// three held by the schema. Present means only that v1 exists; which version is
// current still needs CurrentVersion.
//
// A backfill is almost entirely first sightings, so the probe answers the disposition
// of a whole batch and the per-facet lookup is never issued.
func (l *Ledger) KnownFacets(ctx context.Context, facetIDs []string) (map[string]bool, error) {
	known := make(map[string]bool, len(facetIDs))
	if len(facetIDs) == 0 {
		return known, nil
	}

	// One prepared statement, one lookup per id: SQLite is in-process, so point reads
	// on the primary key cost less than assembling an IN clause.
	stmt, err := l.db.PrepareContext(ctx,
		`SELECT 1 FROM facet WHERE facet_id = ? AND artifact_version = 1`)
	if err != nil {
		return nil, classify("prepare probe", err)
	}
	defer func() { _ = stmt.Close() }()

	for _, id := range facetIDs {
		if known[id] {
			continue
		}
		var one int
		err := stmt.QueryRowContext(ctx, id).Scan(&one)
		switch {
		case err == nil:
			known[id] = true
		case errors.Is(err, sql.ErrNoRows):
		default:
			return nil, classify("probe "+id, err)
		}
	}
	return known, nil
}

// CurrentVersion returns the highest artifact version of one logical facet.
//
// The lookup is by identity alone. Fact-time is a value read out of the payload, and
// keying on it would let a corrected row land where the prior-version lookup cannot
// see it (§8).
func (l *Ledger) CurrentVersion(ctx context.Context, facetID string) (facet.Facet, error) {
	row := l.db.QueryRowContext(ctx, `SELECT `+facetColumns+` FROM facet
		WHERE facet_id = ? ORDER BY artifact_version DESC LIMIT 1`, facetID)
	f, err := scanFacet(row)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return facet.Facet{}, fmt.Errorf("%w: %s", ErrNotFound, facetID)
	case err != nil:
		return facet.Facet{}, classify("read current version of "+facetID, err)
	}
	return f, nil
}

// Version returns one exact artifact version of one logical facet.
//
// CurrentVersion answers "the latest", which is the right question for the gate. It is
// the wrong question for a read at a historical watermark, where the fold has already
// derived which version was in force then, and the latest may be two supersessions
// further on.
//
// An artifact version is immutable once written, so this lookup cannot be raced: the
// answer to "give me v2 of this facet" is the same whether ingest is idle or busy.
// That is what lets the as-of read run without a retry (see fold.ReadAt).
func (l *Ledger) Version(ctx context.Context, facetID string, version int) (facet.Facet, error) {
	if version < 1 {
		return facet.Facet{}, fmt.Errorf("ledger: artifact version %d is not a version", version)
	}
	row := l.db.QueryRowContext(ctx, `SELECT `+facetColumns+` FROM facet
		WHERE facet_id = ? AND artifact_version = ?`, facetID, version)
	f, err := scanFacet(row)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return facet.Facet{}, fmt.Errorf("%w: %s v%d", ErrNotFound, facetID, version)
	case err != nil:
		return facet.Facet{}, classify(fmt.Sprintf("read %s v%d", facetID, version), err)
	}
	return f, nil
}

// VersionRef names one artifact version of one facet.
type VersionRef struct {
	FacetID string
	Version int
}

// versionsBatch bounds the refs one Versions query carries: two parameters each, well
// inside SQLite's limit on bound parameters.
const versionsBatch = 500

// Versions returns the artifact at each ref, in no particular order. It is Version for
// many facets at once: a read that needs thousands of artifacts asks in batches, each
// one query that walks the primary key, rather than a query per artifact. A ref that
// names no artifact is ErrNotFound.
func (l *Ledger) Versions(ctx context.Context, refs []VersionRef) ([]facet.Facet, error) {
	out := make([]facet.Facet, 0, len(refs))
	for start := 0; start < len(refs); start += versionsBatch {
		batch := refs[start:min(start+versionsBatch, len(refs))]
		got, err := l.versions(ctx, batch)
		if err != nil {
			return nil, err
		}
		out = append(out, got...)
	}
	return out, nil
}

func (l *Ledger) versions(ctx context.Context, refs []VersionRef) ([]facet.Facet, error) {
	var q strings.Builder
	args := make([]any, 0, 2*len(refs))
	q.WriteString(`SELECT ` + qualified("f") + ` FROM (VALUES `)
	for i, r := range refs {
		if r.Version < 1 {
			return nil, fmt.Errorf("ledger: artifact version %d is not a version", r.Version)
		}
		if i > 0 {
			q.WriteString(",")
		}
		q.WriteString("(?,?)")
		args = append(args, r.FacetID, r.Version)
	}
	// The VALUES list drives the join, so each ref is one lookup on the primary key.
	q.WriteString(`) AS v CROSS JOIN facet AS f ON f.facet_id = v.column1 AND f.artifact_version = v.column2`)

	rows, err := l.db.QueryContext(ctx, q.String(), args...)
	if err != nil {
		return nil, classify("read versions", err)
	}
	defer func() { _ = rows.Close() }()
	out := make([]facet.Facet, 0, len(refs))
	for rows.Next() {
		f, err := scanFacet(rows)
		if err != nil {
			return nil, classify("read versions", err)
		}
		out = append(out, f)
	}
	if err := rows.Err(); err != nil {
		return nil, classify("read versions", err)
	}
	if len(out) != len(refs) {
		return nil, missing(refs, out)
	}
	return out, nil
}

// missing names a ref that Versions found no artifact for.
func missing(refs []VersionRef, found []facet.Facet) error {
	have := make(map[VersionRef]bool, len(found))
	for _, f := range found {
		have[VersionRef{FacetID: f.ID, Version: f.ArtifactVersion}] = true
	}
	for _, r := range refs {
		if !have[r] {
			return fmt.Errorf("%w: %s v%d", ErrNotFound, r.FacetID, r.Version)
		}
	}
	return fmt.Errorf("%w: %d of %d versions", ErrNotFound, len(refs)-len(found), len(refs))
}

// latestScanAt is the number of facets above which LatestVersions reads the whole
// primary key once instead of asking in batches: past it, one scan of a narrow index
// is cheaper than the lookups.
const latestScanAt = 20000

// LatestVersions returns the newest artifact version of each facet in ids that the
// ledger holds. A facet it does not hold is absent from the map.
func (l *Ledger) LatestVersions(ctx context.Context, ids []string) (map[string]int, error) {
	out := make(map[string]int, len(ids))
	if len(ids) > latestScanAt {
		want := make(map[string]bool, len(ids))
		for _, id := range ids {
			want[id] = true
		}
		return out, l.latestVersions(ctx,
			`SELECT facet_id, MAX(artifact_version) FROM facet GROUP BY facet_id`, nil,
			func(id string, v int) {
				if want[id] {
					out[id] = v
				}
			})
	}
	for start := 0; start < len(ids); start += versionsBatch {
		batch := ids[start:min(start+versionsBatch, len(ids))]
		args := make([]any, len(batch))
		for i, id := range batch {
			args[i] = id
		}
		q := `SELECT facet_id, MAX(artifact_version) FROM facet WHERE facet_id IN (?` +
			strings.Repeat(",?", len(batch)-1) + `) GROUP BY facet_id`
		if err := l.latestVersions(ctx, q, args, func(id string, v int) { out[id] = v }); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (l *Ledger) latestVersions(ctx context.Context, q string, args []any, each func(string, int)) error {
	rows, err := l.db.QueryContext(ctx, q, args...)
	if err != nil {
		return classify("read latest versions", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id string
		var v int
		if err := rows.Scan(&id, &v); err != nil {
			return classify("read latest versions", err)
		}
		each(id, v)
	}
	if err := rows.Err(); err != nil {
		return classify("read latest versions", err)
	}
	return nil
}

// Replay returns the anam-events in (sinceW, uptoW], in seq order: the range scan
// that every derived state is rebuilt from (§7). Pass the head watermark as uptoW for
// "everything since"; pass a historical watermark for an as-of bound.
func (l *Ledger) Replay(ctx context.Context, sinceW, uptoW Watermark) ([]facet.AnamEvent, error) {
	if uptoW <= sinceW {
		return nil, nil
	}
	doing := fmt.Sprintf("replay (%d, %d]", sinceW, uptoW)

	rows, err := l.db.QueryContext(ctx, `SELECT `+eventColumns+` FROM anam_event
		WHERE seq > ? AND seq <= ? ORDER BY seq`, int64(sinceW), int64(uptoW))
	if err != nil {
		return nil, classify(doing, err)
	}
	defer func() { _ = rows.Close() }()

	var events []facet.AnamEvent
	for rows.Next() {
		ev, err := scanEvent(rows)
		if err != nil {
			return nil, classify(doing, err)
		}
		events = append(events, ev)
	}
	if err := rows.Err(); err != nil {
		return nil, classify(doing, err)
	}
	return events, nil
}

// FacetEvents returns one facet's anam-events with seq at or below uptoW, in seq
// order: its whole history to that watermark, and nothing of any other facet. A
// facet's folded state depends only on its own events (fold.Apply), so folding these
// gives the same State for it as folding the ledger. It is one statement, so it sees
// one committed state; the anam_event_by_facet index makes its cost that facet's
// history, not the ledger's.
func (l *Ledger) FacetEvents(ctx context.Context, facetID string, uptoW Watermark) ([]facet.AnamEvent, error) {
	doing := fmt.Sprintf("read the events of %s to %d", facetID, uptoW)
	rows, err := l.db.QueryContext(ctx, `SELECT `+eventColumns+` FROM anam_event
		WHERE facet_id = ? AND seq <= ? ORDER BY seq`, facetID, int64(uptoW))
	if err != nil {
		return nil, classify(doing, err)
	}
	defer func() { _ = rows.Close() }()

	var events []facet.AnamEvent
	for rows.Next() {
		ev, err := scanEvent(rows)
		if err != nil {
			return nil, classify(doing, err)
		}
		events = append(events, ev)
	}
	if err := rows.Err(); err != nil {
		return nil, classify(doing, err)
	}
	return events, nil
}
