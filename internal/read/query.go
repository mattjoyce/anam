package read

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mattjoyce/anam/internal/api"
	"github.com/mattjoyce/anam/internal/consent"
	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/fold"
	"github.com/mattjoyce/anam/internal/projection"
	"github.com/mattjoyce/anam/internal/query"
	"github.com/mattjoyce/anam/internal/receipt"
	"github.com/mattjoyce/anam/internal/serve"
	"github.com/mattjoyce/anam/internal/store/ledger"
)

// The third read (ARCHITECTURE §9, card 173): a Datalog program evaluated over what a
// projection serves at a watermark, answered with a receipt.

// queryNarrowings is what a refused query can still do: ask about fewer instants,
// through a narrower projection, with a more selective program.
var queryNarrowings = []string{"instant", "projection", "program"}

const queryHint = "ask about fewer instants (instant/1), through a narrower projection, or with a more selective program"

// held is src's Definition bodies by type and version, or its catalog's.
func (src Source) held() (query.Held, error) {
	if src.Held != nil {
		return src.Held, nil
	}
	if src.Catalog == nil {
		return nil, ErrNoCatalog
	}
	return query.HeldOf(src.Catalog.Manifests()), nil
}

// Describe is the introspection a query is written against: the relations, every
// type's pointers at every held version, the instant lexeme and the bundled rules.
func Describe(src Source) (api.QueryCatalogue, error) {
	defs, err := src.definitions()
	if err != nil {
		return api.QueryCatalogue{}, err
	}
	held, err := src.held()
	if err != nil {
		return api.QueryCatalogue{}, err
	}
	return query.Describe(defs, held)
}

// Query answers q: the full fold at the knowledge it names, served through its
// projection, the program run over that value within limit (the derivation and the
// answer both; zero is no bound), and the answer filed with a receipt unless it is a
// dry run. A bad program is the reader's error, refused before anything is read.
func Query(ctx context.Context, src Source, d serve.Deps, q api.QueryRequest, limit int) (api.QueryAnswer, error) {
	prog, err := checkQueryRequest(q)
	if err != nil {
		return api.QueryAnswer{}, err
	}
	// The zone only resolves a bare date in as_of; a query at the head or at a W
	// names no date and needs none.
	loc := time.UTC
	if q.AsOf != "" {
		if loc, err = Location(q.Zone); err != nil {
			return api.QueryAnswer{}, err
		}
	}
	know, err := knowledgeOf(api.TimelineQuery{AsOf: q.AsOf, W: q.W}, loc)
	if err != nil {
		return api.QueryAnswer{}, err
	}
	st, err := stand(ctx, src, know, q.Projection)
	if err != nil {
		return api.QueryAnswer{}, err
	}
	defs, t, res, served, w, v := st.defs, st.tmpl, st.res, st.served, st.w, st.view
	ans, rules, err := Evaluate(ctx, served, res.Live, defs, q.Text, q.Relation, limit)
	if err != nil {
		return api.QueryAnswer{}, err
	}
	held, err := src.held()
	if err != nil {
		return api.QueryAnswer{}, err
	}
	warnings, err := query.PointerWarnings([]query.Program{prog}, held)
	if err != nil {
		return api.QueryAnswer{}, err
	}

	program := receipt.Program{RuleSet: rules.Name, RuleSetVersion: rules.Version, RuleSetHash: rules.Hash, Text: q.Text, Relation: q.Relation}
	out, err := serve.PublishQuery(ctx, d, t, w, program, served, ans, q.DryRun)
	if err != nil {
		return api.QueryAnswer{}, err
	}
	return api.QueryAnswer{
		W: int64(w), LedgerID: out.Receipt.LedgerID, AsOf: v.asOfAt, Live: res.Value.Len(), Canonical: res.Value.Hash(),
		Projection:           api.ProjectionRead{Name: t.Name, Version: t.Version, Served: served.Len(), Hash: served.Hash()},
		DefinitionSetVersion: d.DefinitionSetVersion,
		Rules:                api.RuleSetRef{Name: rules.Name, Version: rules.Version, Hash: rules.Hash},
		Answer:               ans, Hash: ans.Hash(), Warnings: warnings,
		ReceiptHash: out.ReceiptHash, Receipt: out.Receipt, Written: out.Written,
	}, nil
}

// Evaluate is the query read's evaluation without its receipt: text run over served,
// a value a projection served from the fold whose live facets are live, within limit
// (the derivation and the answer both; zero is no bound), and relation answered. A bad
// program is the reader's error and a derivation or answer over the bound is a
// *TooLarge, as Query refuses them. The inspector's selection page (card 199) runs a
// program through here, so the page and POST /v1/query answer one program alike.
func Evaluate(ctx context.Context, served fold.Value, live []facet.Facet, defs query.Definitions,
	text, relation string, limit int,
) (query.Answer, query.RuleSet, error) {
	ans, rules, err := query.Run(ctx, served, query.Versions(live), defs, text, relation, limit)
	if err != nil {
		return query.Answer{}, rules, asReadError(err)
	}
	if err := fitAnswer(ans, limit); err != nil {
		return query.Answer{}, rules, err
	}
	return ans, rules, nil
}

