package read

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/mattjoyce/anam/internal/api"
	"github.com/mattjoyce/anam/internal/serve"
)

// ErrCursor marks a page asked for with a cursor that does not name a page of the
// receipt it was presented to: not a cursor at all, another format's, another
// receipt's, or an offset that is out of range or not where the cursor says (card 92).
// Asking again with the same cursor will fail again.
var ErrCursor = errors.New("cursor invalid")

// cursorError is a refused cursor. It reads as the error it wraps, and is ErrCursor.
type cursorError struct{ err error }

func (e cursorError) Error() string        { return "cursor invalid: " + e.err.Error() }
func (e cursorError) Unwrap() error        { return e.err }
func (e cursorError) Is(target error) bool { return target == ErrCursor }

func badCursor(format string, a ...any) error { return cursorError{fmt.Errorf(format, a...)} }

// cursorVersion is the cursor's format. It is an invariant, not a setting: a daemon
// reads only the cursors it can check, and a format change must refuse the old ones
// by name rather than misread them.
const cursorVersion = 1

// cursor is what a next_cursor says, opaque on the wire: its format, the receipt, the
// offset of the page's first facet, and that facet's id. It is checked, never
// trusted. The receipt is immutable and content-addressed, so its own facet_ids are
// the check: an offset that passes is a true portion of the receipted value, and no
// key is needed to sign it. The facet id binds the order: a receipt pages in its own
// facet_ids order, the canonical's (fact-time, id), and the cursor's facet must stand
// at its offset in it.
type cursor struct {
	V       int    `json:"v"`
	Receipt string `json:"receipt"`
	Offset  int    `json:"offset"`
	FacetID string `json:"facet_id"`
}

// encodeCursor is the cursor for the page of receipt hash that starts at offset.
func encodeCursor(hash string, ids []string, offset int) string {
	b, _ := json.Marshal(cursor{V: cursorVersion, Receipt: hash, Offset: offset, FacetID: ids[offset]}) // strings and ints always marshal
	return base64.RawURLEncoding.EncodeToString(b)
}

// decodeCursor reads s as a cursor for receipt hash, whose served facets are ids,
// and returns its offset, or refuses it with ErrCursor.
func decodeCursor(s, hash string, ids []string) (int, error) {
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return 0, badCursor("not a cursor this daemon issued")
	}
	var c cursor
	if err := json.Unmarshal(b, &c); err != nil {
		return 0, badCursor("not a cursor this daemon issued")
	}
	switch {
	case c.V != cursorVersion:
		return 0, badCursor("format %d; this daemon reads format %d", c.V, cursorVersion)
	case c.Receipt != hash:
		return 0, badCursor("it pages receipt %s, not %s; a cursor only pages the receipt that issued it", c.Receipt, hash)
	case c.Offset <= 0 || c.Offset >= len(ids):
		return 0, badCursor("offset %d is outside the receipt's %d facets", c.Offset, len(ids))
	case ids[c.Offset] != c.FacetID:
		return 0, badCursor("offset %d is facet %s in this receipt, not %s", c.Offset, ids[c.Offset], c.FacetID)
	}
	return c.Offset, nil
}

// pageOf is the page's measure: where it starts and ends among the receipt's ids,
// and the cursor to the next page when there is one.
func pageOf(hash string, ids []string, offset, end int) api.Page {
	p := api.Page{Offset: offset, ReturnedCount: end - offset, TotalCount: len(ids)}
	if end < len(ids) {
		p.NextCursor = encodeCursor(hash, ids, end)
	}
	return p
}

// Receipt reads one receipt and the whole served value it names, from the stored rows
// alone: the one page a local read answers with.
func Receipt(ctx context.Context, d serve.Deps, hash string) (api.ReceiptShown, error) {
	return ReceiptPage(ctx, d, hash, "", 0)
}

// ReceiptPage reads one page of the value receipt hash served, from the stored rows
// alone (card 92): the page cursor names, or the first page when cursor is empty, cut
// to whole facets that fit limit bytes as Fit measures them. Zero is no bound, one
// page. Every page is cut from the same served value the receipt hashes, so the
// pages' union is exactly the receipt's facet_ids, in order, whatever the ledger has
// learned since. A facet over limit alone is refused with a *TooLarge naming it.
//
// A query receipt's answer (card 173) is one relation, not facets, and is answered
// whole within limit or refused; a cursor on one is refused.
func ReceiptPage(ctx context.Context, d serve.Deps, hash, cursor string, limit int) (api.ReceiptShown, error) {
	r, shown, err := serve.Show(ctx, d, hash)
	if err != nil {
		return api.ReceiptShown{}, err
	}
	out := api.ReceiptShown{Hash: hash}
	if cursor == "" {
		out.Receipt = &r
	}
	if shown.Answer != nil {
		if cursor != "" {
			return api.ReceiptShown{}, badCursor("receipt %s answers a query, one relation, which is not paged", hash)
		}
		if err := Fit(shown.Answer.Tuples, limit,
			"show it locally with anam receipt show; a query's answer is not paged, so ask a narrower query"); err != nil {
			return api.ReceiptShown{}, err
		}
		out.Answer = shown.Answer
		return out, nil
	}

	ids := r.FacetIDs
	n := len(shown.Entries)
	if shown.Frame != nil {
		n = shown.Frame.Len()
	}
	if n != len(ids) {
		return api.ReceiptShown{}, fmt.Errorf("read: receipt %s names %d facets and its served value holds %d", hash, len(ids), n)
	}
	offset := 0
	if cursor != "" {
		if offset, err = decodeCursor(cursor, hash, ids); err != nil {
			return api.ReceiptShown{}, err
		}
	}

	if shown.Frame != nil {
		end, err := pageEnd(shown.Frame.Rows, ids, offset, limit,
			"show the receipt locally with anam receipt show; a page never cuts a facet")
		if err != nil {
			return api.ReceiptShown{}, past(err, hash, ids, offset)
		}
		fr, err := shown.Frame.Page(offset, end)
		if err != nil {
			return api.ReceiptShown{}, err
		}
		out.Frame = &fr
		out.Page = ptr(pageOf(hash, ids, offset, end))
		return out, nil
	}
	end, err := pageEnd(shown.Entries, ids, offset, limit,
		"open it alone at GET /v1/receipts/"+hash+"/facets/{facet_id}, or show the receipt locally with anam receipt show")
	if err != nil {
		return api.ReceiptShown{}, past(err, hash, ids, offset)
	}
	out.Entries = shown.Entries[offset:end]
	out.Page = ptr(pageOf(hash, ids, offset, end))
	return out, nil
}

// past gives a page refused for its first facet the cursor to the page after that
// facet, so a reader is not stuck at it: the facet opens alone by its own route.
func past(err error, hash string, ids []string, offset int) error {
	var tl *TooLarge
	if errors.As(err, &tl) && offset+1 < len(ids) {
		tl.NextCursor = encodeCursor(hash, ids, offset+1)
	}
	return err
}

func ptr[T any](v T) *T { return &v }
