package fold_test

import (
	"encoding/json"
	"testing"

	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/fold"
)

func entry(id, factTime, hash string) fold.Entry {
	return fold.Entry{
		FacetID: id, Type: "health.blood-pressure",
		FactTime: factTime,
		Source:   "bp-csv", SourceRef: "bp-csv:" + id,
		Status: facet.StatusAdmitted, ContentHash: hash,
		Payload: json.RawMessage(`{"systolic":118}`),
	}
}

func TestValue_OneSerialisation(t *testing.T) {
	t.Parallel()

	a := entry("a", "2026-01-24T06:30:00.000000000Z", "sha256:aa")
	b := entry("b", "2026-01-20T06:30:00.000000000Z", "sha256:bb")

	// Entry order must not matter: the value sorts itself, so the same live set is
	// the same bytes and the same hash however the fetch happened to return it.
	v1, err := fold.NewValue([]fold.Entry{a, b})
	if err != nil {
		t.Fatalf("NewValue: %v", err)
	}
	v2, err := fold.NewValue([]fold.Entry{b, a})
	if err != nil {
		t.Fatalf("NewValue: %v", err)
	}
	if string(v1.Bytes()) != string(v2.Bytes()) || v1.Hash() != v2.Hash() {
		t.Errorf("same entries, different serialisation:\n%s\n%s", v1.Bytes(), v2.Bytes())
	}

	if v1.Entries()[0].FacetID != "b" {
		t.Errorf("entries not in fact-time order: %s first", v1.Entries()[0].FacetID)
	}

	// A changed live set is a different value.
	v3, err := fold.NewValue([]fold.Entry{a})
	if err != nil {
		t.Fatalf("NewValue: %v", err)
	}
	if v3.Hash() == v1.Hash() {
		t.Error("a smaller live set hashed identically")
	}

	// The bytes are valid JSON holding exactly the entries.
	var decoded []fold.Entry
	if err := json.Unmarshal(v1.Bytes(), &decoded); err != nil {
		t.Fatalf("value bytes are not JSON: %v", err)
	}
	if len(decoded) != 2 {
		t.Errorf("decoded %d entries, want 2", len(decoded))
	}
}

func TestValue_TiebreakOnFacetID(t *testing.T) {
	t.Parallel()

	when := "2026-01-24T06:30:00.000000000Z"
	v, err := fold.NewValue([]fold.Entry{
		entry("zz", when, "sha256:zz"),
		entry("aa", when, "sha256:aa"),
	})
	if err != nil {
		t.Fatalf("NewValue: %v", err)
	}
	if v.Entries()[0].FacetID != "aa" {
		t.Error("equal fact-times must fall back to facet-id order, or the value is not a value")
	}
}

func TestValue_Empty(t *testing.T) {
	t.Parallel()

	v, err := fold.NewValue(nil)
	if err != nil {
		t.Fatalf("NewValue: %v", err)
	}
	if v.Len() != 0 || string(v.Bytes()) != "[]" {
		t.Errorf("empty value serialises as %q, want []", v.Bytes())
	}
	if v.Hash() == "" {
		t.Error("even the empty value has an identity")
	}
}
