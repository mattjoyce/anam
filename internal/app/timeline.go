package app

import (
	"context"
	"encoding/json"
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/fold"
	"github.com/mattjoyce/anam/internal/store/ledger"
)

func timelineCmd(cfgPath *string) *cobra.Command {
	var from, to, typeFilter string
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "timeline",
		Short: "the whole-person read: live facets in event-time order (the inversion)",
		Long: `Timeline is Anam's default read: one person's live facts, whole, in event-time
order. No span is required — narrowing with --from/--to or --type is a lens over
the same read, never a different one.

The canonical hash printed in the header identifies the whole-person value at the
watermark W, regardless of any narrowing: stable when nothing changed, moved by
any ingest that changed the live set, identical after a rebuild from source.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			bounds, err := parseBounds(from, to)
			if err != nil {
				return err
			}

			cfg, err := loadConfig(*cfgPath, cmd)
			if err != nil {
				return err
			}
			l, err := ledger.New(cmd.Context(), cfg.DDBEndpoint, cfg.DDBRegion, cfg.TableName, cfg.PersonID)
			if err != nil {
				return err
			}

			ctx, cancel := context.WithTimeout(cmd.Context(), cfg.Timeout)
			defer cancel()
			res, err := fold.Read(ctx, l)
			if err != nil {
				return err
			}

			var shown []fold.Entry
			for _, e := range res.Value.Entries() {
				if bounds.admits(e) && (typeFilter == "" || string(e.Type) == typeFilter) {
					shown = append(shown, e)
				}
			}

			header := fmt.Sprintf("W=%d  live=%d  shown=%d  canonical=%s\n",
				res.W, res.Value.Len(), len(shown), res.Value.Hash())

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
			_, _ = fmt.Fprintln(w, "EVENT TIME\tTYPE\tV\tSTATUS\tSOURCE REF\tID")
			for _, e := range shown {
				_, _ = fmt.Fprintf(w, "%s\t%s\tv%d\t%s\t%s\t%s\n",
					e.EventTime, e.Type, res.States[e.FacetID].Version, e.Status, e.SourceRef, e.FacetID)
			}
			return w.Flush()
		},
	}

	cmd.Flags().StringVar(&from, "from", "", "narrow to event-times at or after this instant (2006-01-02 or RFC 3339)")
	cmd.Flags().StringVar(&to, "to", "", "narrow to event-times at or before this instant; a bare date includes its whole day")
	cmd.Flags().StringVar(&typeFilter, "type", "", "narrow to one facet type (e.g. health.blood-pressure)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit the narrowed entries as JSON, in the canonical serialisation")
	return cmd
}

// bounds is a derived span over event-time: an inclusive [from, to] the person
// names (ARCHITECTURE.md §4), held as clock.Layout strings so admission is the
// same lexical comparison the entries were sorted by.
type bounds struct {
	from string // inclusive lower, "" for unbounded
	toX  string // exclusive upper, "" for unbounded
}

func (b bounds) admits(e fold.Entry) bool {
	if b.from != "" && e.EventTime < b.from {
		return false
	}
	if b.toX != "" && e.EventTime >= b.toX {
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
