package receipt_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/receipt"
)

func digest(b []byte) string {
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func sound() receipt.Receipt {
	return receipt.Receipt{
		ServedHash:           "sha256:" + strings.Repeat("a", 64),
		FacetIDs:             []string{"f-1", "f-2"},
		Watermark:            893,
		Template:             "bp",
		TemplateVersion:      1,
		DefinitionSetVersion: "sha256:" + strings.Repeat("b", 64),
		ServedAt:             "2026-07-25T09:00:00.000000000Z",
		Span:                 "whole-person",
	}
}

// §9 names seven things a receipt must carry. A receipt missing one is not a partial
// record to be tolerated — it is a claim that cannot be checked.
func TestValidate_RequiresEveryField(t *testing.T) {
	t.Parallel()

	cases := map[string]func(*receipt.Receipt){
		"served_hash":            func(r *receipt.Receipt) { r.ServedHash = "" },
		"facet_ids":              func(r *receipt.Receipt) { r.FacetIDs = nil },
		"template":               func(r *receipt.Receipt) { r.Template = "" },
		"template_version":       func(r *receipt.Receipt) { r.TemplateVersion = 0 },
		"definition_set_version": func(r *receipt.Receipt) { r.DefinitionSetVersion = "" },
		"served_at":              func(r *receipt.Receipt) { r.ServedAt = "" },
		"span":                   func(r *receipt.Receipt) { r.Span = "" },
		"watermark":              func(r *receipt.Receipt) { r.Watermark = -1 },
	}
	for field, break_ := range cases {
		t.Run(field, func(t *testing.T) {
			t.Parallel()
			r := sound()
			break_(&r)
			if err := r.Validate(); !errors.Is(err, receipt.ErrMalformed) {
				t.Errorf("Validate without %s: err = %v, want ErrMalformed", field, err)
			}
		})
	}

	if err := sound().Validate(); err != nil {
		t.Errorf("a complete receipt did not validate: %v", err)
	}
}

// An empty serve is a real serve: it records an empty list, not a missing one.
func TestValidate_EmptyServeIsValid(t *testing.T) {
	t.Parallel()

	r := sound()
	r.FacetIDs = []string{}
	if err := r.Validate(); err != nil {
		t.Errorf("an empty serve did not validate: %v", err)
	}
}

func TestBytes_RoundTripsAndHoldsEveryField(t *testing.T) {
	t.Parallel()

	want := sound()
	b, err := want.Bytes()
	if err != nil {
		t.Fatalf("Bytes: %v", err)
	}

	// Every §9 field must appear on the wire, under its snake_case key.
	var onWire map[string]json.RawMessage
	if err := json.Unmarshal(b, &onWire); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, key := range []string{
		"served_hash", "facet_ids", "watermark", "template",
		"template_version", "definition_set_version", "served_at", "span",
	} {
		if _, ok := onWire[key]; !ok {
			t.Errorf("serialised receipt has no %q", key)
		}
	}
	if len(onWire) != 8 {
		t.Errorf("serialised receipt has %d keys, want 8", len(onWire))
	}

	got, err := receipt.Parse(b)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("round trip: got %+v, want %+v", got, want)
	}
}

// Bytes is what the content address is taken over, so it must be a function of the
// receipt alone.
func TestBytes_Deterministic(t *testing.T) {
	t.Parallel()

	r := sound()
	first, err := r.Bytes()
	if err != nil {
		t.Fatalf("Bytes: %v", err)
	}
	for range 10 {
		again, err := r.Bytes()
		if err != nil {
			t.Fatalf("Bytes: %v", err)
		}
		if string(again) != string(first) {
			t.Fatalf("Bytes is not deterministic")
		}
	}
}

// A receipt from a future Anam carrying a field this build cannot check is not
// something to verify partially and then report as sound.
func TestParse_RefusesUnknownFields(t *testing.T) {
	t.Parallel()

	b, err := sound().Bytes()
	if err != nil {
		t.Fatalf("Bytes: %v", err)
	}
	tampered := strings.Replace(string(b), `"span":`, `"seal_point": "2026-08-01",
  "span":`, 1)

	if _, err := receipt.Parse([]byte(tampered)); !errors.Is(err, receipt.ErrMalformed) {
		t.Errorf("Parse with an unknown field: err = %v, want ErrMalformed", err)
	}
}

// --- the two proofs -----------------------------------------------------------------

func TestCompare_BothProofsHold(t *testing.T) {
	t.Parallel()

	served := []byte(`[{"facet_id":"f-1"},{"facet_id":"f-2"}]`)
	r := sound()
	r.ServedHash = digest(served)

	got := receipt.Compare(r, r.ServedHash, r.FacetIDs, served, digest)
	if !got.OK() {
		t.Fatalf("a sound receipt did not verify: %+v", got)
	}
	if !got.Reproduced || !got.Intact {
		t.Errorf("Reproduced=%v Intact=%v, want both true", got.Reproduced, got.Intact)
	}
}

