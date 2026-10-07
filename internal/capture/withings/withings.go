// Package withings is the capture worker for the Withings measurement database:
// it reads the source in its native format and composes Envelopes — the only
// ingress to the gate (ARCHITECTURE.md §6).
//
// This is Anam's first *atomic* health source. health-db reads `summaries`, a
// table produced by an ETL that runs `AVG/MIN/MAX/COUNT ... GROUP BY` over this
// one; every row it holds is a computation. Here a row is a single measurement,
// so nothing has been derived between the sensor and the Facet.
//
// This package reads a SQLite file because the source is one. That is the
// source's substrate, not Anam's (the check.sh carve-out); the ledger's SDK never
// appears here.
package withings

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	_ "modernc.org/sqlite" // registers the pure-Go "sqlite" driver

	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/safepath"
)

// Source is this worker's identity: the token the gate scopes and the provenance
// every composed facet carries.
const Source = "withings"

// Type is the facet type every Withings measurement composes to. One type, not
// one per metric: the shape is identical across all of them — a single scalar of
// one metric at one instant — and which metric it is lives in the payload, where
// the source put it.
const Type facet.Type = "health.measurement"

// payload is one measurement, verbatim.
//
// Every field is the source's own lexeme, read through CAST(... AS TEXT). The
// columns are typed TIMESTAMP/INTEGER/REAL, and left alone the driver would parse
// and re-utter them — a REAL 99.0 comes back as 99, a TIMESTAMP as its own
// re-formatting. Anam would then hold a value the source never wrote.
//
// Two columns are deliberately absent:
//
//   - id — the local table's AUTOINCREMENT surrogate. It says which row in this
//     copy of the database, not which measurement in the world; a re-import
//     renumbers it. Identity is (withings_id, type), which is the source's own
//     declared UNIQUE constraint.
//   - created_at — the sync pipeline's clock, not a health fact. A re-sync bumps
//     it without changing a measurement, and carrying it in a hashed payload
//     would turn every re-scan into a spurious supersession.
type payload struct {
	WithingsID string `json:"withings_id"`
	Date       string `json:"date"`
	Type       string `json:"type"`
	Value      string `json:"value"`
	Unit       string `json:"unit"`
}

// Scan reads the source database and composes one Envelope per measurement, in
// deterministic order. assertedPerson is stamped on every envelope — the source
// is the person's own database, so the binding resolves high (§6).
func Scan(ctx context.Context, dbPath, assertedPerson string) ([]facet.Envelope, error) {
	// Resolve before the DSN is assembled: the path is operator input, and the DSN
	// is a little language of its own — a "?" smuggled into an unresolved path
	// would be parsed as options, not filename.
	real, err := safepath.ResolveFile(dbPath)
	if err != nil {
		return nil, fmt.Errorf("withings: %w", err)
	}
	db, err := sql.Open("sqlite", "file:"+real+"?mode=ro")
	if err != nil {
		return nil, fmt.Errorf("withings: open %q: %w", real, err)
	}
	defer func() { _ = db.Close() }()

	rows, err := db.QueryContext(ctx, `
		SELECT CAST(withings_id AS TEXT), CAST(date AS TEXT), CAST(type AS TEXT),
		       CAST(value AS TEXT), CAST(unit AS TEXT)
		FROM measurements
		ORDER BY withings_id, type`)
	if err != nil {
		return nil, fmt.Errorf("withings: query measurements: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var envelopes []facet.Envelope
	for rows.Next() {
		var p payload
		if err := rows.Scan(&p.WithingsID, &p.Date, &p.Type, &p.Value, &p.Unit); err != nil {
			return nil, fmt.Errorf("withings: scan measurement row: %w", err)
		}
		raw, err := json.Marshal(p)
		if err != nil {
			return nil, fmt.Errorf("withings: marshal %s:%s: %w", p.WithingsID, p.Type, err)
		}
		envelopes = append(envelopes, facet.Envelope{
			Type:     Type,
			Source:   Source,
			Injector: Source,
			// The gate recomputes this from the definition's identity_key and
			// rejects a mismatch — the worker must compose the same string.
			SourceRef:      facet.SourceRef(Source, p.WithingsID, p.Type),
			Payload:        raw,
			AssertedPerson: assertedPerson,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("withings: iterate measurements: %w", err)
	}
	return envelopes, nil
}
