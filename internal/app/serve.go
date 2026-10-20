package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/mattjoyce/anam/internal/api"
	"github.com/mattjoyce/anam/internal/config"
	"github.com/mattjoyce/anam/internal/projection"
	"github.com/mattjoyce/anam/internal/read"
	"github.com/mattjoyce/anam/internal/receipt"
	"github.com/mattjoyce/anam/internal/serve"
	"github.com/mattjoyce/anam/internal/store/artifact"
	"github.com/mattjoyce/anam/internal/store/ops"
)

func serveCmd(cfgPath *string) *cobra.Command {
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "serve [projection]",
		Short: "publish a projection of the canonical, with a receipt",
		Long: `Serve prunes the whole-person canonical through a projection template and
publishes the result: the served value and its receipt, each written to the
artifact directory under the SHA-256 of its own bytes.

A projection prunes and nothing else — it never reshapes a facet, never generates
one, and filters only on metadata. You select a template by name; a consumer never
defines one. Templates are bundled with the binary or registered by the operator
(anam projection register). Run with no name to see what is available.

Serving writes nothing to the ledger. Being shown is not something Anam learned
about a fact, so no anam-event is appended and the watermark does not move; the
receipt is the record of the serve.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig(*cfgPath, cmd)
			if err != nil {
				return err
			}
			if len(args) == 0 {
				templates, err := localProjections(cmd.Context(), cfg)
				if err != nil {
					return err
				}
				return listProjections(cmd, templates)
			}
			deps, closeLedger, err := serveDeps(cmd.Context(), cfg)
			if err != nil {
				return err
			}
			defer closeLedger()

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
	cmd.AddCommand(receiptListCmd(cfgPath), receiptShowCmd(cfgPath), receiptVerifyCmd(cfgPath))
	return cmd
}

func receiptListCmd(cfgPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "list the receipts in the artifact directory, newest first",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := loadRoutedConfig(*cfgPath, cmd)
			if err != nil {
				return err
			}
			var list api.Receipts
			if cfg.Server != "" {
				c, err := newClient(cfg)
				if err != nil {
					return err
				}
				list, err = c.Receipts(cmd.Context())
				if err != nil {
					return err
				}
			} else {
				// Listing reads the artifact directory and nothing else, so it still
				// answers when there is no ledger to open.
				store, err := artifact.New(cfg.ArtifactDir)
				if err != nil {
					return err
				}
				if list, err = read.Receipts(serve.Deps{Artifacts: store}, cfg.ArtifactDir); err != nil {
					return err
				}
			}
			return writeReceipts(cmd, list)
		},
	}
}

func writeReceipts(cmd *cobra.Command, list api.Receipts) error {
	if len(list.Receipts) == 0 {
		_, err := fmt.Fprintf(cmd.OutOrStdout(), "no receipts in %s\n", list.Dir)
		return err
	}
	w := tabwriter.NewWriter(cmd.OutOrStdout(), 2, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "SERVED AT\tPROJECTION\tW\tFACETS\tRECEIPT")
	for _, l := range list.Receipts {
		r := l.Receipt
		_, _ = fmt.Fprintf(w, "%s\t%s v%d\t%d\t%d\t%s\n",
			r.ServedAt, r.Template, r.TemplateVersion, r.Watermark, len(r.FacetIDs), l.Hash)
	}
	return w.Flush()
}

func receiptShowCmd(cfgPath *string) *cobra.Command {
	var (
		asJSON bool
		cursor string
	)

	cmd := &cobra.Command{
		Use:   "show <receipt-hash>",
		Short: "render what a past serve actually published, from the artifact alone",
		Long: `Show reads the receipt and the served value out of the artifact directory and
prints them. It consults no ledger and re-derives nothing.

That restraint is the point. ` + "`receipt verify`" + ` asks whether the ledger still honours
a receipt; show asks what the artifact itself says — the question you have to be
able to answer when the ledger is gone, unreachable, or exactly the thing you
doubt. Both files are checked against their own content addresses on the way out,
so a tampered artifact is refused rather than displayed as if it were genuine.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadRoutedConfig(*cfgPath, cmd)
			if err != nil {
				return err
			}
			var shown api.ReceiptShown
			if cfg.Server != "" {
				c, err := newClient(cfg)
				if err != nil {
					return err
				}
				if cursor != "" {
					if shown, err = c.ReceiptPage(cmd.Context(), args[0], cursor); err != nil {
						return err
					}
					writePage(cmd, shown.Page)
				} else if shown, err = c.Receipt(cmd.Context(), args[0]); err != nil {
					return err
				}
			} else {
				if cursor != "" {
					return errors.New("--cursor pages a receipt over the daemon; a local show reads the whole value")
				}
				store, err := artifact.New(cfg.ArtifactDir)
				if err != nil {
					return err
				}
				if shown, err = read.Receipt(serve.Deps{Artifacts: store}, args[0]); err != nil {
					return err
				}
			}
			return writeReceipt(cmd, shown, asJSON)
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit the served entries as JSON, exactly as published")
	cmd.Flags().StringVar(&cursor, "cursor", "",
		"show only the page this next_cursor names (over the daemon); without it every page is read and shown whole")
	return cmd
}

