package healthdb

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/mattjoyce/anam/internal/facet"
)

// AlertType is the facet type of a heart rate alert the watch raised.
const AlertType facet.Type = "health.heart-rate-alert"

// The source's events table is written by the ETL from the watch's FIT files
// (ductile-healthdata fit_events.py). Only its hr_high_alert rows are heart rate
// alerts; any other kind stays in the source until a definition earns it.

// alert is one heart rate alert, as the source holds it.
//
// Deliberately absent: the source's source_file and first_seen. They record which
// FIT file the ETL read and when, not what the watch said, and a rebuilt source
// could change either without the alert changing. Identity is (source, kind,
// fact_time).
type alert struct {
	Source    string  `json:"source"`
	Kind      string  `json:"kind"`
	FactTime  string  `json:"fact_time"`
	HeartRate float64 `json:"heart_rate"`
	Threshold float64 `json:"threshold"`
	Unit      string  `json:"unit"`
}

// scanAlerts composes one Envelope per heart rate alert, ordered by fact_time.
func scanAlerts(ctx context.Context, db *sql.DB, assertedPerson string) ([]facet.Envelope, error) {
	alerts, err := readAlerts(ctx, db)
	if err != nil {
		return nil, err
	}
	envelopes := make([]facet.Envelope, 0, len(alerts))
	for _, a := range alerts {
		raw, err := json.Marshal(a)
		if err != nil {
			return nil, fmt.Errorf("healthdb: marshal alert %s %s: %w", a.Kind, a.FactTime, err)
		}
		envelopes = append(envelopes, facet.Envelope{
			Type:           AlertType,
			Source:         Source,
			Injector:       Source,
			SourceRef:      facet.SourceRef(Source, a.Source, a.Kind, a.FactTime),
			Payload:        raw,
			AssertedPerson: assertedPerson,
		})
	}
	return envelopes, nil
}

// readAlerts reads the alerts from the events table. A source without the table
// predates the ETL that writes it and holds no alerts; that is not an error.
func readAlerts(ctx context.Context, db *sql.DB) ([]alert, error) {
	var n int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'events'`).Scan(&n); err != nil {
		return nil, fmt.Errorf("healthdb: look for events table: %w", err)
	}
	if n == 0 {
		return nil, nil
	}

	// Rows with a NULL value, threshold or unit are skipped by the WHERE clause:
	// an alert without its heart rate is not one the definition can admit.
	rows, err := db.QueryContext(ctx, `
		SELECT source, kind, fact_time, value, threshold, unit
		FROM events
		WHERE kind = 'hr_high_alert'
		  AND value IS NOT NULL AND threshold IS NOT NULL AND unit IS NOT NULL
		ORDER BY fact_time, source`)
	if err != nil {
		return nil, fmt.Errorf("healthdb: query events: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []alert
	for rows.Next() {
		var a alert
		if err := rows.Scan(&a.Source, &a.Kind, &a.FactTime, &a.HeartRate, &a.Threshold, &a.Unit); err != nil {
			return nil, fmt.Errorf("healthdb: scan event row: %w", err)
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("healthdb: iterate events: %w", err)
	}
	return out, nil
}
