package app

import (
	"context"
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/mattjoyce/anam/internal/catchup"
	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/store/ledger"
	"github.com/mattjoyce/anam/internal/store/ops"
)

func changesCmd(cfgPath *string) *cobra.Command {
	var since, until string

	cmd := &cobra.Command{
		Use:   "changes --since <instant>",
		Short: "what Anam learned between two anam-times",
		Long: `Changes reads anam-events by anam-time (§9): what was admitted, superseded
or tombstoned after a given instant — Anam's own clock, not the events' fact-time.

The query runs over the operational store's derived index, which is first caught
up to the ledger's head by replay, so the answer is as fresh as ingest.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			sinceT, _, err := parseInstant(since)
			if err != nil {
				return fmt.Errorf("--since: %w", err)
			}
			untilS := ""
			if until != "" {
				t, dateOnly, err := parseInstant(until)
				if err != nil {
					return fmt.Errorf("--until: %w", err)
				}
				if dateOnly {
					t = t.AddDate(0, 0, 1)
				}
				untilS = clock.Format(t)
			}

			cfg, err := loadConfig(*cfgPath, cmd)
			if err != nil {
				return err
			}
			l, err := ledger.New(cmd.Context(), cfg.DDBEndpoint, cfg.DDBRegion, cfg.TableName, cfg.PersonID)
			if err != nil {
				return err
			}
			buckets, err := cfg.Buckets()
			if err != nil {
				return err
			}
			db, err := ops.Open(cmd.Context(), cfg.DBPath, buckets)
			if err != nil {
				return err
			}
			defer func() { _ = db.Close() }()

			ctx, cancel := context.WithTimeout(cmd.Context(), cfg.Timeout)
			defer cancel()
			sync, err := catchup.Sync(ctx, l, db, buckets)
			if err != nil {
				return err
			}

			changes, err := db.ChangesSince(ctx, clock.Format(sinceT), untilS)
			if err != nil {
				return err
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "index at seq %d (caught up from %d)  ·  %d change(s) since %s\n",
				sync.To, sync.From, len(changes), clock.Format(sinceT))
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 2, 0, 2, ' ', 0)
			_, _ = fmt.Fprintln(w, "SEQ\tANAM TIME\tKIND\tV\tFACET")
			for _, c := range changes {
				_, _ = fmt.Fprintf(w, "%d\t%s\t%s\tv%d\t%s\n", c.Seq, c.AnamTime, c.Kind, c.ArtifactVersion, c.FacetID)
			}
			return w.Flush()
		},
	}
	cmd.Flags().StringVar(&since, "since", "", "report anam-events after this instant (2006-01-02 or RFC 3339; required)")
	cmd.Flags().StringVar(&until, "until", "", "stop at this instant, inclusive; a bare date includes its whole day")
	_ = cmd.MarkFlagRequired("since")
	return cmd
}
