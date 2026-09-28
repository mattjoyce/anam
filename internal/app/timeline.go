package app

import (
	"context"
	"encoding/json"
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/consent"
	"github.com/mattjoyce/anam/internal/fold"
	"github.com/mattjoyce/anam/internal/projection"
)

func timelineCmd(cfgPath *string) *cobra.Command {
	var from, to, typeFilter, projName, asOf string
	var asJSON, explain bool

	cmd := &cobra.Command{
		Use:   "timeline",
		Short: "the whole-person read: live facets in fact-time order (the inversion)",
		Long: `Timeline is Anam's default read: one person's live facts, whole, in fact-time
order. No span is required — narrowing with --from/--to or --type is a lens over
the same read, never a different one.

The canonical hash printed in the header identifies the whole-person value at the
watermark W, regardless of any narrowing: stable when nothing changed, moved by
any ingest that changed the live set, identical after a rebuild from source.

Three lenses compose over that read:

  --projection  switch to a governed variant — the same read, pruned by a bundled
                template. The served hash is the one a receipt would record, so
                what you see here is what serve would publish.
  --as-of       wind Anam's clock back to an anam-time and read what it knew then.
                Fact-time (--from/--to) asks when things happened; anam-time
                asks when Anam found out.
  --explain     show every facet with the verdict on it rather than only the ones
                that survived — including the artifact versions since superseded.
                This is the same decision procedure the projection applies, not a
                second account of it.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			bounds, err := parseBounds(from, to)
			if err != nil {
				return err
			}
			asOfTime, err := parseAsOf(asOf)
			if err != nil {
				return err
			}

			cfg, err := loadConfig(*cfgPath, cmd)
			if err != nil {
				return err
			}
			l, err := openLedgerReadOnly(cmd.Context(), cfg)
			if err != nil {
				return err
			}
			defer func() { _ = l.Close() }()

			ctx, cancel := context.WithTimeout(cmd.Context(), cfg.Timeout)
			defer cancel()

			view, err := readView(ctx, l, asOfTime, projName)
			if err != nil {
				return err
			}
			if explain {
				return writeExplain(cmd, ctx, l, view, bounds, typeFilter)
			}
			return writeTimeline(cmd, view, bounds, typeFilter, asJSON)
		},
	}

	cmd.Flags().StringVar(&from, "from", "", "narrow to fact-times at or after this instant (2006-01-02 or RFC 3339)")
	cmd.Flags().StringVar(&to, "to", "", "narrow to fact-times at or before this instant; a bare date includes its whole day")
	cmd.Flags().StringVar(&typeFilter, "type", "", "narrow to one facet type (e.g. health.blood-pressure)")
	cmd.Flags().StringVar(&projName, "projection", "", "read through a bundled projection instead of the canonical (see: anam serve)")
	cmd.Flags().StringVar(&asOf, "as-of", "", "read what Anam knew at this anam-time (2006-01-02 or RFC 3339)")
	cmd.Flags().BoolVar(&explain, "explain", false, "show every facet with its verdict, superseded versions included")
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit the narrowed entries as JSON, in the canonical serialisation")
	return cmd
}

// parseAsOf reads the --as-of anam-time. A bare date means "as Anam knew it at the
// end of that day", which is deliberately NOT --to's rule: --to feeds an exclusive
// comparison and so rolls to the next midnight, while --as-of names one instant to read
// at. Rolling it forward too would silently read a day further than was asked for.
func parseAsOf(asOf string) (time.Time, error) {
	if asOf == "" {
		return time.Time{}, nil
	}
	t, dateOnly, err := parseInstant(asOf)
	if err != nil {
		return time.Time{}, fmt.Errorf("--as-of: %w", err)
	}
	if dateOnly {
		t = t.AddDate(0, 0, 1).Add(-time.Nanosecond)
	}
	return t, nil
}

// view is one read of the person, optionally wound back and optionally pruned.
type view struct {
	res fold.Result

	// tmpl and served are set only when a projection was named. shown is whichever
	// value the caller should render — the canonical, or the served variant.
	tmpl   *projection.Template
	shown  fold.Value
	asOf   bool
	asOfAt string
}

// readView performs the read the flags describe: at head or at an anam-time, through
// a projection or not.
func readView(ctx context.Context, l fold.Ledger, asOfTime time.Time, projName string) (view, error) {
	var v view

	res, err := readAtOrHead(ctx, l, asOfTime, &v)
	if err != nil {
		return view{}, err
	}
	v.res, v.shown = res, res.Value

	if projName == "" {
		return v, nil
	}
	templates, err := projection.Bundled()
	if err != nil {
		return view{}, err
	}
	t, err := templates.Lookup(projName)
	if err != nil {
		return view{}, err
	}
	served, err := projection.Apply(res.Value, t, consent.Open{})
	if err != nil {
		return view{}, err
	}
	v.tmpl, v.shown = &t, served
	return v, nil
}

func readAtOrHead(ctx context.Context, l fold.Ledger, asOfTime time.Time, v *view) (fold.Result, error) {
	if asOfTime.IsZero() {
		return fold.Read(ctx, l)
	}
	w, err := fold.WatermarkAt(ctx, l, asOfTime)
	if err != nil {
		return fold.Result{}, err
	}
	v.asOf, v.asOfAt = true, clock.Format(asOfTime)
	return fold.ReadAt(ctx, l, w)
}

// header describes the read: which watermark, which lens, and the hash that identifies
// what is being shown. When a projection is in force the served hash is the one a
// receipt would carry, so the viewer and the publisher can be checked against each
// other by eye.
func (v view) header(shownCount int) string {
	h := fmt.Sprintf("W=%d  live=%d", v.res.W, v.res.Value.Len())
	if v.tmpl != nil {
		h += fmt.Sprintf("  served=%d", v.shown.Len())
	}
	h += fmt.Sprintf("  shown=%d\n", shownCount)

	if v.asOf {
		h += fmt.Sprintf("as-of      %s (anam-time)\n", v.asOfAt)
	}
	h += fmt.Sprintf("canonical  %s\n", v.res.Value.Hash())
	if v.tmpl != nil {
		h += fmt.Sprintf("projection %s v%d  %s\n", v.tmpl.Name, v.tmpl.Version, v.shown.Hash())
	}
	return h
}

func writeTimeline(cmd *cobra.Command, v view, b bounds, typeFilter string, asJSON bool) error {
	var shown []fold.Entry
	for _, e := range v.shown.Entries() {
		if b.admits(e) && (typeFilter == "" || string(e.Type) == typeFilter) {
			shown = append(shown, e)
		}
	}
	header := v.header(len(shown))

	if asJSON {
		// Entries to stdout, bookkeeping to stderr, so the output pipes clean.
		_, _ = fmt.Fprint(cmd.ErrOrStderr(), header)
		out, err := json.Marshal(shown)
		if err != nil {
			return fmt.Errorf("timeline: marshal entries: %w", err)
		}
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s\n", out)
		return err
	}

	_, _ = fmt.Fprint(cmd.OutOrStdout(), header)
	w := tabwriter.NewWriter(cmd.OutOrStdout(), 2, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "FACT TIME\tTYPE\tV\tSTATUS\tSOURCE REF\tID")
	for _, e := range shown {
		_, _ = fmt.Fprintf(w, "%s\t%s\tv%d\t%s\t%s\t%s\n",
			e.FactTime, e.Type, v.res.States[e.FacetID].Version, e.Status, e.SourceRef, e.FacetID)
	}
	return w.Flush()
}

// bounds is a derived span over fact-time: an inclusive [from, to] the person
// names (ARCHITECTURE.md §4), held as clock.Layout strings so admission is the
// same lexical comparison the entries were sorted by.
type bounds struct {
	from string // inclusive lower, "" for unbounded
	toX  string // exclusive upper, "" for unbounded
}

func (b bounds) admits(e fold.Entry) bool {
	if b.from != "" && e.FactTime < b.from {
		return false
	}
	if b.toX != "" && e.FactTime >= b.toX {
		return false
	}
	return true
}

func parseBounds(from, to string) (bounds, error) {
	var b bounds
	if from != "" {
		t, _, err := parseInstant(from)
		if err != nil {
			return bounds{}, fmt.Errorf("--from: %w", err)
		}
		b.from = clock.Format(t)
	}
	if to != "" {
		t, dateOnly, err := parseInstant(to)
		if err != nil {
			return bounds{}, fmt.Errorf("--to: %w", err)
		}
		// The interval is inclusive; the comparison is exclusive. A bare date rolls
		// to the next midnight so it includes its whole day; an instant nudges one
		// nanosecond — the wire format's full precision — so ≤t becomes <t'.
		if dateOnly {
			t = t.AddDate(0, 0, 1)
		} else {
			t = t.Add(time.Nanosecond)
		}
		b.toX = clock.Format(t)
	}
	return b, nil
}

// parseInstant reads a bound as a bare UTC date or an RFC 3339 instant.
func parseInstant(s string) (t time.Time, dateOnly bool, err error) {
	if t, err := time.ParseInLocation("2006-01-02", s, time.UTC); err == nil {
		return t, true, nil
	}
	t, err = time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, false, fmt.Errorf("%q is neither 2006-01-02 nor RFC 3339", s)
	}
	return t, false, nil
}