// standing is what a query stands on: the Definitions, the projection, the full fold
// at the watermark its knowledge names, and the value the projection served from it.
type standing struct {
	defs   *def.Registry
	tmpl   projection.Template
	res    fold.Result
	served fold.Value
	w      ledger.Watermark
	view   view
}

// stand reads the full fold at the knowledge asked for and serves it through the
// projection. The full fold, not the composed read: the base relations carry each
// facet's Definition version, which only the fold's live facets hold.
func stand(ctx context.Context, src Source, know knowledge, ref api.ProjectionRef) (standing, error) {
	defs, err := src.definitions()
	if err != nil {
		return standing{}, err
	}
	t, err := queryTemplate(src, defs, ref)
	if err != nil {
		return standing{}, err
	}
	var v view
	w, err := watermarkOf(ctx, src.Ledger, know, &v)
	if err != nil {
		return standing{}, err
	}
	res, err := fold.ReadAt(ctx, src.Ledger, w)
	if err != nil {
		return standing{}, err
	}
	served, err := serve.Derive(res.Value, t, consent.Open{}, defs, "")
	if err != nil {
		return standing{}, err
	}
	return standing{defs: defs, tmpl: t, res: res, served: served, w: w, view: v}, nil
}

// queryTemplate is the template a query reads through: the registered or bundled one
// ref names, newest version, or the inline one ref carries (card 211), checked by the
// loader a registered manifest is and against the Definitions in force. Nothing has
// been read when it refuses, and a bad template is the reader's error naming the field.
func queryTemplate(src Source, defs *def.Registry, ref api.ProjectionRef) (projection.Template, error) {
	if len(ref.Inline) == 0 {
		templates, err := src.projections()
		if err != nil {
			return projection.Template{}, err
		}
		t, err := templates.Lookup(ref.Name)
		if err != nil {
			return projection.Template{}, badQuery(err)
		}
		return t, nil
	}
	t, err := projection.ParseInline(ref.Inline)
	if err != nil {
		return projection.Template{}, badQuery(err)
	}
	if err := t.CheckDefined(func(ft facet.Type) bool { return defs.Has(ft) }); err != nil {
		return projection.Template{}, badQuery(err)
	}
	return t, nil
}

// checkQueryRequest refuses a request that cannot run, and parses its program.
func checkQueryRequest(q api.QueryRequest) (query.Program, error) {
	switch {
	case q.Projection.IsZero():
		return query.Program{}, badQuery(errors.New("a query runs through a projection; name one, or give the template inline"))
	case q.Projection.Name != "" && len(q.Projection.Inline) > 0:
		return query.Program{}, badQuery(errors.New("a projection is named or inline, not both"))
	case strings.TrimSpace(q.Text) == "":
		return query.Program{}, badQuery(errors.New("the query has no program"))
	case q.Relation == "":
		return query.Program{}, badQuery(errors.New("name the relation to answer"))
	}
	prog, err := query.Parse(q.Text)
	if err != nil {
		return query.Program{}, badQuery(err)
	}
	return prog, nil
}

// asReadError maps the evaluator's refusals onto the read's: a bad program is the
// reader's error, a derivation over the bound is too large with its measure.
func asReadError(err error) error {
	var tl *query.TooLarge
	switch {
	case errors.Is(err, query.ErrSyntax), errors.Is(err, query.ErrInvalid), errors.Is(err, query.ErrBase):
		return badQuery(err)
	case errors.As(err, &tl) && tl.Bound == query.BoundNumber:
		return badQuery(err)
	case errors.As(err, &tl):
		tooLarge := api.TooLarge{Matched: tl.Tuples, Narrow: queryNarrowings}
		if tl.Bound == query.BoundSteps {
			tooLarge.Steps = tl.Steps
			tooLarge.MaxSteps = tl.MaxSteps
		} else {
			tooLarge.Bytes = tl.Bytes
			tooLarge.MaxBytes = tl.MaxBytes
		}
		return &TooLarge{
			TooLarge: tooLarge,
			Hint:     fmt.Sprintf("the derivation grew past the bound at %s; %s", tl.Relation, queryHint),
		}
	}
	return err
}

// fitAnswer is Fit over the answer's tuples, with the query's narrowings.
func fitAnswer(ans query.Answer, limit int) error {
	err := Fit(ans.Tuples(), limit, queryHint)
	var tl *TooLarge
	if errors.As(err, &tl) {
		tl.Narrow = queryNarrowings
	}
	return err
}
