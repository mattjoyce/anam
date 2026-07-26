package ledger

import (
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	ddbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/facet"
)

// facetItem and anamEventItem are marshalling envelopes, quarantined to this package —
// not domain types (ARCHITECTURE.md §8). The kind discriminator exists only at this
// edge. Current status is absent by design: it is folded from anam-events, and an
// artifact has nowhere to carry one.
//
// The payload is a Binary attribute, never a Map: this is where the metadata/payload
// separation of §3 is physically enforced. Timestamps are fixed-width strings
// (clock.Layout) so that lexical comparison anywhere downstream stays chronological.
type facetItem struct {
	PK   string `dynamodbav:"pk"`
	SK   string `dynamodbav:"sk"`
	Kind string `dynamodbav:"kind"`

	ID                string  `dynamodbav:"id"`
	Type              string  `dynamodbav:"type"`
	Confidence        float64 `dynamodbav:"confidence"`
	Classification    string  `dynamodbav:"classification"`
	ArtifactVersion   int     `dynamodbav:"artifact_version"`
	DefinitionVersion int     `dynamodbav:"definition_version"`

	FactTime string `dynamodbav:"fact_time"`

	BindingPersonID  string `dynamodbav:"binding_person_id"`
	BindingAssurance string `dynamodbav:"binding_assurance"`
	BindingSpanHint  string `dynamodbav:"binding_span_hint,omitempty"`

	ProvSource    string `dynamodbav:"prov_source"`
	ProvInjector  string `dynamodbav:"prov_injector"`
	ProvSourceRef string `dynamodbav:"prov_source_ref"`
	ProvMethod    string `dynamodbav:"prov_method,omitempty"`
	ProvModel     string `dynamodbav:"prov_model,omitempty"`
	ProvWatermark string `dynamodbav:"prov_watermark,omitempty"`

	SourceSchema  string `dynamodbav:"source_schema"`
	SchemaVersion int    `dynamodbav:"schema_version"`
	ContentHash   string `dynamodbav:"content_hash"`
	PayloadKind   string `dynamodbav:"payload_kind"`
	MediaType     string `dynamodbav:"media_type"`

	Payload []byte `dynamodbav:"payload"`
}

type anamEventItem struct {
	PK   string `dynamodbav:"pk"`
	SK   string `dynamodbav:"sk"`
	Kind string `dynamodbav:"kind"`

	EventKind            string `dynamodbav:"event_kind"`
	FacetID              string `dynamodbav:"facet_id"`
	AnamTime             string `dynamodbav:"anam_time"`
	Seq                  int64  `dynamodbav:"seq"`
	ArtifactVersion      int    `dynamodbav:"artifact_version"`
	PriorArtifactVersion int    `dynamodbav:"prior_artifact_version,omitempty"`
}

func toFacetItem(pk string, f facet.Facet) facetItem {
	// Derivation is optional on the domain type; the item flattens it to three
	// omitempty attributes.
	var method, model, watermark string
	if d := f.Provenance.Derivation; d != nil {
		method, model, watermark = d.Method, d.Model, d.Watermark
	}
	return facetItem{
		PK:   pk,
		SK:   facetSK(f.ID, f.ArtifactVersion),
		Kind: "facet",

		ID:                f.ID,
		Type:              string(f.Type),
		Confidence:        f.Confidence,
		Classification:    string(f.Classification),
		ArtifactVersion:   f.ArtifactVersion,
		DefinitionVersion: f.DefinitionVersion,

		FactTime: clock.Format(f.FactTime),

		BindingPersonID:  f.Binding.PersonID,
		BindingAssurance: string(f.Binding.Assurance),
		BindingSpanHint:  f.Binding.SpanHint,

		ProvSource:    f.Provenance.Source,
		ProvInjector:  f.Provenance.Injector,
		ProvSourceRef: f.Provenance.SourceRef,
		ProvMethod:    method,
		ProvModel:     model,
		ProvWatermark: watermark,

		SourceSchema:  f.Descriptor.SourceSchema,
		SchemaVersion: f.Descriptor.SchemaVersion,
		ContentHash:   f.Descriptor.ContentHash,
		PayloadKind:   string(f.Descriptor.PayloadKind),
		MediaType:     f.Descriptor.MediaType,

		Payload: f.Payload.Bytes(),
	}
}

