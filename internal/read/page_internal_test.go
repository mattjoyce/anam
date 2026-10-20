package read

import (
	"errors"
	"strings"
	"testing"
)

// A page is the most whole entries from its offset that fit the bound, never fewer
// than one, measured as Fit measures; one entry over the bound alone is refused by
// name, with the cursor past it.
func TestPageEnd(t *testing.T) {
	t.Parallel()
	// Each entry encodes to 5 bytes ("aaa" with its quotes); an array of k of them
	// is 2 + 5k + (k-1).
	entries := []string{"aaa", "bbb", "ccc", "ddd", "eee"}
	ids := []string{"f0", "f1", "f2", "f3", "f4"}
	for _, tt := range []struct {
		name          string
		offset, limit int
		want          int
	}{
		{"no bound is one page", 0, 0, 5},
		{"two fit exactly", 0, 13, 2},
		{"one byte short of two", 0, 12, 1},
		{"from an offset", 3, 13, 5},
		{"the rest fits", 1, 100, 5},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := pageEnd(entries, ids, tt.offset, tt.limit, "")
			if err != nil || got != tt.want {
				t.Errorf("pageEnd(offset %d, limit %d) = %d, %v; want %d", tt.offset, tt.limit, got, err, tt.want)
			}
			if Fit(entries[tt.offset:got], tt.limit, "") != nil {
				t.Errorf("the page [%d, %d) does not Fit the bound %d", tt.offset, got, tt.limit)
			}
		})
	}

	big := []string{"aaa", strings.Repeat("x", 50), "ccc"}
	_, err := pageEnd(big, ids, 1, 13, "open it alone")
	var tl *TooLarge
	if !errors.As(err, &tl) || tl.FacetID != "f1" || tl.Matched != 1 || !strings.Contains(err.Error(), "facet f1 alone") {
		t.Fatalf("an entry over the bound alone: %v", err)
	}
	_ = past(err, "sha256:r", ids[:3], 1) // it sets the refusal's cursor in place
	if off, cerr := decodeCursor(tl.NextCursor, "sha256:r", ids[:3]); cerr != nil || off != 2 {
		t.Errorf("the refusal's cursor = %d, %v; want the page after the facet, offset 2", off, cerr)
	}
	if fitEach(big, ids, 13, "") == nil || fitEach(entries, ids, 13, "") != nil {
		t.Error("fitEach should refuse only the entry over the bound alone")
	}
}

// A cursor is checked against the receipt it is presented to, never trusted.
func TestDecodeCursor(t *testing.T) {
	t.Parallel()
	ids := []string{"f0", "f1", "f2"}
	good := encodeCursor("sha256:r", ids, 2)
	if off, err := decodeCursor(good, "sha256:r", ids); err != nil || off != 2 {
		t.Fatalf("its own cursor: %d, %v", off, err)
	}
	for _, tt := range []struct {
		name, cursor, hash string
		ids                []string
	}{
		{"another receipt", good, "sha256:other", ids},
		{"another order of the same ids", good, "sha256:r", []string{"f0", "f2", "f1"}},
		{"past the end", good, "sha256:r", ids[:2]},
		{"not base64", "%%%", "sha256:r", ids},
		{"not a cursor", "bm90LWpzb24", "sha256:r", ids},
		{"the first page's offset", encodeCursor("sha256:r", ids, 0), "sha256:r", ids},
	} {
		if _, err := decodeCursor(tt.cursor, tt.hash, tt.ids); !errors.Is(err, ErrCursor) {
			t.Errorf("%s: %v, want ErrCursor", tt.name, err)
		}
	}
}
