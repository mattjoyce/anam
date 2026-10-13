package read

import (
	"context"
	"errors"

	"github.com/mattjoyce/anam/internal/api"
	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/consent"
	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/mint"
	"github.com/mattjoyce/anam/internal/projection"
	"github.com/mattjoyce/anam/internal/receipt"
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
// ones alone. templates are the projections held, bundled and registered (card 125);
// nil means the bundled ones alone.
func ServeDeps(l serve.Ledger, artifacts serve.Store, checkpoints mint.CheckpointLog, defs *def.Registry,
	templates *projection.Registry,
) (serve.Deps, error) {
	var err error
	if templates == nil {
		if templates, err = projection.Bundled(); err != nil {
			return serve.Deps{}, err
		}
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
	r, shown, err := serve.Show(d, hash)
	if err != nil {
		return api.ReceiptShown{}, err
	}
	return api.ReceiptShown{Hash: hash, Receipt: r, Entries: shown.Entries, Frame: shown.Frame}, nil
}

// VerifyReceipt proves one receipt against the ledger and the artifact on disk.
func VerifyReceipt(ctx context.Context, d serve.Deps, hash string) (api.ReceiptVerified, error) {
	r, res, err := serve.Verify(ctx, d, hash)
	if err != nil {
		return api.ReceiptVerified{}, err
	}
	return api.ReceiptVerified{Hash: hash, Receipt: r, Result: res}, nil
}

// Publish performs the read q describes and files what it shows with a receipt (card
// 87). It reads as Timeline reads, at the head, an anam-time or a named W, through q's
// projection, then narrows by q's snapshot, window and type with serve.Narrow, the
// function serve.Verify replays. The receipt records W and the resolved query, never
// the zone or as_of: they only located the instants and the W it already holds.
//
// A published read names a projection, because a receipt is what a projection served
// and access is decided by projections; explain publishes nothing. Both are refused
// as bad queries. Publishing writes artifacts and nothing on the ledger.
//
// limit bounds what the publish answers with, as Fit measures it; it is checked
// before anything is filed, so a refused publish leaves no artifact. Zero is no bound.
func Publish(ctx context.Context, src Source, d serve.Deps, q api.TimelineQuery, limit int) (api.Published, error) {
	if q.Projection == "" {
		return api.Published{}, badQuery(errors.New("a published read names a projection: a receipt is what a projection served"))
	}
	if q.Explain {
		return api.Published{}, badQuery(errors.New("explain shows verdicts and publishes nothing; drop it to publish"))
	}
	loc, err := Location(q.Zone)
	if err != nil {
		return api.Published{}, err
	}
	b, at, err := parseBoundsAt(q.From, q.To, q.At, false, loc)
	if err != nil {
		return api.Published{}, badQuery(err)
	}
	know, err := knowledgeOf(q, loc)
	if err != nil {
		return api.Published{}, err
	}
	// The view is the timeline read's own, snapshot included, so the header a reader
	// sees is the one they would see without publishing. Narrow then takes the
	// snapshot again, which keeps the same value (a snapshot of a snapshot is itself),
	// so the published value is Narrow over the template's: the one Verify replays.
	v, err := readView(ctx, src, know, q.Projection, at)
	if err != nil {
		return api.Published{}, err
	}
	if err := v.checkFields(q); err != nil {
		return api.Published{}, err
	}

	rq := &receipt.Query{From: b.from, Before: b.toX, At: at, Type: q.Type, Fields: q.Fields}
	if rq.IsZero() {
		rq = nil
	}
	value, err := serve.Narrow(v.shown, rq, v.defs)
	if err != nil {
		return api.Published{}, err
	}
	// The bound is measured over what is answered: the frame's rows when fields
	// were named, else the whole entries. Cut once here for the measure; Publish
	// cuts again for the receipt, and the two are the same value.
	fr, err := serve.Cut(value, rq)
	if err != nil {
		return api.Published{}, badQuery(err)
	}
	if fr != nil {
		err = fitQuery(fr.Rows, limit, q)
	} else {
		err = fitQuery(value.Entries(), limit, q)
	}
	if err != nil {
		return api.Published{}, err
	}
	served, err := serve.Publish(ctx, d, *v.tmpl, v.res.W, rq, value, false)
	if err != nil {
		return api.Published{}, err
	}

	t := v.describe()
	if served.Frame != nil {
		t.Frame = served.Frame
		t.Shown = served.Frame.Len()
		return api.Published{Timeline: t, ReceiptHash: served.ReceiptHash, Receipt: served.Receipt}, nil
	}
	t.Entries = value.Entries()
	ids := make([]string, 0, len(t.Entries))
	for _, e := range t.Entries {
		ids = append(ids, e.FacetID)
	}
	if t.Versions, err = v.versionsOf(ctx, src.Ledger, ids); err != nil {
		return api.Published{}, err
	}
	t.Shown = len(t.Entries)
	return api.Published{Timeline: t, ReceiptHash: served.ReceiptHash, Receipt: served.Receipt}, nil
}