func fromFacetItem(it facetItem) (facet.Facet, error) {
	factTime, err := parseTime("fact_time", it.FactTime)
	if err != nil {
		return facet.Facet{}, err
	}

	return facet.Facet{
		ID:                it.ID,
		Type:              facet.Type(it.Type),
		Confidence:        it.Confidence,
		Classification:    facet.Classification(it.Classification),
		ArtifactVersion:   it.ArtifactVersion,
		DefinitionVersion: it.DefinitionVersion,

		FactTime: factTime,

		Binding: facet.Binding{
			PersonID:  it.BindingPersonID,
			Assurance: facet.BindingAssurance(it.BindingAssurance),
			SpanHint:  it.BindingSpanHint,
		},
		Provenance: facet.Provenance{
			Source:     it.ProvSource,
			Injector:   it.ProvInjector,
			SourceRef:  it.ProvSourceRef,
			Derivation: derivationFromItem(it),
		},

		Descriptor: facet.Descriptor{
			SourceSchema:  it.SourceSchema,
			SchemaVersion: it.SchemaVersion,
			ContentHash:   it.ContentHash,
			PayloadKind:   facet.PayloadKind(it.PayloadKind),
			MediaType:     it.MediaType,
		},

		// The stored bytes are the canonical bytes a previous Canonicalize wrote; this
		// is the store layer's re-entry point that CanonicalFromStored exists for.
		Payload: facet.CanonicalFromStored(it.Payload),
	}, nil
}

func toAnamEventItem(pk string, ev facet.AnamEvent) anamEventItem {
	return anamEventItem{
		PK:   pk,
		SK:   anamSK(ev.Seq),
		Kind: "rec",

		EventKind:            string(ev.Kind),
		FacetID:              ev.FacetID,
		AnamTime:             clock.Format(ev.AnamTime),
		Seq:                  ev.Seq,
		ArtifactVersion:      ev.ArtifactVersion,
		PriorArtifactVersion: ev.PriorArtifactVersion,
	}
}

func fromAnamEventItem(it anamEventItem) (facet.AnamEvent, error) {
	anamTime, err := parseTime("anam_time", it.AnamTime)
	if err != nil {
		return facet.AnamEvent{}, err
	}
	return facet.AnamEvent{
		Kind:                 facet.AnamEventKind(it.EventKind),
		FacetID:              it.FacetID,
		AnamTime:             anamTime,
		Seq:                  it.Seq,
		ArtifactVersion:      it.ArtifactVersion,
		PriorArtifactVersion: it.PriorArtifactVersion,
	}, nil
}

// derivationFromItem rehydrates the optional Derivation: present iff any of its three
// attributes were stored.
func derivationFromItem(it facetItem) *facet.Derivation {
	if it.ProvMethod == "" && it.ProvModel == "" && it.ProvWatermark == "" {
		return nil
	}
	return &facet.Derivation{Method: it.ProvMethod, Model: it.ProvModel, Watermark: it.ProvWatermark}
}

// parseTime is strict: a timestamp the ledger holds was written by clock.Format, and
// one at any other width is not Anam's — reject it rather than guess.
func parseTime(attr, s string) (time.Time, error) {
	t, err := clock.Parse(s)
	if err != nil {
		return time.Time{}, fmt.Errorf("ledger: item attribute %s: %w", attr, err)
	}
	return t, nil
}

func marshalItem(v any) (map[string]ddbtypes.AttributeValue, error) {
	m, err := attributevalue.MarshalMap(v)
	if err != nil {
		return nil, fmt.Errorf("ledger: marshal item: %w", err)
	}
	return m, nil
}
