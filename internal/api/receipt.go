package api

import (
	"github.com/mattjoyce/anam/internal/fold"
	"github.com/mattjoyce/anam/internal/frame"
	"github.com/mattjoyce/anam/internal/query"
	"github.com/mattjoyce/anam/internal/receipt"
)

// Receipts lists the receipts of past serves, newest first.
type Receipts struct {
	// Dir is the artifact directory the receipts were read from, on the machine
	// that read them.
	Dir      string           `json:"dir"`
	Receipts []ReceiptListing `json:"receipts"`
}

// ReceiptListing is one receipt and the hash it is filed under.
type ReceiptListing struct {
	Hash    string          `json:"hash"`
	Receipt receipt.Receipt `json:"receipt"`
}

// ReceiptShown is a receipt and the served value it names, read from the artifacts
// alone: the whole value, or over the daemon one page of it (card 92).
//
// Receipt rides the first page only. A page asked for by cursor carries Hash and
// Page with its entries: the cursor is bound to that hash, so the reader already
// holds the receipt, and its facet_ids, one id per facet of the whole value, are
// not sent again with every page.
type ReceiptShown struct {
	Hash    string           `json:"hash"`
	Receipt *receipt.Receipt `json:"receipt,omitempty"`
	Entries []fold.Entry     `json:"entries"`
	// Frame is the served value when the receipt's query named fields; Entries is
	// then empty.
	Frame *frame.Frame `json:"frame,omitempty"`
	// Answer is the served value of a query receipt (card 173); Entries is then
	// empty. An answer is one relation and is not paged, so Page is then nil.
	Answer *query.Answer `json:"answer,omitempty"`
	// Page says which of the receipt's facets Entries, or the frame's rows, are.
	Page *Page `json:"page,omitempty"`
}

// Page is which portion of a receipted value an answer carries (card 92): the
// receipt's facets from Offset, ReturnedCount of them, of TotalCount in all. The
// receipt names the whole value and every page is cut from it, never a new read. A
// frame's page carries every column and this page's rows.
// NextCursor, when set, asks for the page after this one, from the same receipt
// (GET /v1/receipts/{hash}?cursor=); empty on the last page.
type Page struct {
	Offset        int    `json:"offset"`
	ReturnedCount int    `json:"returned_count"`
	TotalCount    int    `json:"total_count"`
	NextCursor    string `json:"next_cursor,omitempty"`
}

// ReceiptVerified is a receipt and the two proofs of it: re-derived from the ledger,
// and re-hashed on disk.
type ReceiptVerified struct {
	Hash    string          `json:"hash"`
	Receipt receipt.Receipt `json:"receipt"`
	Result  receipt.Result  `json:"result"`
}

// Published is a read filed with its receipt (POST /v1/receipts): the timeline the
// reader sees, narrowed as asked, and the receipt that names exactly those entries at
// W. The receipt's served hash is the hash of the whole value, so the answer and its
// citation cannot drift apart.
//
// A read too large for one answer is still published whole, and answered a page at
// a time (card 92): Entries, Versions or the frame's rows are the first page, Shown
// stays the count the narrowing kept, and Page says where the rest is.
type Published struct {
	Timeline
	ReceiptHash string          `json:"receipt_hash"`
	Receipt     receipt.Receipt `json:"receipt"`
	Page        Page            `json:"page"`
}
