package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/mattjoyce/anam/internal/api"
	"github.com/mattjoyce/anam/internal/config"
	"github.com/mattjoyce/anam/internal/read"
)

func coverageCmd(cfgPath *string) *cobra.Command {
	var q api.CoverageQuery
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "coverage",
		Short: "what the record holds: each type's count, fact-time range and sources at a W",
		Long: `Coverage counts the live facets of each type at the head, or at --w or --as-of,
with the earliest and latest fact-time, each source's share and the projections that
serve the type ("none" is a type the query route cannot reach). With --projection
it counts what that projection serves. The same W gives the same answer.

Coverage says what is recorded, never that a source is complete: a quiet source
may be current, and a gap in fact-time may be a gap in the world.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := loadRoutedConfig(*cfgPath, cmd)
			if err != nil {
				return err
			}
			c, err := readCoverage(cmd.Context(), cfg, q)
			if err != nil {
				return err
			}
			if asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(c)
			}
			return writeCoverage(cmd, c)
		},
	}
	cmd.Flags().Int64Var(&q.W, "w", 0, "count at this watermark (a receipt's W) instead of the head; excludes --as-of")
	cmd.Flags().StringVar(&q.AsOf, "as-of", "", "count what Anam knew at this anam-time (2006-01-02 or RFC 3339)")
	cmd.Flags().StringVar(&q.Projection, "projection", "", "count what a projection serves (see: anam projection list)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit the coverage as JSON, as the daemon answers it")
	return cmd
}

// readCoverage asks the daemon when a server is configured, and otherwise counts over
// the local files, opened read-only.
func readCoverage(ctx context.Context, cfg config.Config, q api.CoverageQuery) (api.Coverage, error) {
	if cfg.Server != "" {
		zone, err := cfg.ZoneName()
		if err != nil {
			return api.Coverage{}, err
		}
		q.Zone = zone
		c, err := newClient(cfg)
		if err != nil {
			return api.Coverage{}, err
		}
		return c.Coverage(ctx, q)
	}
	q.Zone = cfg.Zone
	cat, err := loadCatalog(ctx, cfg)
	if err != nil {
		return api.Coverage{}, err
	}
	src, closeAll, err := readSource(ctx, cfg, cat)
	if err != nil {
		return api.Coverage{}, err
	}
	defer closeAll()
	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()
	return read.Coverage(ctx, src, q)
}

func writeCoverage(cmd *cobra.Command, c api.Coverage) error {
	out := cmd.OutOrStdout()
	_, _ = fmt.Fprintf(out, "ledger  %s\n", c.LedgerID)
	_, _ = fmt.Fprintf(out, "W       %d, %d live facets\n", c.W, c.Live)
	if p := c.Projection; p != nil {
		_, _ = fmt.Fprintf(out, "through %s v%d\n", p.Name, p.Version)
	}
	if len(c.Types) == 0 {
		_, err := fmt.Fprintln(out, "nothing recorded")
		return err
	}
	_, _ = fmt.Fprintln(out)
	w := tabwriter.NewWriter(out, 2, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "TYPE\tCOUNT\tEARLIEST\tLATEST\tSOURCES\tPROJECTIONS")
	for _, t := range c.Types {
		names := make([]string, len(t.Sources))
		for i, s := range t.Sources {
			names[i] = fmt.Sprintf("%s (%d)", s.Source, s.Count)
		}
		served := strings.Join(t.Projections, ", ")
		if served == "" {
			served = "none"
		}
		_, _ = fmt.Fprintf(w, "%s\t%d\t%s\t%s\t%s\t%s\n", t.Type, t.Count, day(t.Earliest), day(t.Latest), strings.Join(names, ", "), served)
	}
	return w.Flush()
}

// day is a fact-time's UTC date, which is as fine as a coverage table needs.
func day(factTime string) string {
	d, _, _ := strings.Cut(factTime, "T")
	return d
}
