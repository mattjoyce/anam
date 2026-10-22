package receipt_test

import (
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/receipt"
)

// oldReceipt is the exact bytes of a receipt cut before it named its reader, and the
// content address those bytes had. A build that changes either has re-addressed every
// receipt already filed, and none of them would be found again.
const oldReceipt = `{
  "served_hash": "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
  "facet_ids": [
    "f-1",
    "f-2"
  ],
  "watermark": 893,
  "template": "bp",
  "template_version": 1,
  "definition_set_version": "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
  "served_at": "2026-07-25T09:00:00.000000000Z",
  "span": "whole-person"
}
`

const oldReceiptHash = "sha256:7ed6047357d020994573f2cb3ccc3bf80f55e0584a9510ade667ed0c73b69d01"

// A receipt cut before the principal field parses, serialises to the same bytes, and
// so keeps its hash.
func TestPrincipal_OldReceiptKeepsItsBytesAndHash(t *testing.T) {
	t.Parallel()
	if got := digest([]byte(oldReceipt)); got != oldReceiptHash {
		t.Fatalf("the fixture hashes to %s, want %s", got, oldReceiptHash)
	}
	r, err := receipt.Parse([]byte(oldReceipt))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if r.Principal != "" {
		t.Errorf("Principal = %q, want none", r.Principal)
	}
	b, err := r.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != oldReceipt {
		t.Errorf("an old receipt serialises to other bytes:\n%s", b)
	}
	if got := digest(b); got != oldReceiptHash {
		t.Errorf("an old receipt now hashes to %s, was %s", got, oldReceiptHash)
	}
}

// A receipt that names a reader carries the name under its key, round-trips it, and
// is a different receipt from the same serve with no reader.
func TestPrincipal_NewReceiptNamesItsReader(t *testing.T) {
	t.Parallel()
	anon := sound()
	named := sound()
	named.Principal = "lifeos"

	b, err := named.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"principal": "lifeos"`) {
		t.Errorf("the bytes do not carry the reader:\n%s", b)
	}
	back, err := receipt.Parse(b)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if back.Principal != "lifeos" {
		t.Errorf("Principal = %q after a round trip", back.Principal)
	}
	ab, err := anon.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(ab), "principal") {
		t.Errorf("a receipt with no reader carries the key:\n%s", ab)
	}
	if digest(ab) == digest(b) {
		t.Errorf("two readers of one serve share a receipt")
	}
}
