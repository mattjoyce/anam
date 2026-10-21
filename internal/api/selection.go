package api

// Saved selections (card 206): a selection program filed under a short code.

// SelectionRequest is POST /v1/selections's body: the program to save. A program
// within the selection page's chip grammar is saved in that grammar's one layout, so
// the same selection written two ways has one code; any other is saved as its text,
// trimmed.
type SelectionRequest struct {
	Program string `json:"program"`
}

// Selection is a saved selection: its code, the hash of the text it was saved as,
// the timeline's address of it, and the program.
type Selection struct {
	// Code is the first eight hex digits of Hash, or more when another saved
	// selection shares them. It is written sel:<code> where it could be mistaken.
	Code string `json:"code"`
	// Hash is the SHA-256 of Program, "sha256:<64 hex>": the artifact's address.
	Hash string `json:"hash"`
	// URL is the timeline drawing the selection, a path on the daemon's address
	// ("/?sel=<code>").
	URL string `json:"url"`
	// Program is the text saved, the canonical layout of a chip program.
	Program string `json:"program"`
}
