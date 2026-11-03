package daemon

import (
	"errors"
	"testing"

	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/gate"
)

// checkReferences refuses a record.document whose sha256 it cannot read, rather than
// leaving it to the gate: a Definition that types sha256 as something other than a
// string would otherwise admit a reference to no bytes (Sol's review of #37). Here the
// library is empty, so a readable sha256 is refused as missing; an unreadable one is
// refused for that reason, with the same sentinel.
func TestCheckReferences_RefusesAnUnreadableSHA256(t *testing.T) {
	t.Parallel()
	lib := t.TempDir()
	for _, tt := range []struct {
		name, payload string
	}{
		{"a number", `{"sha256": 123, "path": "a.pdf"}`},
		{"null", `{"sha256": null, "path": "a.pdf"}`},
		{"an object", `{"sha256": {"x": 1}, "path": "a.pdf"}`},
		{"not json", `{"sha256": `},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			env := facet.Envelope{Type: docType, Payload: []byte(tt.payload)}
			err := checkReferences(lib, []facet.Envelope{env})
			var refused *gate.Refused
			if !errors.As(err, &refused) || len(refused.Envelopes) != 1 {
				t.Fatalf("checkReferences = %v, want one refused envelope", err)
			}
			if !errors.Is(refused.Envelopes[0].Err, ErrDocumentMissing) {
				t.Errorf("refusal %v does not wrap ErrDocumentMissing", refused.Envelopes[0].Err)
			}
		})
	}
}
