package facet_test

import (
	"errors"
	"testing"

	"github.com/mattjoyce/anam/internal/facet"
)

func TestCanonicalize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{"sorts object keys", `{"b":1,"a":2}`, `{"a":2,"b":1}`},
		{"strips whitespace", "{\n  \"a\" : 1\n}", `{"a":1}`},
		{"sorts nested keys", `{"m":{"z":1,"a":2}}`, `{"m":{"a":2,"z":1}}`},
		{"preserves array order", `[3,1,2]`, `[3,1,2]`},
		{"sorts keys inside arrays", `[{"b":1,"a":2}]`, `[{"a":2,"b":1}]`},
		{"empty object", `{}`, `{}`},
		{"empty array", `[]`, `[]`},
		{"null", `null`, `null`},
		{"booleans", `{"t":true,"f":false}`, `{"f":false,"t":true}`},
		{"bare string", `"hi"`, `"hi"`},
		{"bare number", `42`, `42`},
		{"does not HTML-escape", `{"note":"a<b & c>d"}`, `{"note":"a<b & c>d"}`},
		{"escapes control characters", "{\"note\":\"a\\nb\"}", `{"note":"a\nb"}`},
		{"unicode survives", `{"note":"café"}`, `{"note":"café"}`},
		{"key byte order, not case-insensitive", `{"b":1,"A":2}`, `{"A":2,"b":1}`},
		{
			"realistic health payload",
			`{"metrics":{"sleep_total":{"min":null,"avg":7.5},"sleep_rem":{"avg":1.2}},"period_start":"2026-06-06T00:00:00"}`,
			`{"metrics":{"sleep_rem":{"avg":1.2},"sleep_total":{"avg":7.5,"min":null}},"period_start":"2026-06-06T00:00:00"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := facet.Canonicalize([]byte(tt.in))
			if err != nil {
				t.Fatalf("Canonicalize() error = %v", err)
			}
			if string(got.Bytes()) != tt.want {
				t.Errorf("Canonicalize() = %s, want %s", got.Bytes(), tt.want)
			}
		})
	}
}

// TestCanonicalizePreservesNumbersVerbatim guards the verbatim rule: a payload's values
// must reach the ledger as the source wrote them. Routing numbers through float64 would
// rewrite "1.50" as "1.5" and round a large integer.
func TestCanonicalizePreservesNumbersVerbatim(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{"trailing zero kept", `{"weight":"1.50"}`, `{"weight":"1.50"}`},
		{"numeric trailing zero kept", `{"weight":1.50}`, `{"weight":1.50}`},
		{"large integer not rounded", `{"n":9007199254740993}`, `{"n":9007199254740993}`},
		{"high precision kept", `{"n":0.1234567890123456789}`, `{"n":0.1234567890123456789}`},
		{"exponent form kept", `{"n":1e3}`, `{"n":1e3}`},
		{"negative zero kept", `{"n":-0.0}`, `{"n":-0.0}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := facet.Canonicalize([]byte(tt.in))
			if err != nil {
				t.Fatalf("Canonicalize() error = %v", err)
			}
			if string(got.Bytes()) != tt.want {
				t.Errorf("Canonicalize() = %s, want %s", got.Bytes(), tt.want)
			}
		})
	}
}

// TestCanonicalizeIsStableAcrossKeyOrder is the property idempotent commit rests on:
// the same source row, serialised in any key order, must produce identical bytes and
// therefore an identical content hash.
func TestCanonicalizeIsStableAcrossKeyOrder(t *testing.T) {
	t.Parallel()

	orderings := []string{
		`{"a":1,"b":{"x":1,"y":2},"c":[1,2]}`,
		`{"c":[1,2],"a":1,"b":{"y":2,"x":1}}`,
		`{"b":{"x":1,"y":2},"c":[1,2],"a":1}`,
		"{\n\t\"b\" : { \"y\":2, \"x\":1 },\n\t\"c\":[1,2],\n\t\"a\":1\n}",
	}

	var wantBytes, wantHash string
	for i, in := range orderings {
		got, err := facet.Canonicalize([]byte(in))
		if err != nil {
			t.Fatalf("Canonicalize(%s) error = %v", in, err)
		}
		hash := facet.ContentHash(got)

		if i == 0 {
			wantBytes, wantHash = string(got.Bytes()), hash
			continue
		}
		if string(got.Bytes()) != wantBytes {
			t.Errorf("Canonicalize(%s) = %s, want %s", in, got.Bytes(), wantBytes)
		}
		if hash != wantHash {
			t.Errorf("ContentHash for %s = %s, want %s", in, hash, wantHash)
		}
	}
}

