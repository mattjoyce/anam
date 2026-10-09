package read

import (
	"context"

	"github.com/mattjoyce/anam/internal/api"
	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/consent"
	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/mint"
	"github.com/mattjoyce/anam/internal/projection"
	"github.com/mattjoyce/anam/internal/serve"
)

// ServeDeps assembles what a serve or a verify needs around a ledger and an artifact
// store: the bundled templates and Definitions, and the consent gate. The gate is the
// pass-through one (§10). It is wired in on the real path rather than reserved for
// later, because a gate threaded through afterwards is a gate a caller can forget.
//
// checkpoints is the mint's log, so a serve or a verify composes from the last mint
// run rather than folding the ledger from the beginning; nil folds in full. defs are
// the Definitions the record's facets were admitted against; nil means the bundled
// ones alone.
func ServeDeps(l serve.Ledger, artifacts serve.Store, checkpoints mint.CheckpointLog, defs *def.Registry,
) (serve.Deps, error) {
	templates, err := projection.Bundled()
	if err != nil {
		return serve.Deps{}, err
	}
	defSet, err := def.SetVersion()
	if err != nil {
		return serve.Deps{}, err
	}
	if defs == nil {
		if defs, err = def.Bundled(); err != nil {
			return serve.Deps{}, err
		}
	}
	return serve.Deps{
		Ledger:               l,
		Templates:            templates,
		Artifacts:            artifacts,
		Checkpoints:          checkpoints,
		Gate:                 consent.Open{},
		Clock:                clock.Real{},
		DefinitionSetVersion: defSet,
		Series:               defs,
	}, nil
}

// Receipts lists the receipts in d's artifact store, newest serve first. dir names
// the store for the reader.
func Receipts(d serve.Deps, dir string) (api.Receipts, error) {
	receipts, hashes, err := serve.List(d)
	if err != nil {
		return api.Receipts{}, err
	}
	out := api.Receipts{Dir: dir, Receipts: make([]api.ReceiptListing, 0, len(receipts))}
	for i, r := range receipts {
		out.Receipts = append(out.Receipts, api.ReceiptListing{Hash: hashes[i], Receipt: r})
	}
	return out, nil
}

// Receipt reads one receipt and the served value it names from the artifacts alone.
func Receipt(d serve.Deps, hash string) (api.ReceiptShown, error) {
	r, entries, err := serve.Show(d, hash)
	if err != nil {
		return api.ReceiptShown{}, err
	}
	return api.ReceiptShown{Hash: hash, Receipt: r, Entries: entries}, nil
}

// VerifyReceipt proves one receipt against the ledger and the artifact on disk.
func VerifyReceipt(ctx context.Context, d serve.Deps, hash string) (api.ReceiptVerified, error) {
	r, res, err := serve.Verify(ctx, d, hash)
	if err != nil {
		return api.ReceiptVerified{}, err
	}
	return api.ReceiptVerified{Hash: hash, Receipt: r, Result: res}, nil
}
