// Package healthdb is the capture worker for the unified health database
// (Garmin + Withings): it reads the source in its native format and composes
// Envelopes — the only ingress to the gate (ARCHITECTURE.md §6; MVP.md §5).
//
// A capture worker is deliberately dumb and disposable: it composes, it never
// admits. Everything it produces goes through the same three-pillar gate as any
// other source, and the worker can crash mid-batch and simply be re-run — commit
// is idempotent, so known-good is wherever it got to.
//
// This package reads a SQLite file because the source is one. That is the
// source's substrate, not Anam's (the check.sh carve-out); the ledger's SDK never
// appears here.
package healthdb

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"

	_ "modernc.org/sqlite" // registers the pure-Go "sqlite" driver

	"github.com/mattjoyce/anam/internal/facet"
)

// Source is this worker's identity: the token the gate scopes and the provenance
// every composed facet carries.
const Source = "health-db"

// typeFor maps a source metric group to its facet type (MVP.md §5): three groups
// have earned their own type; the rest ride the generic metric-group until finer
// access control or time semantics earn them a cut.
func typeFor(group string) facet.Type {
	switch group {
	case "sleep":
		return "health.sleep"
	case "body":
		return "health.body-composition"
	case "heart":
		return "health.heart-rate"
	default:
		return "health.metric-group"
	}
}

// payload is one facet's content: a period's metric rows for one group, verbatim.
//
// Deliberately absent: the source's last_updated. It is the sync pipeline's clock,
// not a health fact — a Garmin re-sync bumps it without changing a value, and
// carrying it would turn every re-scan into a spurious supersession. Content is
// the values; identity is (group, period_type, period_start).
type payload struct {
	Group       string            `json:"group"`
	PeriodType  string            `json:"period_type"`
	PeriodStart string            `json:"period_start"`
	Metrics     map[string]metric `json:"metrics"`
}

// metric carries one row's aggregates. Nulls stay null (a single-sample period
// has no min/max), exactly as the source holds them.
type metric struct {
	Unit    string   `json:"unit"`
	Avg     *float64 `json:"avg"`
	Min     *float64 `json:"min"`
	Max     *float64 `json:"max"`
	Samples *int64   `json:"samples"`
}

// Scan reads the source database and composes one Envelope per (group,
// period_type, period_start), in deterministic order. assertedPerson is stamped on
// every envelope — the source is the person's own database, so the binding
// resolves high (§6).
//
// bp_* metrics are excluded from health.heart-rate (MVP.md §5): a blood-pressure
// reading is bp-csv's fact at its own reading time; the health-db copy is a lossy
// aggregate that would enter the timeline as a duplicate with worse provenance.
func Scan(ctx context.Context, dbPath, assertedPerson string) ([]facet.Envelope, error) {
	db, err := sql.Open("sqlite", "file:"+dbPath+"?mode=ro")
	if err != nil {
		return nil, fmt.Errorf("healthdb: open %q: %w", dbPath, err)
	}
	defer func() { _ = db.Close() }()

	groups, err := readGroups(ctx, db)
	if err != nil {
		return nil, err
	}

	envelopes := make([]facet.Envelope, 0, len(groups))
	for _, p := range groups {
		raw, err := json.Marshal(p)
		if err != nil {
			return nil, fmt.Errorf("healthdb: marshal %s %s %s: %w", p.Group, p.PeriodType, p.PeriodStart, err)
		}
		envelopes = append(envelopes, facet.Envelope{
			Type:     typeFor(p.Group),
			Source:   Source,
			Injector: Source,
			// The gate recomputes this from the definition's identity_key and
			// rejects a mismatch — the worker must compose the same string.
			SourceRef:      Source + ":" + p.Group + ":" + p.PeriodType + ":" + p.PeriodStart,
			Payload:        raw,
			AssertedPerson: assertedPerson,
		})
	}
	return envelopes, nil
}

// readGroups scans the summaries and assembles them into per-(group, period)
// payloads, sorted by (group, period_type, period_start).
func readGroups(ctx context.Context, db *sql.DB) ([]payload, error) {
	// period_start is CAST to TEXT so the source's own lexeme reaches the payload.
	// The column is declared DATE, and left alone the driver would parse it and
	// hand back its own re-formatting ("...T00:00:00Z") — a value Anam would then
	// hold that the source never wrote.
	rows, err := db.QueryContext(ctx, `
		SELECT m.group_id, s.period_type, CAST(s.period_start AS TEXT), s.metric_id, m.unit,
		       s.avg_value, s.min_value, s.max_value, s.sample_count
		FROM summaries s
		JOIN metrics m ON m.metric_id = s.metric_id
		WHERE NOT (m.group_id = 'heart' AND s.metric_id LIKE 'bp\_%' ESCAPE '\')
		ORDER BY m.group_id, s.period_type, s.period_start, s.metric_id`)
	if err != nil {
		return nil, fmt.Errorf("healthdb: query summaries: %w", err)
	}
	defer func() { _ = rows.Close() }()

	byKey := map[string]*payload{}
	for rows.Next() {
		var group, periodType, periodStart, metricID, unit string
		var avg, minV, maxV sql.NullFloat64
		var samples sql.NullInt64
		if err := rows.Scan(&group, &periodType, &periodStart, &metricID, &unit,
			&avg, &minV, &maxV, &samples); err != nil {
			return nil, fmt.Errorf("healthdb: scan summary row: %w", err)
		}

		key := group + ":" + periodType + ":" + periodStart
		p, ok := byKey[key]
		if !ok {
			p = &payload{Group: group, PeriodType: periodType, PeriodStart: periodStart,
				Metrics: map[string]metric{}}
			byKey[key] = p
		}
		p.Metrics[metricID] = metric{
			Unit: unit,
			Avg:  nullFloat(avg), Min: nullFloat(minV), Max: nullFloat(maxV),
			Samples: nullInt(samples),
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("healthdb: iterate summaries: %w", err)
	}

	out := make([]payload, 0, len(byKey))
	for _, p := range byKey {
		out = append(out, *p)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Group != out[j].Group {
			return out[i].Group < out[j].Group
		}
		if out[i].PeriodType != out[j].PeriodType {
			return out[i].PeriodType < out[j].PeriodType
		}
		return out[i].PeriodStart < out[j].PeriodStart
	})
	return out, nil
}

func nullFloat(v sql.NullFloat64) *float64 {
	if !v.Valid {
		return nil
	}
	return &v.Float64
}

func nullInt(v sql.NullInt64) *int64 {
	if !v.Valid {
		return nil
	}
	return &v.Int64
}