func TestCanonicalizeIsIdempotent(t *testing.T) {
	t.Parallel()

	once, err := facet.Canonicalize([]byte(`{"b":1,"a":{"d":4,"c":"<x>"}}`))
	if err != nil {
		t.Fatalf("Canonicalize() error = %v", err)
	}
	twice, err := facet.Canonicalize(once.Bytes())
	if err != nil {
		t.Fatalf("Canonicalize() on canonical bytes error = %v", err)
	}
	if string(once.Bytes()) != string(twice.Bytes()) {
		t.Errorf("Canonicalize is not idempotent: %s then %s", once.Bytes(), twice.Bytes())
	}
}

// TestCanonicalFromStoredCopies guards the payload against a caller who keeps, and
// then mutates, the buffer an admitted facet was built from. The payload is an
// immutable value (§3); Go will not enforce that on a slice, so the boundary copies.
func TestCanonicalFromStoredCopies(t *testing.T) {
	t.Parallel()

	stored := []byte(`{"a":1}`)
	c := facet.CanonicalFromStored(stored)
	stored[2] = 'X'

	if got := string(c.Bytes()); got != `{"a":1}` {
		t.Errorf("payload = %s after the caller mutated its buffer, want {\"a\":1}", got)
	}
}

func TestCanonicalZeroValue(t *testing.T) {
	t.Parallel()

	var c facet.Canonical
	if c.Len() != 0 {
		t.Errorf("zero Canonical Len() = %d, want 0", c.Len())
	}
	if c.Bytes() != nil {
		t.Errorf("zero Canonical Bytes() = %v, want nil", c.Bytes())
	}
}

func TestCanonicalizeRejectsMalformed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
	}{
		{"empty", ``},
		{"not json", `this is not json`},
		{"unterminated object", `{"a":1`},
		{"trailing data", `{"a":1} {"b":2}`},
		{"trailing garbage", `{"a":1} oops`},
		{"two values", `1 2`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := facet.Canonicalize([]byte(tt.in))
			if err == nil {
				t.Fatalf("Canonicalize(%q) = nil error, want error", tt.in)
			}
			if !errors.Is(err, facet.ErrMalformedPayload) {
				t.Errorf("Canonicalize(%q) error = %v, want it to wrap ErrMalformedPayload", tt.in, err)
			}
		})
	}
}

func TestContentHash(t *testing.T) {
	t.Parallel()

	canonical, err := facet.Canonicalize([]byte(`{"a":1}`))
	if err != nil {
		t.Fatalf("Canonicalize() error = %v", err)
	}

	// The digest of the canonical bytes {"a":1}, pinned so a change to the hashing or
	// the canonical form cannot pass silently: it would orphan every stored facet's
	// idempotency marker.
	const want = "sha256:015abd7f5cc57a2dd94b7590f04ad8084273905ee33ec5cebeae62276a97f862"

	got := facet.ContentHash(canonical)
	if got != want {
		t.Errorf("ContentHash() = %q, want %q", got, want)
	}

	other, err := facet.Canonicalize([]byte(`{"a":2}`))
	if err != nil {
		t.Fatalf("Canonicalize() error = %v", err)
	}
	if facet.ContentHash(other) == got {
		t.Error("ContentHash() collided for different payloads; a changed source row must hash differently")
	}
}

// TestContentHashDetectsChangedSourceRow is the supersession trigger from §6a: the same
// source_ref with a different value must produce a different hash.
func TestContentHashDetectsChangedSourceRow(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		original string
		changed  string
	}{
		{"value changed", `{"systolic":"118"}`, `{"systolic":"121"}`},
		{"field added", `{"systolic":"118"}`, `{"systolic":"118","pulse":"61"}`},
		{"field removed", `{"systolic":"118","pulse":"61"}`, `{"systolic":"118"}`},
		{"type changed", `{"systolic":"118"}`, `{"systolic":118}`},
		{"null vs absent", `{"a":null}`, `{}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			a, err := facet.Canonicalize([]byte(tt.original))
			if err != nil {
				t.Fatalf("Canonicalize() error = %v", err)
			}
			b, err := facet.Canonicalize([]byte(tt.changed))
			if err != nil {
				t.Fatalf("Canonicalize() error = %v", err)
			}
			if facet.ContentHash(a) == facet.ContentHash(b) {
				t.Errorf("hash of %s equals hash of %s; supersession would not trigger", tt.original, tt.changed)
			}
		})
	}
}