// writePage says on stderr which portion of a receipt one page is, and the cursor
// to the next, so stdout stays the page alone.
func writePage(cmd *cobra.Command, p *api.Page) {
	if p == nil {
		return
	}
	_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "page: facets %d to %d of %d\n", p.Offset+1, p.Offset+p.ReturnedCount, p.TotalCount)
	if p.NextCursor != "" {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "next cursor: %s\n", p.NextCursor)
	}
}

func writeReceipt(cmd *cobra.Command, shown api.ReceiptShown, asJSON bool) error {
	out := cmd.OutOrStdout()
	if shown.Receipt == nil {
		// A page asked for by cursor: the receipt rode the first page.
		return writeServed(out, shown, asJSON)
	}
	r := *shown.Receipt
	if asJSON {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "%s v%d at W=%d, served %s\n",
			r.Template, r.TemplateVersion, r.Watermark, r.ServedAt)
		return writeServed(out, shown, true)
	}

	_, _ = fmt.Fprintf(out, "%s v%d at W=%d, served %s\n", r.Template, r.TemplateVersion, r.Watermark, r.ServedAt)
	_, _ = fmt.Fprintf(out, "  %d facets, span %s\n", len(r.FacetIDs), r.Span)
	_, _ = fmt.Fprintf(out, "  served     %s  [intact]\n", r.ServedHash)
	_, _ = fmt.Fprintf(out, "  definitions %s\n", r.DefinitionSetVersion)
	_, _ = fmt.Fprintf(out, "  ledger     %s\n", ledgerOrUnnamed(r.LedgerID))
	if r.Query != nil {
		_, _ = fmt.Fprintf(out, "  query      %s\n", queryLine(*r.Query))
	}
	if p := r.Program; p != nil {
		_, _ = fmt.Fprintf(out, "  program    %s answered with %s v%d %s\n%s\n", p.Relation, p.RuleSet, p.RuleSetVersion, p.RuleSetHash, indent(p.Text))
	}
	_, _ = fmt.Fprintln(out)
	return writeServed(out, shown, false)
}

// writeServed renders the served value a receipt names, or one page of it: as JSON,
// exactly as published, or as a table.
func writeServed(out io.Writer, shown api.ReceiptShown, asJSON bool) error {
	if asJSON {
		if shown.Answer != nil {
			_, err := fmt.Fprintf(out, "%s\n", shown.Answer.Bytes())
			return err
		}
		if shown.Frame != nil {
			_, err := fmt.Fprintf(out, "%s\n", shown.Frame.Bytes())
			return err
		}
		b, err := json.Marshal(shown.Entries)
		if err != nil {
			return fmt.Errorf("receipt show: marshal: %w", err)
		}
		_, err = fmt.Fprintf(out, "%s\n", b)
		return err
	}
	if shown.Answer != nil {
		return writeAnswer(out, *shown.Answer)
	}
	if shown.Frame != nil {
		// The served frame holds fact times as stored; show renders from the
		// artifact alone and names no zone, so they stay as served.
		return writeFrame(out, shown.Frame, nil)
	}

	w := tabwriter.NewWriter(out, 2, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "FACT TIME\tTYPE\tSTATUS\tPAYLOAD")
	for _, e := range shown.Entries {
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", e.FactTime, e.Type, e.Status, clipPayload(e.Payload))
	}
	return w.Flush()
}

// clipPayload keeps a payload on one terminal line. The whole value is in the served
// file; this is the viewer, and --json prints it untouched.
func clipPayload(b []byte) string {
	const max = 76
	s := string(b)
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "\u2026"
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
			cfg, err := loadRoutedConfig(*cfgPath, cmd)
			if err != nil {
				return err
			}
			v, err := verifyReceipt(cmd.Context(), cfg, args[0])
			if err != nil {
				return err
			}
			return writeVerified(cmd, v)
		},
	}
}

