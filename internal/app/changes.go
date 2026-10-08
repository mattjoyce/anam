package app

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/mattjoyce/anam/internal/api"
	"github.com/mattjoyce/anam/internal/catchup"
	"github.com/mattjoyce/anam/internal/config"
	"github.com/mattjoyce/anam/internal/read"
	"github.com/mattjoyce/anam/internal/store/ops"
)

func changesCmd(cfgPath *string) *cobra.Command {
	var since, until string

	cmd := &cobra.Command{
		Use:   "changes --since <instant>",
		Short: "what Anam learned between two anam-times",
		Long: `Changes reads anam-events by anam-time (§9): what was admitted, superseded
or tombstoned after a given instant — Anam's own clock, not the events' fact-time.

The query runs over the operational store's derived index. Read locally, the index
is first caught up to the ledger's head by replay. The daemon catches it up after
every ingest, so its answer is as fresh as the last one; while an ingest runs the
header says how far the index is behind W.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := loadRoutedConfig(*cfgPath, cmd)
			if err != nil {
				return err
			}
			c, err := readChanges(cmd.Context(), cfg, api.ChangesQuery{Since: since, Until: until}, cmd.ErrOrStderr())
			if err != nil {
				return err
			}
			return writeChanges(cmd, c)
		},
	}
	cmd.Flags().StringVar(&since, "since", "", "report anam-events after this instant (2006-01-02 or RFC 3339; required)")
	cmd.Flags().StringVar(&until, "until", "", "stop at this instant, inclusive; a bare date includes its whole day")
	_ = cmd.MarkFlagRequired("since")
	return cmd
}

// readChanges asks the daemon when a server is configured. Locally it catches the
// index up to the ledger's head first, saying so on stderr: this process is the only
// one writing it.
func readChanges(ctx context.Context, cfg config.Config, q api.ChangesQuery, stderr io.Writer) (api.Changes, error) {
	if cfg.Server != "" {
		zone, err := cfg.ZoneName()
		if err != nil {
			return api.Changes{}, err
		}
		q.Zone = zone
		c, err := newClient(cfg)
		if err != nil {
			return api.Changes{}, err
		}
		return c.Changes(ctx, q)
	}
	q.Zone = cfg.Zone

	l, err := openLedgerReadOnly(ctx, cfg)
	if err != nil {
		return api.Changes{}, err
	}
	defer func() { _ = l.Close() }()
	buckets, err := cfg.Buckets()
	if err != nil {
		return api.Changes{}, err
	}
	db, err := ops.Open(ctx, cfg.DBPath, buckets)
	if err != nil {
		return api.Changes{}, err
	}
	defer func() { _ = db.Close() }()

	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()
	sync, err := catchup.Sync(ctx, l, db, buckets)
	if err != nil {
		return api.Changes{}, err
	}
	if sync.To != sync.From {
		_, _ = fmt.Fprintf(stderr, "caught the index up from seq %d to %d\n", sync.From, sync.To)
	}
	return read.Changes(ctx, db, sync.To, q)
}

func writeChanges(cmd *cobra.Command, c api.Changes) error {
	out := cmd.OutOrStdout()
	_, _ = fmt.Fprintf(out, "index at seq %d  ·  %d change(s) since %s\n", c.IndexSeq, len(c.Changes), c.Since)
	if c.W > c.IndexSeq {
		_, _ = fmt.Fprintf(out, "the ledger is at W=%d: the index is behind, and its newest changes are not shown\n", c.W)
	}
	w := tabwriter.NewWriter(out, 2, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "SEQ\tANAM TIME\tKIND\tV\tFACET")
	for _, ch := range c.Changes {
		_, _ = fmt.Fprintf(w, "%d\t%s\t%s\tv%d\t%s\n", ch.Seq, ch.AnamTime, ch.Kind, ch.ArtifactVersion, ch.FacetID)
	}
	return w.Flush()
}
