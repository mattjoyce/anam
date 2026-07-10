package ledger

import (
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	ddbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/facet"
)

// facetItem and recordItem are marshalling envelopes, quarantined to this package —
// not domain types (ARCHITECTURE.md §8). The kind discriminator exists only at this
// edge. Current status is absent by design: it is folded from record-events, and an
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

	EventTime    string `dynamodbav:"event_time"`
	RecordedTime string `dynamodbav:"recorded_time"`
	IngestedTime string `dynamodbav:"ingested_time"`

	BindingPersonID  string `dynamodbav:"binding_person_id"`
	BindingAssurance string `dynamodbav:"binding_assurance"`
	BindingSpanHint  string `dynamodbav:"binding_span_hint,omitempty"`

	ProvSource    string `dynamodbav:"prov_source"`
	ProvInjector  string `dynamodbav:"prov_injector"`
	ProvSourceRef string `dynamodbav:"prov_source_ref"`
	ProvMethod    string `dynamodbav:"prov_method,omitempty"`
	ProvModel     string `dynamodbav:"prov_model,omitempty"`
	ProvWatermark string `dynamodbav:"prov_watermark,omitempty"`

	BirthStatus string `dynamodbav:"birth_status"`

	SourceSchema  string `dynamodbav:"source_schema"`
	SchemaVersion int    `dynamodbav:"schema_version"`
	ContentHash   string `dynamodbav:"content_hash"`
	PayloadKind   string `dynamodbav:"payload_kind"`
	MediaType     string `dynamodbav:"media_type"`

	Payload []byte `dynamodbav:"payload"`
}

type recordItem struct {
	PK   string `dynamodbav:"pk"`
	SK   string `dynamodbav:"sk"`
	Kind string `dynamodbav:"kind"`

	EventKind            string `dynamodbav:"event_kind"`
	FacetID              string `dynamodbav:"facet_id"`
	RecordTime           string `dynamodbav:"record_time"`
	Seq                  int64  `dynamodbav:"seq"`
	ArtifactVersion      int    `dynamodbav:"artifact_version"`
	PriorArtifactVersion int    `dynamodbav:"prior_artifact_version,omitempty"`
}

func toFacetItem(pk string, f facet.Facet) facetItem {
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

		EventTime:    clock.Format(f.EventTime),
		RecordedTime: clock.Format(f.RecordedTime),
		IngestedTime: clock.Format(f.IngestedTime),

		BindingPersonID:  f.Binding.PersonID,
		BindingAssurance: string(f.Binding.Assurance),
		BindingSpanHint:  f.Binding.SpanHint,

		ProvSource:    f.Provenance.Source,
		ProvInjector:  f.Provenance.Injector,
		ProvSourceRef: f.Provenance.SourceRef,
		ProvMethod:    f.Provenance.Method,
		ProvModel:     f.Provenance.Model,
		ProvWatermark: f.Provenance.Watermark,

		BirthStatus: string(f.BirthStatus),

		SourceSchema:  f.Descriptor.SourceSchema,
		SchemaVersion: f.Descriptor.SchemaVersion,
		ContentHash:   f.Descriptor.ContentHash,
		PayloadKind:   string(f.Descriptor.PayloadKind),
		MediaType:     f.Descriptor.MediaType,

		Payload: f.Payload.Bytes(),
	}
}

func fromFacetItem(it facetItem) (facet.Facet, error) {
	eventTime, err := parseTime("event_time", it.EventTime)
	if err != nil {
		return facet.Facet{}, err
	}
	recordedTime, err := parseTime("recorded_time", it.RecordedTime)
	if err != nil {
		return facet.Facet{}, err
	}
	ingestedTime, err := parseTime("ingested_time", it.IngestedTime)
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

		EventTime:    eventTime,
		RecordedTime: recordedTime,
		IngestedTime: ingestedTime,

		Binding: facet.Binding{
			PersonID:  it.BindingPersonID,
			Assurance: facet.BindingAssurance(it.BindingAssurance),
			SpanHint:  it.BindingSpanHint,
		},
		Provenance: facet.Provenance{
			Source:    it.ProvSource,
			Injector:  it.ProvInjector,
			SourceRef: it.ProvSourceRef,
			Method:    it.ProvMethod,
			Model:     it.ProvModel,
			Watermark: it.ProvWatermark,
		},

		BirthStatus: facet.BirthStatus(it.BirthStatus),

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

func toRecordItem(pk string, ev facet.RecordEvent) recordItem {
	return recordItem{
		PK:   pk,
		SK:   recSK(ev.Seq),
		Kind: "rec",

		EventKind:            string(ev.Kind),
		FacetID:              ev.FacetID,
		RecordTime:           clock.Format(ev.RecordTime),
		Seq:                  ev.Seq,
		ArtifactVersion:      ev.ArtifactVersion,
		PriorArtifactVersion: ev.PriorArtifactVersion,
	}
}

func fromRecordItem(it recordItem) (facet.RecordEvent, error) {
	recordTime, err := parseTime("record_time", it.RecordTime)
	if err != nil {
		return facet.RecordEvent{}, err
	}
	return facet.RecordEvent{
		Kind:                 facet.RecordEventKind(it.EventKind),
		FacetID:              it.FacetID,
		RecordTime:           recordTime,
		Seq:                  it.Seq,
		ArtifactVersion:      it.ArtifactVersion,
		PriorArtifactVersion: it.PriorArtifactVersion,
	}, nil
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
