package ledger

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/facet"
)

// The mapping between domain values and rows, quarantined to this package (§8).
// Current status has no column by design: it is folded from anam-events, and an
// artifact has nowhere to carry one.
//
// The payload is a BLOB, never parsed and never a JSON column. This is where the
// metadata/payload separation of §3 is physically enforced.

// facetColumns is the one column list the insert and every select share, so the
// writer and the scanner cannot disagree about order.
const facetColumns = `facet_id, artifact_version, type, confidence, classification,
	definition_version, fact_time, series,
	binding_person_id, binding_assurance, binding_span_hint,
	prov_source, prov_injector, prov_source_ref, prov_method, prov_model, prov_watermark,
	source_schema, schema_version, content_hash, payload_kind, media_type, payload`

const insertFacetSQL = `INSERT INTO facet (` + facetColumns + `)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

const eventColumns = `seq, kind, facet_id, anam_time, artifact_version, prior_artifact_version`

const insertEventSQL = `INSERT INTO anam_event (` + eventColumns + `) VALUES (?, ?, ?, ?, ?, ?)`

// execer is the write half of what *sql.Tx offers.
type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func insertFacet(ctx context.Context, x execer, f facet.Facet) error {
	// Derivation is optional on the domain type; the row flattens it to three columns
	// that are empty when it is absent.
	var method, model, watermark string
	if d := f.Provenance.Derivation; d != nil {
		method, model, watermark = d.Method, d.Model, d.Watermark
	}
	// A nil slice would bind as NULL. The column is NOT NULL, and an empty payload is
	// still a payload.
	payload := f.Payload.Bytes()
	if payload == nil {
		payload = []byte{}
	}
	_, err := x.ExecContext(ctx, insertFacetSQL,
		f.ID, f.ArtifactVersion, string(f.Type), f.Confidence, string(f.Classification),
		f.DefinitionVersion, clock.Format(f.FactTime), f.Series,
		f.Binding.PersonID, string(f.Binding.Assurance), f.Binding.SpanHint,
		f.Provenance.Source, f.Provenance.Injector, f.Provenance.SourceRef, method, model, watermark,
		f.Descriptor.SourceSchema, f.Descriptor.SchemaVersion, f.Descriptor.ContentHash,
		string(f.Descriptor.PayloadKind), f.Descriptor.MediaType, payload,
	)
	return err
}

func insertEvent(ctx context.Context, x execer, ev facet.AnamEvent) error {
	_, err := x.ExecContext(ctx, insertEventSQL,
		ev.Seq, string(ev.Kind), ev.FacetID, clock.Format(ev.AnamTime),
		ev.ArtifactVersion, ev.PriorArtifactVersion,
	)
	return err
}

// scanner is what *sql.Row and *sql.Rows share.
type scanner interface {
	Scan(dest ...any) error
}

func scanFacet(s scanner) (facet.Facet, error) {
	var (
		f                        facet.Facet
		typ, class, factTime     string
		assurance, payloadKind   string
		method, model, watermark string
		payload                  []byte
	)
	err := s.Scan(
		&f.ID, &f.ArtifactVersion, &typ, &f.Confidence, &class,
		&f.DefinitionVersion, &factTime, &f.Series,
		&f.Binding.PersonID, &assurance, &f.Binding.SpanHint,
		&f.Provenance.Source, &f.Provenance.Injector, &f.Provenance.SourceRef, &method, &model, &watermark,
		&f.Descriptor.SourceSchema, &f.Descriptor.SchemaVersion, &f.Descriptor.ContentHash,
		&payloadKind, &f.Descriptor.MediaType, &payload,
	)
	if err != nil {
		return facet.Facet{}, err
	}

	f.Type = facet.Type(typ)
	f.Classification = facet.Classification(class)
	f.Binding.Assurance = facet.BindingAssurance(assurance)
	f.Descriptor.PayloadKind = facet.PayloadKind(payloadKind)
	// Derivation is present iff any of its three columns was stored.
	if method != "" || model != "" || watermark != "" {
		f.Provenance.Derivation = &facet.Derivation{Method: method, Model: model, Watermark: watermark}
	}
	if f.FactTime, err = parseTime("fact_time", factTime); err != nil {
		return facet.Facet{}, err
	}
	// The stored bytes are the canonical bytes a previous Canonicalize wrote; this is
	// the store layer's re-entry point that CanonicalFromStored exists for.
	f.Payload = facet.CanonicalFromStored(payload)
	return f, nil
}

func scanEvent(s scanner) (facet.AnamEvent, error) {
	var (
		ev             facet.AnamEvent
		kind, anamTime string
	)
	err := s.Scan(&ev.Seq, &kind, &ev.FacetID, &anamTime, &ev.ArtifactVersion, &ev.PriorArtifactVersion)
	if err != nil {
		return facet.AnamEvent{}, err
	}
	ev.Kind = facet.AnamEventKind(kind)
	if ev.AnamTime, err = parseTime("anam_time", anamTime); err != nil {
		return facet.AnamEvent{}, err
	}
	return ev, nil
}

// parseTime is strict: a timestamp the ledger holds was written by clock.Format, and
// one at any other width is not Anam's. Reject it instead of guessing.
func parseTime(column, s string) (time.Time, error) {
	t, err := clock.Parse(s)
	if err != nil {
		return time.Time{}, fmt.Errorf("ledger: column %s: %w", column, err)
	}
	return t, nil
}
