package policy

import (
	"slices"

	"github.com/mattjoyce/anam/internal/config"
)

// Hold is the operator's admission hold as a request carries it: the sources held, and
// whether every source is. It is operator-authored state in the operational store; the
// daemon reads it inside the write and passes it in, so the policy still reads nothing
// but the request and the importers file (row 30 of the card 223 survey).
type Hold struct {
	Sources []string
	All     bool
}

// Holdable reports whether p's submissions can be held at all. The workbench is never
// held: a hand entry is the operator's own, not an importer's submission. A caller asks
// it first, so an entry never reads the hold, and a hold that cannot be read refuses
// only what it could have held.
func Holdable(p config.Principal) bool {
	return MayHold(p.Source)
}

// MayHold reports whether the operator may hold source: any source but the workbench's.
func MayHold(source string) bool {
	return source != config.WorkbenchSource
}

// Held reports whether req's caller is held: it can be held, and the hold in force,
// req.Hold, names its source or every source.
func Held(req Request) bool {
	return Holdable(req.Caller) && (req.Hold.All || slices.Contains(req.Hold.Sources, req.Caller.Source))
}
