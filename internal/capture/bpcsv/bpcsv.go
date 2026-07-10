// Package bpcsv is the capture worker for the blood-pressure CSV export: one
// Envelope per reading row (MVP.md §5), values kept as the source's own strings.
//
// The export is messy in ways the worker absorbs rather than corrects: fields are
// padded with tabs and spaces, strings carry their own quotes, lines end CRLF, and
// several distinct readings can share one timestamp (the device takes them a
// minute apart or faster). What it never does is touch a value — "137.0" reaches
// the ledger as the export wrote it, typos in comments included.
package bpcsv

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/mattjoyce/anam/internal/facet"
)

// Source is this worker's identity: the token the gate scopes and the provenance
// every composed facet carries.
const Source = "bp-csv"

// Type is the one facet type this source emits.
const Type facet.Type = "health.blood-pressure"

// Scan reads the export and composes one Envelope per reading, in file order.
//
// Identity is (date_excel, reading): the export carries no reading id, and
// distinct readings genuinely share a timestamp, so the row's position within its
// timestamp is part of WHICH reading it is. The ordinal rides in the payload as
// "reading" because identity is definition-owned (§6a) — the gate recomposes
// source_ref from the payload, so everything the identity needs must be in it.
func Scan(path, assertedPerson string) ([]facet.Envelope, error) {
	f, err := os.Open(path) // #nosec G304 -- the path IS the input: the operator names their own export via --csv
	if err != nil {
		return nil, fmt.Errorf("bpcsv: open %q: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	r := csv.NewReader(f)
	r.TrimLeadingSpace = true // the export pads every field with "\t "
	r.LazyQuotes = true       // quotes appear mid-field, after the padding
	r.FieldsPerRecord = -1

	records, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("bpcsv: parse %q: %w", path, err)
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("bpcsv: %q has no header row", path)
	}

	keys := make([]string, len(records[0]))
	for i, h := range records[0] {
		keys[i] = slug(h)
	}

	envelopes := make([]facet.Envelope, 0, len(records)-1)
	ordinal := map[string]int{}
	for n, rec := range records[1:] {
		row := map[string]string{}
		for i, cell := range rec {
			if i >= len(keys) || keys[i] == "" {
				continue
			}
			if v := clean(cell); v != "" {
				row[keys[i]] = v
			}
		}
		if len(row) == 0 {
			continue // a blank line, not a reading
		}

		ts, ok := row["date_excel"]
		if !ok {
			return nil, fmt.Errorf("bpcsv: row %d has no date_excel", n+2)
		}
		ordinal[ts]++
		reading := strconv.Itoa(ordinal[ts])
		row["reading"] = reading

		// Escaping in transit is fine: the gate decodes and canonicalises, so the
		// value bytes that reach the ledger are the source's.
		payload, err := json.Marshal(row)
		if err != nil {
			return nil, fmt.Errorf("bpcsv: row %d: %w", n+2, err)
		}
		envelopes = append(envelopes, facet.Envelope{
			Type:           Type,
			Source:         Source,
			Injector:       Source,
			SourceRef:      Source + ":" + ts + ":" + reading,
			Payload:        payload,
			AssertedPerson: assertedPerson,
		})
	}
	return envelopes, nil
}

// slug derives a payload key from a header: "Date (Excel)" → date_excel,
// "Oxygen Saturation [%]" → oxygen_saturation_pct. Deterministic, so a re-export
// with the same headers composes the same keys.
func slug(header string) string {
	s := strings.ToLower(strings.TrimSpace(header))
	s = strings.ReplaceAll(s, "%", "pct")
	var b strings.Builder
	lastUnderscore := true // suppress a leading underscore
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastUnderscore = false
		default:
			if !lastUnderscore {
				b.WriteByte('_')
				lastUnderscore = true
			}
		}
	}
	return strings.TrimSuffix(b.String(), "_")
}

// clean strips the export's packaging from a cell — padding and the surrounding
// quotes — and nothing else. The value inside is the source's, verbatim.
func clean(cell string) string {
	v := strings.Trim(cell, " \t\r")
	if len(v) >= 2 && v[0] == '"' && v[len(v)-1] == '"' {
		v = v[1 : len(v)-1]
	}
	return v
}
