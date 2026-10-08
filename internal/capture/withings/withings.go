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

// payload is one measurement, verbatim, with its group's attribution beside it.
//
// Every field is the source's own lexeme, read through CAST(... AS TEXT). The
// columns are typed TIMESTAMP/INTEGER/REAL, and left alone the driver would parse
// and re-utter them — a REAL 99.0 comes back as 99, a TIMESTAMP as its own
// re-formatting. Anam would then hold a value the source never wrote.
//
// The group fields come from measure_groups, one row per Withings measure group.
// A column the source left NULL is omitted, not written as null: a group from before
// Withings sent timezones has none, and saying so as a string would be inventing one.
//
// Columns deliberately absent:
//
//   - id — the local table's AUTOINCREMENT surrogate. It says which row in this
//     copy of the database, not which measurement in the world; a re-import
//     renumbers it. Identity is (withings_id, type), which is the source's own
//     declared UNIQUE constraint.
//   - created_at — the sync pipeline's clock, not a health fact. A re-sync bumps
//     it without changing a measurement, and carrying it in a hashed payload
//     would turn every re-scan into a spurious supersession.
//   - the group's created and modified — Withings' bookkeeping clocks. A change
//     that matters (attrib, a value) arrives in its own field; carrying modified
//     would supersede a facet for an edit that changed nothing Anam holds.
//   - category — always 1 here; the plugin does not store goals.
type payload struct {
	WithingsID string  `json:"withings_id"`
	Date       string  `json:"date"`
	Type       string  `json:"type"`
	Value      string  `json:"value"`
	Unit       string  `json:"unit"`
	Attrib     *string `json:"attrib,omitempty"`
	Model      *string `json:"model,omitempty"`
	DeviceID   *string `json:"deviceid,omitempty"`
	Timezone   *string `json:"timezone,omitempty"`
}

// attribAmbiguous is Withings' attrib for a reading the device captured but could
// not place: it "may belong to other users as well as this one". Every other value
// says the reading is this account's — 0 and 8 from a device, 2 and 4 entered by
// hand for this user, 5 and 7 confirmed — so they leave the binding where it is.
// Entered by hand is how a reading was taken, not whose it is.
const attribAmbiguous = "1"

// assertion is who the source says the reading belongs to. An ambiguous group
// asserts no one, so the gate's binding falls to medium (defaulted) rather than
// high (the source explicitly scopes it to the person).
func assertion(p payload, person string) string {
	if p.Attrib != nil && *p.Attrib == attribAmbiguous {
		return ""
	}
	return person
}

// Scan reads the source database and composes one Envelope per measurement, in
// deterministic order. assertedPerson is stamped on every envelope whose group
// Withings attributes to the account — the source is the person's own database, so
// the binding resolves high (§6). An ambiguous group asserts no one.
//
// The source must carry measure_groups (the withings plugin 0.4.0, after its
// reconcile has run). Without it Scan fails rather than bind every reading high on
// attribution it cannot see.
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

	if err := requireGroups(ctx, db); err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, `
		SELECT CAST(m.withings_id AS TEXT), CAST(m.date AS TEXT), CAST(m.type AS TEXT),
		       CAST(m.value AS TEXT), CAST(m.unit AS TEXT),
		       CAST(g.attrib AS TEXT), g.model, g.deviceid, g.timezone
		FROM measurements m
		LEFT JOIN measure_groups g ON g.withings_id = m.withings_id
		ORDER BY m.withings_id, m.type`)
	if err != nil {
		return nil, fmt.Errorf("withings: query measurements: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var envelopes []facet.Envelope
	for rows.Next() {
		var p payload
		var attrib, model, deviceID, timezone sql.NullString
		if err := rows.Scan(&p.WithingsID, &p.Date, &p.Type, &p.Value, &p.Unit,
			&attrib, &model, &deviceID, &timezone); err != nil {
			return nil, fmt.Errorf("withings: scan measurement row: %w", err)
		}
		p.Attrib, p.Model, p.DeviceID, p.Timezone = present(attrib), present(model), present(deviceID), present(timezone)
		raw, err := json.Marshal(p)
		if err != nil {
			return nil, fmt.Errorf("withings: marshal %s:%s: %w", p.WithingsID, p.Type, err)
		}
		envelopes = append(envelopes, facet.Envelope{
			Type:           Type,
			Source:         Source,
			Injector:       Source,
			Payload:        raw,
			AssertedPerson: assertion(p, assertedPerson),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("withings: iterate measurements: %w", err)
	}
	return envelopes, nil
}

// requireGroups fails when the source predates measure_groups.
func requireGroups(ctx context.Context, db *sql.DB) error {
	var n int
	err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'measure_groups'`).Scan(&n)
	if err != nil {
		return fmt.Errorf("withings: look for measure_groups: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("withings: the source has no measure_groups table, so attribution is unknown; " +
			"deploy the withings plugin 0.4.0 and run its reconcile command")
	}
	return nil
}

// present is a NULL column as an absent field, and any other value as itself.
func present(s sql.NullString) *string {
	if !s.Valid {
		return nil
	}
	return &s.String
}
