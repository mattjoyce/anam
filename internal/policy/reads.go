package policy

import (
	"fmt"

	"github.com/mattjoyce/anam/internal/config"
)

// Records reports whether every read p makes is filed as a receipt naming it. A dry-run
// query files nothing, for a recording token too: the receipt log covers real reads,
// and dry_run stays open to every token (Matt, 2026-10-22). There is no rule for it.
func Records(p config.Principal) bool {
	return p.Records()
}

// ReaderOf is the name a receipt filed for p carries: p's when its token records every
// read, else none, as every receipt carried before card 216.
func ReaderOf(p config.Principal) string {
	if Records(p) {
		return p.Name
	}
	return ""
}

// TimelineRead decides how p's timeline read is answered. A recording token's read is
// filed as a publish (publish true), and so must name a projection and leave out
// explain, since a receipt is what a projection served; anything else is refused, with
// the reason. Every other token reads plainly.
func TimelineRead(p config.Principal, projection string, explain bool) (publish bool, err error) {
	if !Records(p) {
		return false, nil
	}
	if projection == "" || explain {
		return true, fmt.Errorf("%s files a receipt for every read, and a receipt is what a projection served: "+
			"name a projection, and leave out explain", p.Name)
	}
	return true, nil
}

// OpensAnyReceipt reports whether p opens every receipt without its reader being
// looked up: every token but a recording one. A caller asks it first so that only a
// recording token's open costs a read of the receipt's reader.
func OpensAnyReceipt(p config.Principal) bool {
	return !Records(p)
}

// MayOpenReceipt reports whether p may open, or verify, the receipt whose reader is
// reader (the name it carries, "" for none). A recording token opens only the receipts
// that name it: opening another's is a read of facets no receipt of its own records,
// and a verify answers the receipt's facet ids and query text. Every other token opens
// any receipt.
func MayOpenReceipt(p config.Principal, reader string) bool {
	return OpensAnyReceipt(p) || reader == p.Name
}

// OpenRefusal is the reason p may not open the receipt, for the 403.
func OpenRefusal(p config.Principal) error {
	return fmt.Errorf("%s files a receipt for every read, so it opens only the receipts that name it; "+
		"read through a projection to file one", p.Name)
}

// Scope is what a principal's lists may show: the receipts that name Reader ("" for
// every receipt), and the snapshots of Source ("" for every source's).
type Scope struct {
	Reader string
	Source string
}

// receiptRule is one row of the receipt list's scope: the first row whose when holds
// names the reader whose receipts p lists. A per-principal override later is one more
// row, above the others.
type receiptRule struct {
	when   func(config.Principal) bool
	reader func(config.Principal) string
}

// receiptRules are the receipt list's rows, in order. A recording token lists only the
// receipts that name it (Matt, 2026-10-22). With no row matching, the operator and an
// ordinary reader list every receipt, as before.
var receiptRules = []receiptRule{
	{when: Records, reader: func(p config.Principal) string { return p.Name }},
}

// ListScope is what p's lists may show. An importer sees only its own source's
// snapshots; a reader or the operator sees any.
func ListScope(p config.Principal) Scope {
	var s Scope
	for _, r := range receiptRules {
		if r.when(p) {
			s.Reader = r.reader(p)
			break
		}
	}
	if p.Role == config.RoleImporter {
		s.Source = p.Source
	}
	return s
}

// Receipts narrows a list's principal filter to s: the filter asked for when s allows
// it, s's reader when the filter is empty, and a refusal for another reader's.
func (s Scope) Receipts(asked string) (string, error) {
	switch {
	case s.Reader == "":
		return asked, nil
	case asked == "" || asked == s.Reader:
		return s.Reader, nil
	}
	return "", fmt.Errorf("%s files a receipt for every read, so it lists only the receipts that name it, not %s's",
		s.Reader, asked)
}

// Snapshot reports whether s may see a snapshot of source.
func (s Scope) Snapshot(source string) bool {
	return s.Source == "" || s.Source == source
}