// The two proofs are independent: a tampered file under a sound ledger, or a moved
// ledger under an untouched file. Collapsing them would let each hide behind the other.
func TestCompare_ProofsFailIndependently(t *testing.T) {
	t.Parallel()

	served := []byte(`[{"facet_id":"f-1"},{"facet_id":"f-2"}]`)
	r := sound()
	r.ServedHash = digest(served)

	t.Run("tampered file, sound ledger", func(t *testing.T) {
		t.Parallel()
		altered := []byte(`[{"facet_id":"f-1"},{"facet_id":"f-3"}]`)
		got := receipt.Compare(r, r.ServedHash, r.FacetIDs, altered, digest)
		if !got.Reproduced {
			t.Error("the ledger re-derived the promised bytes; Reproduced should hold")
		}
		if got.Intact {
			t.Error("the file on disk was altered; Intact should not hold")
		}
		if got.OK() {
			t.Error("OK() held with a tampered artifact")
		}
	})

	t.Run("sound file, ledger no longer reproduces", func(t *testing.T) {
		t.Parallel()
		got := receipt.Compare(r, digest([]byte("something else")), []string{"f-1"}, served, digest)
		if got.Reproduced {
			t.Error("the re-derivation differed; Reproduced should not hold")
		}
		if !got.Intact {
			t.Error("the file on disk is untouched; Intact should hold")
		}
		if want := []string{"f-2"}; !reflect.DeepEqual(got.Missing, want) {
			t.Errorf("Missing = %v, want %v", got.Missing, want)
		}
	})

	t.Run("artifact absent is not artifact altered", func(t *testing.T) {
		t.Parallel()
		got := receipt.Compare(r, r.ServedHash, r.FacetIDs, nil, digest)
		if !got.Reproduced {
			t.Error("Reproduced should hold; the ledger was not consulted about the file")
		}
		if got.Intact || got.StoredHash != "" {
			t.Errorf("an absent artifact should leave Intact false and StoredHash empty, got %v/%q", got.Intact, got.StoredHash)
		}
		if !strings.Contains(got.Summary(), "not present") {
			t.Errorf("summary should distinguish absent from altered:\n%s", got.Summary())
		}
	})
}

// A mismatch should be readable, not merely noticed.
func TestCompare_NamesTheDifferences(t *testing.T) {
	t.Parallel()

	r := sound()
	r.FacetIDs = []string{"f-1", "f-2", "f-3"}

	got := receipt.Compare(r, "sha256:different", []string{"f-2", "f-9"}, nil, digest)
	if want := []string{"f-1", "f-3"}; !reflect.DeepEqual(got.Missing, want) {
		t.Errorf("Missing = %v, want %v", got.Missing, want)
	}
	if want := []string{"f-9"}; !reflect.DeepEqual(got.Unexpected, want) {
		t.Errorf("Unexpected = %v, want %v", got.Unexpected, want)
	}

	summary := got.Summary()
	for _, want := range []string{"FAIL", "reproduced", "missing", "unexpected", "f-1", "f-9"} {
		if !strings.Contains(summary, want) {
			t.Errorf("summary omits %q:\n%s", want, summary)
		}
	}
}

// The same facet set in a different order is a different served value: order is part
// of the bytes, and the receipt's list is in served order.
func TestCompare_HashDisagreementOutweighsSetEquality(t *testing.T) {
	t.Parallel()

	r := sound()
	got := receipt.Compare(r, "sha256:different", r.FacetIDs, nil, digest)
	if got.Reproduced {
		t.Error("Reproduced held with matching ids but a different hash")
	}
	if len(got.Missing) != 0 || len(got.Unexpected) != 0 {
		t.Errorf("the id sets match; Missing=%v Unexpected=%v", got.Missing, got.Unexpected)
	}
}

// The type narrowing is facet.Matches: one exact type, or a prefix at a segment
// boundary, so the timeline and a receipt's replay agree on a prefix (card 193).
func TestQueryAdmits_TypePattern(t *testing.T) {
	t.Parallel()

	const at = "2026-02-10T00:00:00.000000000Z"
	tests := []struct {
		name    string
		pattern string
		typ     string
		want    bool
	}{
		{"no type narrows nothing", "", "health.sleep", true},
		{"exact", "health.sleep", "health.sleep", true},
		{"exact other", "health.sleep", "health.weight", false},
		{"prefix leaf", "health.heart-rate.", "health.heart-rate.day", true},
		{"prefix hyphenated sibling", "health.heart-rate.", "health.heart-rate-variability", false},
		{"prefix is not its own concept", "health.heart-rate.", "health.heart-rate", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			q := &receipt.Query{Type: tc.pattern}
			if got := q.Admits(at, tc.typ); got != tc.want {
				t.Errorf("Admits(%q) under type %q = %v, want %v", tc.typ, tc.pattern, got, tc.want)
			}
		})
	}
}
