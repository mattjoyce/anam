package app

import (
	"context"
	"encoding/json"
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/mattjoyce/anam/internal/api"
	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/config"
	"github.com/mattjoyce/anam/internal/read"
	"github.com/mattjoyce/anam/internal/store/artifact"
	"github.com/mattjoyce/anam/internal/store/ops"
)

func timelineCmd(cfgPath *string) *cobra.Command {
	var from, to, at, typeFilter, projName, asOf string
	var w int64
	var asJSON, explain, publish bool

	cmd := &cobra.Command{
		Use:   "timeline",
		Short: "the whole-person read: live facets in fact-time order (the inversion)",
		Long: `Timeline is Anam's default read: one person's live facts, whole, in fact-time
order. No span is required — narrowing with --from/--to or --type is a lens over
the same read, never a different one.

The canonical hash printed in the header identifies the whole-person value at the
watermark W, regardless of any narrowing: stable when nothing changed, moved by
any ingest that changed the live set, identical after a rebuild from source.

Four lenses compose over that read:

  --projection  switch to a governed variant — the same read, pruned by a bundled
                template. The served hash is the one a receipt would record, so
                what you see here is what serve would publish.
  --as-of       wind Anam's clock back to an anam-time and read what it knew then.
                Fact-time (--from/--to) asks when things happened; anam-time
                asks when Anam found out.
  --at          the snapshot: what was true at a fact-time. One facet per series,
                the latest at or before then, a closed series gone, facts without
                a series left alone. --as-of then --at is the bitemporal answer:
                what Anam believed on T was true at D.
  --explain     show every facet with the verdict on it rather than only the ones
                that survived — including the artifact versions since superseded.
                This is the same decision procedure the projection applies, not a
                second account of it.
  --w           read at this watermark instead of an anam-time: the W a receipt
                names, so a follow-up stands on the same knowledge as a citation.
  --publish     file what is shown with a receipt, as serve does, narrowed as
                asked (needs --projection). Verify it with anam receipt verify.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := loadRoutedConfig(*cfgPath, cmd)
			if err != nil {
				return err
			}
			loc, err := cfg.Location()
			if err != nil {
				return err
			}
			q := api.TimelineQuery{
				From: from, To: to, At: at, AsOf: asOf, W: w, Type: typeFilter, Projection: projName,
				Explain: explain,
			}
			if publish {
				p, err := publishTimeline(cmd.Context(), cfg, q)
				if err != nil {
					return err
				}
				if err := writeTimeline(cmd, p.Timeline, loc, asJSON); err != nil {
					return err
				}
				r := p.Receipt
				_, err = fmt.Fprintf(cmd.ErrOrStderr(), "published %s v%d at W=%d: receipt %s\n",
					r.Template, r.TemplateVersion, r.Watermark, p.ReceiptHash)
				return err
			}
			t, err := readTimeline(cmd.Context(), cfg, q)
			if err != nil {
				return err
			}
			if explain {
				return writeExplain(cmd, t)
			}
			return writeTimeline(cmd, t, loc, asJSON)
		},
	}

	cmd.Flags().StringVar(&from, "from", "", "narrow to fact-times at or after this instant (2006-01-02 or RFC 3339)")
	cmd.Flags().StringVar(&to, "to", "", "narrow to fact-times at or before this instant; a bare date includes its whole day")
	cmd.Flags().StringVar(&typeFilter, "type", "", "narrow to one facet type (e.g. health.blood-pressure)")
	cmd.Flags().StringVar(&projName, "projection", "", "read through a bundled projection instead of the canonical (see: anam serve)")
	cmd.Flags().StringVar(&asOf, "as-of", "", "read what Anam knew at this anam-time (2006-01-02 or RFC 3339)")
	cmd.Flags().StringVar(&at, "at", "", "the snapshot at this fact-time: latest per series at or before it (2006-01-02 or RFC 3339); excludes --to")
	cmd.Flags().Int64Var(&w, "w", 0, "read at this watermark (a receipt's W) instead of the head; excludes --as-of")
	cmd.Flags().BoolVar(&publish, "publish", false, "publish what is shown with a receipt (needs --projection)")
	cmd.Flags().BoolVar(&explain, "explain", false, "show every facet with its verdict, superseded versions included")
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit the narrowed entries as JSON, in the canonical serialisation")
	return cmd
}

// readTimeline asks the daemon when a server is configured and reads the local
// ledger when not. Either way the reader's zone goes with the query.
func readTimeline(ctx context.Context, cfg config.Config, q api.TimelineQuery) (api.Timeline, error) {
	if cfg.Server != "" {
		zone, err := cfg.ZoneName()
		if err != nil {
			return api.Timeline{}, err
		}
		q.Zone = zone
		c, err := newClient(cfg)
		if err != nil {
			return api.Timeline{}, err
		}
		return c.Timeline(ctx, q)
	}
	q.Zone = cfg.Zone
	src, closeAll, err := readSource(ctx, cfg)
	if err != nil {
		return api.Timeline{}, err
	}
	defer closeAll()
	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()
	return read.Timeline(ctx, src, q)
}

// publishTimeline publishes the read q describes, through the daemon when a server
// is configured and into the local artifact directory when not.
func publishTimeline(ctx context.Context, cfg config.Config, q api.TimelineQuery) (api.Published, error) {
	if cfg.Server != "" {
		zone, err := cfg.ZoneName()
		if err != nil {
			return api.Published{}, err
		}
		q.Zone = zone
		c, err := newClient(cfg)
		if err != nil {
			return api.Published{}, err
		}
		return c.Publish(ctx, q)
	}
	q.Zone = cfg.Zone
	src, closeSrc, err := readSource(ctx, cfg)
	if err != nil {
		return api.Published{}, err
	}
	defer closeSrc()
	deps, closeDeps, err := serveDeps(ctx, cfg)
	if err != nil {
		return api.Published{}, err
	}
	defer closeDeps()
	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()
	return read.Publish(ctx, src, deps, q)
}

// readSource opens what a local read is taken from: the ledger read-only, and the
// checkpoint log and artifacts so the read can start from the last mint run. The
// returned func closes them.
func readSource(ctx context.Context, cfg config.Config) (read.Source, func(), error) {
	l, err := openLedgerReadOnly(ctx, cfg)
	if err != nil {
		return read.Source{}, nil, err
	}
	buckets, err := cfg.Buckets()
	if err != nil {
		_ = l.Close()
		return read.Source{}, nil, err
	}
	db, err := ops.Open(ctx, cfg.DBPath, buckets)
	if err != nil {
		_ = l.Close()
		return read.Source{}, nil, err
	}
	store, err := artifact.New(cfg.ArtifactDir)
	if err != nil {
		_ = db.Close()
		_ = l.Close()
		return read.Source{}, nil, err
	}
	closeAll := func() { _ = store.Close(); _ = db.Close(); _ = l.Close() }
	defs, err := registryOf(ctx, db)
	if err != nil {
		closeAll()
		return read.Source{}, nil, err
	}
	return read.Source{Ledger: l, Checkpoints: db, Artifacts: store, Definitions: defs}, closeAll, nil
}

// timelineHeader describes the read: which watermark, which lens, and the hash that
// identifies what is being shown. When a projection is in force the served hash is
// the one a receipt would carry, so the viewer and the publisher can be checked
// against each other by eye.
func timelineHeader(t api.Timeline) string {
	h := fmt.Sprintf("W=%d  live=%d", t.W, t.Live)
	if t.Projection != nil {
		h += fmt.Sprintf("  served=%d", t.Projection.Served)
	}
	h += fmt.Sprintf("  shown=%d\n", t.Shown)

	if t.AsOf != "" {
		h += fmt.Sprintf("as-of      %s (anam-time)\n", t.AsOf)
	}
	if t.At != "" {
		h += fmt.Sprintf("at         %s (fact-time, snapshot: latest per series)  %s\n", t.At, t.AtHash)
	}
	h += fmt.Sprintf("canonical  %s\n", t.Canonical)
	if p := t.Projection; p != nil {
		h += fmt.Sprintf("projection %s v%d  %s\n", p.Name, p.Version, p.Hash)
	}
	return h
}

// showIn renders a stored fact-time in the reader's zone, at the wire format's full
// precision so nothing is rounded away. The stored string is what Anam holds; this
// is only where the reader is standing when they look at it.
func showIn(loc *time.Location, stored string) string {
	t, err := clock.Parse(stored)
	if err != nil {
		return stored
	}
	return t.In(loc).Format(clock.Layout)
}

func writeTimeline(cmd *cobra.Command, t api.Timeline, loc *time.Location, asJSON bool) error {
	header := timelineHeader(t)

	if asJSON {
		// Entries to stdout, bookkeeping to stderr, so the output pipes clean.
		_, _ = fmt.Fprint(cmd.ErrOrStderr(), header)
		out, err := json.Marshal(t.Entries)
		if err != nil {
			return fmt.Errorf("timeline: marshal entries: %w", err)
		}
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s\n", out)
		return err
	}

	_, _ = fmt.Fprint(cmd.OutOrStdout(), header)
	w := tabwriter.NewWriter(cmd.OutOrStdout(), 2, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "FACT TIME\tTYPE\tV\tSTATUS\tSOURCE REF\tID")
	for _, e := range t.Entries {
		_, _ = fmt.Fprintf(w, "%s\t%s\tv%d\t%s\t%s\t%s\n",
			showIn(loc, e.FactTime), e.Type, t.Versions[e.FacetID], e.Status, e.SourceRef, e.FacetID)
	}
	return w.Flush()
}
