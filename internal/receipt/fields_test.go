package receipt_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/receipt"
)

// A query's fields are part of what the receipt pins (card 138): they make it
// non-zero, they must be JSON pointers, and a receipt without them keeps the bytes it
// had before the field existed.
func TestQuery_Fields(t *testing.T) {
	t.Parallel()
	if (receipt.Query{Fields: []string{"/x"}}).IsZero() {
		t.Error("a query with fields narrows something")
	}
	if !(receipt.Query{}).IsZero() {
		t.Error("an empty query narrows nothing")
	}

	r := receipt.Receipt{
		ServedHash: "sha256:x", FacetIDs: []string{}, Watermark: 1, Template: "bp", TemplateVersion: 1,
		DefinitionSetVersion: "d", ServedAt: "2026-01-01T00:00:00.000000000Z", Span: "s",
		Query: &receipt.Query{Type: "health.blood-pressure", Fields: []string{"x"}},
	}
	if err := r.Validate(); !errors.Is(err, receipt.ErrMalformed) || !strings.Contains(err.Error(), `"x" is not a JSON pointer`) {
		t.Errorf("a field without a slash: %v", err)
	}

	r.Query.Fields = []string{"/systolic", "/pulse"}
	b, err := r.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"fields": [`) {
		t.Errorf("the receipt does not carry its fields:\n%s", b)
	}
	again, err := receipt.Parse(b)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if strings.Join(again.Query.Fields, " ") != "/systolic /pulse" {
		t.Errorf("fields after a round trip = %v", again.Query.Fields)
	}

	r.Query.Fields = nil
	if b, _ := r.Bytes(); strings.Contains(string(b), "fields") {
		t.Errorf("a receipt without fields names them:\n%s", b)
	}
}
