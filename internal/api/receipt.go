package api

import (
	"github.com/mattjoyce/anam/internal/fold"
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
// alone.
type ReceiptShown struct {
	Hash    string          `json:"hash"`
	Receipt receipt.Receipt `json:"receipt"`
	Entries []fold.Entry    `json:"entries"`
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
// W. The receipt's served hash is the hash of Entries, so the answer and its citation
// cannot drift apart.
type Published struct {
	Timeline
	ReceiptHash string          `json:"receipt_hash"`
	Receipt     receipt.Receipt `json:"receipt"`
}
