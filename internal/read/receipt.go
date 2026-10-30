package read

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/mattjoyce/anam/internal/api"
	"github.com/mattjoyce/anam/internal/catalog"
	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/consent"
	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/mint"
	"github.com/mattjoyce/anam/internal/projection"
	"github.com/mattjoyce/anam/internal/receipt"
	"github.com/mattjoyce/anam/internal/serve"
)

// ServeDeps assembles what a serve or a verify needs around a ledger and the store
// that holds receipts: the bundled templates and Definitions, and the consent gate. The gate is the
// pass-through one (§10). It is wired in on the real path rather than reserved for
// later, because a gate threaded through afterwards is a gate a caller can forget.
//
// checkpoints is the mint's log, so a serve or a verify composes from the last mint
// run rather than folding the ledger from the beginning; nil folds in full. cat is the
// provisioned catalog the process loaded: a receipt names its definition set version,
// however many Definitions were registered beside it. defs are the Definitions the
// record's facets were admitted against; nil means cat's alone. templates are the
// projections held, provisioned and registered (card 125); nil means cat's alone.
func ServeDeps(cat *catalog.Catalog, l serve.Ledger, published serve.Store, checkpoints mint.CheckpointLog,
	defs *def.Registry, templates *projection.Registry,
) (serve.Deps, error) {
	if cat == nil {
		return serve.Deps{}, ErrNoCatalog
	}
	if templates == nil {
		templates = cat.Projections()
	}
	if defs == nil {
		defs = cat.Definitions()
	}
	defSet := cat.SetVersion()
	return serve.Deps{
		Ledger:               l,
		Templates:            templates,
		Published:            published,
		Checkpoints:          checkpoints,
		Gate:                 consent.Open{},
		Clock:                clock.Real{},
		DefinitionSetVersion: defSet,
		Series:               defs,
		Definitions:          defs,
	}, nil
}

// Receipts lists the receipts in d's store that pass q, newest serve first. store names
// it for the reader. A q that names no part lists them all.
func Receipts(ctx context.Context, d serve.Deps, store string, q api.ReceiptsQuery) (api.Receipts, error) {
	f, err := listFilterOf(q)
	if err != nil {
		return api.Receipts{}, err
	}
	receipts, hashes, err := serve.List(ctx, d, f)
	if err != nil {
		return api.Receipts{}, err
	}
	out := api.Receipts{Store: store, Receipts: make([]api.ReceiptListing, 0, len(receipts))}
	for i, r := range receipts {
		out.Receipts = append(out.Receipts, api.ReceiptListing{Hash: hashes[i], Receipt: r})
	}
	return out, nil
}

// listFilterOf resolves q's instants. A bare date is a day in the reader's zone, so it
// needs one; an RFC 3339 instant names its own. A bare date for Until is the whole
// day, so the bound is the start of the next.
func listFilterOf(q api.ReceiptsQuery) (serve.ListFilter, error) {
	f := serve.ListFilter{Principal: q.Principal}
	instant := func(name, s string) (time.Time, error) {
		t, dateOnly, err := parseInstant(s, nil)
		if err != nil {
			return time.Time{}, badQuery(fmt.Errorf("%s: %w", name, err))
		}
		if !dateOnly {
			return t, nil
		}
		loc, err := Location(q.Zone)
		if err != nil {
			return time.Time{}, err
		}
		t, _, _ = parseInstant(s, loc)
		if name == "until" {
			t = t.AddDate(0, 0, 1)
		}
		return t, nil
	}
	if q.Since != "" {
		t, err := instant("since", q.Since)
		if err != nil {
			return serve.ListFilter{}, err
		}
		f.Since = clock.Format(t)
	}
	if q.Until != "" {
		t, err := instant("until", q.Until)
		if err != nil {
			return serve.ListFilter{}, err
		}
		f.Until = clock.Format(t)
	}
	return f, nil
}

// ReceiptReader names the principal a stored receipt files, empty when it names none.
// The daemon asks it to keep a token that records every read from opening a value
// nobody recorded it reading.
func ReceiptReader(ctx context.Context, d serve.Deps, hash string) (string, error) {
	raw, err := d.Published.Get(ctx, receipt.Kind, hash)
	if err != nil {
		return "", err
	}
	r, err := receipt.Parse(raw)
	if err != nil {
		return "", err
	}
	return r.Principal, nil
}

// VerifyReceipt proves one receipt against the ledger and the stored value.
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
// as bad queries. Publishing writes receipts and nothing on the ledger.
//
// limit bounds what the publish answers with, as Fit measures it (card 92). The read
// is filed whole, once, and answered a page at a time: the first page here, the rest
// by ReceiptPage from the stored value. Only a facet over limit alone, which no page can
// carry, is refused, and that is checked before anything is filed, so a refused
// publish stores nothing. Zero is no bound, one page.
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
	if err := checkType(q.Type); err != nil {
		return api.Published{}, err
	}
	know, err := knowledgeOf(q, loc)
	if err != nil {
		return api.Published{}, err
	}
	// The view is the timeline read's own, snapshot included, so the header a reader
	// sees is the one they would see without publishing. Narrow then takes the
	// snapshot again at the same at, which keeps the same value, so the published
	// value is Narrow over serve.Derive's: the one Verify replays.
	v, err := readView(ctx, src, know, q.Projection, at)
	if err != nil {
		return api.Published{}, err
	}
	if err := v.checkFields(q); err != nil {
		return api.Published{}, err
	}

	rq := queryOf(b, at, q.Type, q.Fields)
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
	ids := make([]string, 0, value.Len())
	for _, e := range value.Entries() {
		ids = append(ids, e.FacetID)
	}
	narrowings := Narrowings(q)
	hint := "no page can carry it; " + NarrowHint(narrowings)
	if fr != nil {
		err = fitEach(fr.Rows, ids, limit, hint)
	} else {
		err = fitEach(value.Entries(), ids, limit, hint)
	}
	var tl *TooLarge
	if errors.As(err, &tl) {
		tl.Narrow = narrowings
	}
	if err != nil {
		return api.Published{}, err
	}
	served, err := serve.Publish(ctx, d, *v.tmpl, v.res.W, rq, value, false)
	if err != nil {
		return api.Published{}, err
	}

	// The first page is read back from what was just filed, by the function every
	// later page is read by, so no page is cut two ways. The answer's header and
	// versions are the read's own, as a timeline read's are; the pages after it are
	// the stored value's alone.
	first, err := ReceiptPage(ctx, d, served.ReceiptHash, "", limit)
	if err != nil {
		return api.Published{}, err
	}
	t := v.describe()
	// Shown is what the narrowing kept, the whole published value, and so the page's
	// total_count; the page says which of it this answer carries.
	t.Shown = len(ids)
	out := api.Published{ReceiptHash: served.ReceiptHash, Receipt: served.Receipt, Page: *first.Page}
	if first.Frame != nil {
		t.Frame = first.Frame
		out.Timeline = t
		return out, nil
	}
	t.Entries = first.Entries
	if t.Versions, err = v.versionsOf(ctx, src.Ledger, ids[:first.Page.ReturnedCount]); err != nil {
		return api.Published{}, err
	}
	out.Timeline = t
	return out, nil
}