func verifyReceipt(ctx context.Context, cfg config.Config, hash string) (api.ReceiptVerified, error) {
	if cfg.Server != "" {
		c, err := newClient(cfg)
		if err != nil {
			return api.ReceiptVerified{}, err
		}
		return c.VerifyReceipt(ctx, hash)
	}
	deps, closeLedger, err := serveDeps(ctx, cfg)
	if err != nil {
		return api.ReceiptVerified{}, err
	}
	defer closeLedger()
	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()
	return read.VerifyReceipt(ctx, deps, hash)
}

func writeVerified(cmd *cobra.Command, v api.ReceiptVerified) error {
	r, res := v.Receipt, v.Result
	w := cmd.OutOrStdout()
	_, _ = fmt.Fprintf(w, "receipt %s\n", v.Hash)
	_, _ = fmt.Fprintf(w, "  served %s v%d at W=%d on %s — %d facets, span %s, ledger %s\n",
		r.Template, r.TemplateVersion, r.Watermark, r.ServedAt, len(r.FacetIDs), r.Span, ledgerOrUnnamed(r.LedgerID))
	if r.Query != nil {
		_, _ = fmt.Fprintf(w, "  narrowed by %s\n", queryLine(*r.Query))
	}
	if p := r.Program; p != nil {
		_, _ = fmt.Fprintf(w, "  answered %s with %s v%d %s\n", p.Relation, p.RuleSet, p.RuleSetVersion, p.RuleSetHash)
	}
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprint(w, res.Summary())
	if !res.OK() {
		return fmt.Errorf("receipt %s did not verify", v.Hash)
	}
	return nil
}

// listProjections prints the templates held, newest version of each. A consumer
// selects one; this is the menu, and the operator adds to it with anam projection
// register (card 125).
func listProjections(cmd *cobra.Command, templates *projection.Registry) error {
	w := tabwriter.NewWriter(cmd.OutOrStdout(), 2, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "PROJECTION\tV\tSHAPE\tTYPES\tSTATUSES\tMAX CLASS\tSPAN")
	for _, t := range templates.All() {
		_, _ = fmt.Fprintf(w, "%s\t%d\t%s\t%s\t%s\t%s\t%s\n",
			t.Name, t.Version, t.Shape, joinTypes(t), joinStatuses(t), t.MaxClassification, t.Span())
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

// queryLine renders the narrowing a receipt records, in its own resolved instants.
func queryLine(q receipt.Query) string {
	var parts []string
	for _, p := range []struct{ name, val string }{
		{"at", q.At}, {"from", q.From}, {"before", q.Before}, {"type", q.Type},
	} {
		if p.val != "" {
			parts = append(parts, p.name+" "+p.val)
		}
	}
	if len(q.Fields) > 0 {
		parts = append(parts, "fields "+strings.Join(q.Fields, " "))
	}
	return strings.Join(parts, ", ")
}

// ledgerOrUnnamed renders a receipt's ledger id, or says a receipt predates them.
func ledgerOrUnnamed(id string) string {
	if id == "" {
		return "(unnamed: served before ledger ids were recorded)"
	}
	return id
}

// serveDeps assembles the serve path's dependencies from config (read.ServeDeps):
// the ledger read-only, since serving writes nothing to it, the artifacts, and the
// checkpoint log so a serve or a verify starts from the last mint run rather than
// the ledger's first event. The returned func closes them.
func serveDeps(ctx context.Context, cfg config.Config) (serve.Deps, func(), error) {
	l, err := openLedgerReadOnly(ctx, cfg)
	if err != nil {
		return serve.Deps{}, nil, err
	}
	buckets, err := cfg.Buckets()
	if err != nil {
		_ = l.Close()
		return serve.Deps{}, nil, err
	}
	db, err := ops.Open(ctx, cfg.DBPath, buckets)
	if err != nil {
		_ = l.Close()
		return serve.Deps{}, nil, err
	}
	store, err := artifact.New(cfg.ArtifactDir)
	if err != nil {
		_ = db.Close()
		_ = l.Close()
		return serve.Deps{}, nil, err
	}
	closeAll := func() { _ = store.Close(); _ = db.Close(); _ = l.Close() }
	defs, err := registryOf(ctx, db)
	if err != nil {
		closeAll()
		return serve.Deps{}, nil, err
	}
	templates, err := projectionsOf(ctx, db)
	if err != nil {
		closeAll()
		return serve.Deps{}, nil, err
	}
	deps, err := read.ServeDeps(l, store, db, defs, templates)
	if err != nil {
		closeAll()
		return serve.Deps{}, nil, err
	}
	return deps, closeAll, nil
}
