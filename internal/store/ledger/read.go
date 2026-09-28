package ledger

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

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
