package app

import (
	"context"
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/config"
	"github.com/mattjoyce/anam/internal/consent"
	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/projection"
	"github.com/mattjoyce/anam/internal/serve"
	"github.com/mattjoyce/anam/internal/store/artifact"
	"github.com/mattjoyce/anam/internal/store/ledger"
)

func serveCmd(cfgPath *string) *cobra.Command {
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "serve [projection]",
		Short: "publish a projection of the canonical, with a receipt",
		Long: `Serve prunes the whole-person canonical through a bundled projection template
and publishes the result: the served value and its receipt, each written to the
artifact directory under the SHA-256 of its own bytes.

A projection prunes and nothing else — it never reshapes a facet, never generates
one, and filters only on metadata. You select a template by name; you cannot define
one. Run with no name to see what is available.

Serving writes nothing to the ledger. Being shown is not something Anam learned
about a fact, so no record-event is appended and the watermark does not move; the
receipt is the record of the serve.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			templates, err := projection.Bundled()
			if err != nil {
				return err
			}
			if len(args) == 0 {
				return listProjections(cmd, templates)
			}

			cfg, err := loadConfig(*cfgPath, cmd)
			if err != nil {
				return err
			}
			deps, err := serveDeps(cmd.Context(), cfg, templates)
			if err != nil {
				return err
			}

			ctx, cancel := context.WithTimeout(cmd.Context(), cfg.Timeout)
			defer cancel()
			out, err := serve.Run(ctx, deps, args[0], dryRun)
			if err != nil {
				return err
			}

			w := cmd.OutOrStdout()
			verb := "served"
			if !out.Written {
				verb = "derived (dry run — nothing written)"
			}
			_, _ = fmt.Fprintf(w, "%s %s v%d at W=%d\n", verb, out.Receipt.Template, out.Receipt.TemplateVersion, out.Receipt.Watermark)
			_, _ = fmt.Fprintf(w, "  canonical  %d facets  %s\n", out.CanonicalLen, out.CanonicalHash)
			_, _ = fmt.Fprintf(w, "  served     %d facets  %s\n", len(out.Receipt.FacetIDs), out.Receipt.ServedHash)
			_, _ = fmt.Fprintf(w, "  receipt    %s\n", out.ReceiptHash)
			_, _ = fmt.Fprintf(w, "  span       %s\n", out.Receipt.Span)
			return nil
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "derive and report, but publish nothing")
	return cmd
}

func receiptCmd(cfgPath *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "receipt",
		Short: "list and verify the receipts of past serves",
	}
	cmd.AddCommand(receiptListCmd(cfgPath), receiptVerifyCmd(cfgPath))
	return cmd
}

func receiptListCmd(cfgPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "list the receipts in the artifact directory, newest first",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := loadConfig(*cfgPath, cmd)
			if err != nil {
				return err
			}
			templates, err := projection.Bundled()
			if err != nil {
				return err
			}
			deps, err := serveDeps(cmd.Context(), cfg, templates)
			if err != nil {
				return err
			}

			receipts, hashes, err := serve.List(deps)
			if err != nil {
				return err
			}
			if len(receipts) == 0 {
				_, err := fmt.Fprintf(cmd.OutOrStdout(), "no receipts in %s\n", cfg.ArtifactDir)
				return err
			}

			w := tabwriter.NewWriter(cmd.OutOrStdout(), 2, 0, 2, ' ', 0)
			_, _ = fmt.Fprintln(w, "SERVED AT\tPROJECTION\tW\tFACETS\tRECEIPT")
			for i, r := range receipts {
				_, _ = fmt.Fprintf(w, "%s\t%s v%d\t%d\t%d\t%s\n",
					r.ServedAt, r.Template, r.TemplateVersion, r.Watermark, len(r.FacetIDs), hashes[i])
			}
			return w.Flush()
		},
	}
}

func receiptVerifyCmd(cfgPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "verify <receipt-hash>",
		Short: "prove a receipt: re-derive it from the ledger and re-hash the artifact",
		Long: `Verify honours a receipt with two independent proofs.

  reproduced  the ledger, folded at the receipt's own watermark and pruned through
              the same template version, still produces exactly those bytes.
  intact      the served artifact on disk still hashes to the address it is filed
              under.

Either can fail without the other: a tampered file under a sound ledger, or a
corrupted ledger under an untouched file. Both must hold for the serve to be proved.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig(*cfgPath, cmd)
			if err != nil {
				return err
			}
			templates, err := projection.Bundled()
			if err != nil {
				return err
			}
			deps, err := serveDeps(cmd.Context(), cfg, templates)
			if err != nil {
				return err
			}

			ctx, cancel := context.WithTimeout(cmd.Context(), cfg.Timeout)
			defer cancel()
			r, res, err := serve.Verify(ctx, deps, args[0])
			if err != nil {
				return err
			}

			w := cmd.OutOrStdout()
			_, _ = fmt.Fprintf(w, "receipt %s\n", args[0])
			_, _ = fmt.Fprintf(w, "  served %s v%d at W=%d on %s — %d facets, span %s\n\n",
				r.Template, r.TemplateVersion, r.Watermark, r.ServedAt, len(r.FacetIDs), r.Span)
			_, _ = fmt.Fprint(w, res.Summary())
			if !res.OK() {
				return fmt.Errorf("receipt %s did not verify", args[0])
			}
			return nil
		},
	}
}

// listProjections prints the bundled templates. A consumer selects one; this is the
// menu, and there is no way to add to it at runtime (§9).
func listProjections(cmd *cobra.Command, templates *projection.Registry) error {
	w := tabwriter.NewWriter(cmd.OutOrStdout(), 2, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "PROJECTION\tV\tTYPES\tSTATUSES\tMAX CLASS\tSPAN")
	for _, t := range templates.All() {
		_, _ = fmt.Fprintf(w, "%s\t%d\t%s\t%s\t%s\t%s\n",
			t.Name, t.Version, joinTypes(t), joinStatuses(t), t.MaxClassification, t.Span())
	}
	if err := w.Flush(); err != nil {
		return err
	}
	_, err := fmt.Fprintf(cmd.ErrOrStderr(), "\nselect one: anam serve <projection>\n")
	return err
}

func joinTypes(t projection.Template) string {
	out := ""
	for i, ft := range t.Types {
		if i > 0 {
			out += ","
		}
		out += string(ft)
	}
	if out == "" {
		return "(none)"
	}
	return out
}

func joinStatuses(t projection.Template) string {
	out := ""
	for i, s := range t.Statuses {
		if i > 0 {
			out += ","
		}
		out += string(s)
	}
	if out == "" {
		return "(none)"
	}
	return out
}

// serveDeps assembles the serve path's dependencies from config. The consent gate is
// the pass-through one (§10) — it is wired in on the real path rather than reserved
// for later, because a gate threaded through afterwards is a gate a caller can forget.
func serveDeps(ctx context.Context, cfg config.Config, templates *projection.Registry) (serve.Deps, error) {
	l, err := ledger.New(ctx, cfg.DDBEndpoint, cfg.DDBRegion, cfg.TableName, cfg.PersonID)
	if err != nil {
		return serve.Deps{}, err
	}
	store, err := artifact.New(cfg.ArtifactDir)
	if err != nil {
		return serve.Deps{}, err
	}
	defSet, err := def.SetVersion()
	if err != nil {
		return serve.Deps{}, err
	}
	return serve.Deps{
		Ledger:               l,
		Templates:            templates,
		Artifacts:            store,
		Gate:                 consent.Open{},
		Clock:                clock.Real{},
		DefinitionSetVersion: defSet,
	}, nil
}
