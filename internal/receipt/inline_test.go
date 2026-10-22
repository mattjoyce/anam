package receipt_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/receipt"
)

// queryReceipt is a query receipt as one was cut before inline projections (card 211).
func queryReceipt() receipt.Receipt {
	r := sound()
	r.Program = &receipt.Program{
		RuleSet: "anam", RuleSetVersion: 1, RuleSetHash: "sha256:cc",
		Text: "x(E) :- entry(E, _, _, _, _, _, _).", Relation: "x",
	}
	r.TemplateHash = "sha256:dd"
	r.Principal = "assistant"
	return r
}

// Card 211: a receipt cut before inline projections keeps its bytes, so its address and
// its hash are what they were. The digests were taken from the build before the field
// existed. A receipt that is not inline never writes the marker.
func TestInline_EarlierReceiptsKeepTheirBytes(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		r    receipt.Receipt
		want string
	}{
		{"timeline receipt", sound(), "sha256:7ed6047357d020994573f2cb3ccc3bf80f55e0584a9510ade667ed0c73b69d01"},
		{"query receipt", queryReceipt(), "sha256:e3afe4a68f4e3dd8020562fc1fac9c36c66e9b1829ff7c7a432c39fcd43cc7e7"},
	} {
		b, err := tc.r.Bytes()
		if err != nil {
			t.Fatal(err)
		}
		if got := digest(b); got != tc.want {
			t.Errorf("%s: hash is %s, was %s before the inline marker", tc.name, got, tc.want)
		}
		if strings.Contains(string(b), "template_inline") {
			t.Errorf("%s: names the inline marker:\n%s", tc.name, b)
		}
		if _, err := receipt.Parse(b); err != nil {
			t.Errorf("%s: no longer parses: %v", tc.name, err)
		}
	}
}

// An inline receipt names its template by hash and says it is inline; it round-trips,
// and a receipt that claims it without a hash to find the body by, or on a read that
// is not a query, could not be replayed and is refused.
func TestInline_ReceiptRoundTripsAndIsHeldToWhatReplayNeeds(t *testing.T) {
	t.Parallel()
	r := queryReceipt()
	r.Template, r.TemplateVersion, r.TemplateInline = "inline", 1, true
	b, err := r.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"template_inline": true`) {
		t.Errorf("an inline receipt does not say so:\n%s", b)
	}
	back, err := receipt.Parse(b)
	if err != nil || !back.TemplateInline || back.TemplateHash != r.TemplateHash {
		t.Fatalf("Parse = %+v, %v", back, err)
	}

	noHash := r
	noHash.TemplateHash = ""
	notQuery := r
	notQuery.Program = nil
	for name, bad := range map[string]receipt.Receipt{"no hash": noHash, "not a query": notQuery} {
		if err := bad.Validate(); !errors.Is(err, receipt.ErrMalformed) {
			t.Errorf("%s: Validate = %v, want ErrMalformed", name, err)
		}
	}
}
