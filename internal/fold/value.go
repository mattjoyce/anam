package fold

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/facet"
)

// Entry is one live facet's contribution to the canonical value.
//
// Every field is rebuild-stable: it comes from the source row itself or from the
// definition applied to it, so drop-the-table → init → re-ingest reproduces it
// byte for byte. That is what makes the value's hash the equality oracle for
// "Anam owns no fact" (MVP.md §7). Deliberately absent, because a rebuild
// reassigns them: the artifact version (a superseded chain collapses to v1), the
// ingested time, and every seq/watermark. The one exception is Status — endorsed
// is Anam-authored — and it is carried anyway because projections gate on it;
// endorsements come back from backup, not replay (§12).
type Entry struct {
	FacetID      string          `json:"facet_id"`
	Type         facet.Type      `json:"type"`
	EventTime    string          `json:"event_time"`
	RecordedTime string          `json:"recorded_time"`
	Source       string          `json:"source"`
	SourceRef    string          `json:"source_ref"`
	Status       facet.Status    `json:"status"`
	ContentHash  string          `json:"content_hash"`
	Payload      json.RawMessage `json:"payload"`
}

// entryOf projects a live facet at its folded state into the value.
func entryOf(f facet.Facet, s State) Entry {
	return Entry{
		FacetID:      f.ID,
		Type:         f.Type,
		EventTime:    clock.Format(f.EventTime),
		RecordedTime: clock.Format(f.RecordedTime),
		Source:       f.Provenance.Source,
		SourceRef:    f.Provenance.SourceRef,
		Status:       s.Status,
		ContentHash:  f.Descriptor.ContentHash,
		Payload:      json.RawMessage(f.Payload.Bytes()),
	}
}

// Value is the canonical: the live-only fold at a watermark (§7, §9), an immutable
// value with one deterministic serialisation. Superseded and tombstoned facets are
// excluded from it — they remain in the ledger, marked.
//
// The watermark itself is not part of the bytes: it is Anam-authored bookkeeping,
// and a rebuilt ledger reaches the same value at a different W. It travels beside
// the value (Result.W, the checkpoint row), never inside it.
type Value struct {
	entries []Entry
	bytes   []byte
	hash    string
}

// NewValue builds the value from its entries, ordering them by (event_time,
// facet_id) — timeline order, with identity as the total-order tiebreak — and
// pinning the serialisation. Both timestamps are fixed-width clock.Layout strings,
// so the lexical sort is the chronological one.
func NewValue(entries []Entry) (Value, error) {
	sorted := make([]Entry, len(entries))
	copy(sorted, entries)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].EventTime != sorted[j].EventTime {
			return sorted[i].EventTime < sorted[j].EventTime
		}
		return sorted[i].FacetID < sorted[j].FacetID
	})

	b, err := json.Marshal(sorted)
	if err != nil {
		return Value{}, fmt.Errorf("fold: serialise canonical value: %w", err)
	}
	sum := sha256.Sum256(b)
	return Value{entries: sorted, bytes: b, hash: "sha256:" + hex.EncodeToString(sum[:])}, nil
}

// Entries returns the value's entries in timeline order. The caller must not
// modify them; the value is immutable.
func (v Value) Entries() []Entry { return v.entries }

// Len reports how many live facets the value holds.
func (v Value) Len() int { return len(v.entries) }

// Bytes is the value's one serialisation — what a checkpoint file will hold (R8).
func (v Value) Bytes() []byte { return v.bytes }

// Hash is "sha256:<hex>" over Bytes: stable when nothing changed, moved by any
// ingest that changed the live set, and identical across a rebuild from source.
func (v Value) Hash() string { return v.hash }
