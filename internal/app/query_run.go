package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/mattjoyce/anam/internal/api"
	"github.com/mattjoyce/anam/internal/config"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/query"
	"github.com/mattjoyce/anam/internal/read"
	"github.com/mattjoyce/anam/internal/safepath"
	"github.com/mattjoyce/anam/internal/store/ops"
)

func queryRunCmd(cfgPath *string) *cobra.Command {
	var projName, relation, text, file, asOf string
	var w int64
	var dryRun, asJSON bool
	cmd := &cobra.Command{
		Use:   "run",
		Short: "run a Datalog program through a projection, answered with a receipt",
		Long: `Run evaluates a program over what --projection serves at the head (or at --as-of
or --w), built on Anam's bundled rules, and answers with one relation: --relation,
or the head of the program's last rule. The answer is filed with a receipt naming
the projection, the rule set and the program, so anam receipt verify re-runs it;
--dry-run derives the receipt and files nothing. anam query describe says what can
be asked.

  anam query run --projection everything -e '
    instant(At) :- entry(_, "health.blood-pressure", At, _, _, _, _).
    regimen(At, Drug) :- entry(_, "health.blood-pressure", At, _, _, _, _), current(E, At),
                         entry(E, "health.medication.started", _, _, _, _, _), attr(E, "/drug", Drug).'

The program is -e, -f, or stdin.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := loadRoutedConfig(*cfgPath, cmd)
			if err != nil {
				return err
			}
			program, err := programText(cmd, text, file)
			if err != nil {
				return err
			}
			if relation == "" {
				if relation, err = lastHead(program); err != nil {
					return err
				}
			}
			q := api.QueryRequest{Projection: api.Named(projName), Text: program, Relation: relation, AsOf: asOf, W: w, DryRun: dryRun}
			a, err := runQuery(cmd.Context(), cfg, q)
			if err != nil {
				return err
			}
			return writeQueryAnswer(cmd, a, asJSON)
		},
	}
	cmd.Flags().StringVar(&projName, "projection", "", "the template the query runs through (required)")
	cmd.Flags().StringVar(&relation, "relation", "", "the relation to answer (default: the last rule's head)")
	cmd.Flags().StringVarP(&text, "text", "e", "", "the program")
	cmd.Flags().StringVarP(&file, "file", "f", "", "a file holding the program")
	cmd.Flags().StringVar(&asOf, "as-of", "", "read what Anam knew at this anam-time")
	cmd.Flags().Int64Var(&w, "w", 0, "read at this watermark")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "derive the answer and its receipt, file nothing")
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit the answer as JSON, receipt included")
	return cmd
}

// programText is the program from -e, -f or stdin, exactly one of them.
func programText(cmd *cobra.Command, text, file string) (string, error) {
	switch {
	case text != "" && file != "":
		return "", errors.New("give the program as -e or -f, not both")
	case text != "":
		return text, nil
	case file != "":
		fh, err := safepath.Open(file)
		if err != nil {
			return "", err
		}
		defer func() { _ = fh.Close() }()
		b, err := io.ReadAll(fh)
		if err != nil {
			return "", err
		}
		return string(b), nil
	}
	b, err := io.ReadAll(cmd.InOrStdin())
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(string(b)) == "" {
		return "", errors.New("no program: give it as -e, -f or on stdin")
	}
	return string(b), nil
}

// lastHead is the relation the program's last rule defines, the answer when none is
// named.
func lastHead(text string) (string, error) {
	p, err := query.Parse(text)
	if err != nil {
		return "", err
	}
	if len(p.Rules) == 0 {
		return "", errors.New("the program has no rules; name --relation")
	}
	return p.Rules[len(p.Rules)-1].Head.Name, nil
}

func runQuery(ctx context.Context, cfg config.Config, q api.QueryRequest) (api.QueryAnswer, error) {
	if cfg.Server != "" {
		zone, err := cfg.ZoneName()
		if err != nil {
			return api.QueryAnswer{}, err
		}
		q.Zone = zone
		c, err := newClient(cfg)
		if err != nil {
			return api.QueryAnswer{}, err
		}
		return c.Query(ctx, q)
	}
	q.Zone = cfg.Zone
	src, closeSrc, err := readSource(ctx, cfg)
	if err != nil {
		return api.QueryAnswer{}, err
	}
	defer closeSrc()
	deps, closeDeps, err := serveDeps(ctx, cfg)
	if err != nil {
		return api.QueryAnswer{}, err
	}
	defer closeDeps()
	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()
	return read.Query(ctx, src, deps, q, 0)
}

func writeQueryAnswer(cmd *cobra.Command, a api.QueryAnswer, asJSON bool) error {
	out := cmd.OutOrStdout()
	if asJSON {
		b, err := json.MarshalIndent(a, "", "  ")
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(out, "%s\n", b)
		return err
	}
	_, _ = fmt.Fprintf(out, "W=%d  live=%d  served=%d  canonical %s\n", a.W, a.Live, a.Projection.Served, a.Canonical)
	if a.AsOf != "" {
		_, _ = fmt.Fprintf(out, "as-of      %s (anam-time)\n", a.AsOf)
	}
	_, _ = fmt.Fprintf(out, "projection %s v%d  %s\n", a.Projection.Name, a.Projection.Version, a.Projection.Hash)
	_, _ = fmt.Fprintf(out, "rules      %s v%d  %s\n", a.Rules.Name, a.Rules.Version, a.Rules.Hash)
	filed := "filed"
	if !a.Written {
		filed = "dry run, not filed"
	}
	_, _ = fmt.Fprintf(out, "receipt    %s  (%s)\n", a.ReceiptHash, filed)
	for _, w := range a.Warnings {
		_, _ = fmt.Fprintf(out, "warning    %s\n", w)
	}
	_, _ = fmt.Fprintln(out)
	return writeAnswer(out, a.Answer)
}

// writeAnswer renders one relation's tuples, a row each, with the answer's hash.
func writeAnswer(out io.Writer, a query.Answer) error {
	_, _ = fmt.Fprintf(out, "%s  %d tuples  %s\n", a.Relation, a.Len(), a.Hash())
	w := tabwriter.NewWriter(out, 2, 0, 2, ' ', 0)
	for _, t := range a.Tuples {
		cells := make([]string, len(t))
		for i, c := range t {
			cells[i] = c.Text()
		}
		_, _ = fmt.Fprintln(w, strings.Join(cells, "\t"))
	}
	return w.Flush()
}

func indent(text string) string {
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	for i, l := range lines {
		lines[i] = "    " + l
	}
	return strings.Join(lines, "\n")
}

func queryDescribeCmd(cfgPath *string) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "describe",
		Short: "what a query can be written against: relations, attributes per type and version, the rules",
		Long: `Describe is the introspection an agent reads before writing a program: the base
relations and the instant parameter, the exact fact-time lexeme, every type's
payload pointers at every Definition version held, and the bundled rule set with
its hash and its text. It reads no facet.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := loadRoutedConfig(*cfgPath, cmd)
			if err != nil {
				return err
			}
			cat, err := describeQuery(cmd.Context(), cfg)
			if err != nil {
				return err
			}
			return writeCatalogue(cmd, cat, asJSON)
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit the catalogue as JSON")
	return cmd
}

func describeQuery(ctx context.Context, cfg config.Config) (api.QueryCatalogue, error) {
	if cfg.Server != "" {
		c, err := newClient(cfg)
		if err != nil {
			return api.QueryCatalogue{}, err
		}
		return c.QueryCatalogue(ctx)
	}
	src, closeSrc, err := readSource(ctx, cfg)
	if err != nil {
		return api.QueryCatalogue{}, err
	}
	defer closeSrc()
	held, err := localHeld(ctx, cfg)
	if err != nil {
		return api.QueryCatalogue{}, err
	}
	src.Held = held
	return read.Describe(src)
}

// localHeld is every Definition body the local files hold: the bundled manifests and
// their history, and the registered ones when there is an operational store.
func localHeld(ctx context.Context, cfg config.Config) (query.Held, error) {
	held, err := query.HeldBundled()
	if err != nil {
		return nil, err
	}
	buckets, err := cfg.Buckets()
	if err != nil {
		return nil, err
	}
	db, err := ops.Open(ctx, cfg.DBPath, buckets)
	if err != nil {
		return nil, err
	}
	defer func() { _ = db.Close() }()
	rows, err := db.RegisteredDefinitions(ctx)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		t := facet.Type(r.Type)
		if held[t] == nil {
			held[t] = map[int][]byte{}
		}
		held[t][r.Version] = []byte(r.Body)
	}
	return held, nil
}

func writeCatalogue(cmd *cobra.Command, cat api.QueryCatalogue, asJSON bool) error {
	out := cmd.OutOrStdout()
	if asJSON {
		b, err := json.MarshalIndent(cat, "", "  ")
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(out, "%s\n", b)
		return err
	}
	_, _ = fmt.Fprintln(out, cat.Syntax)
	_, _ = fmt.Fprintf(out, "\nfact-time lexeme  %s  e.g. %s\n\nrelations\n", cat.Instant.Layout, cat.Instant.Example)
	for _, r := range append(append([]query.Relation{}, cat.Relations...), cat.Rules.Relations...) {
		_, _ = fmt.Fprintf(out, "  %-10s %s(%s)\n      %s\n", r.Kind, r.Name, strings.Join(r.Args, ", "), r.Doc)
	}
	_, _ = fmt.Fprintln(out, "\nbuiltins")
	for _, b := range cat.Builtins {
		names := make([]string, len(b.Args))
		for i, a := range b.Args {
			names[i] = a.Name
		}
		_, _ = fmt.Fprintf(out, "  %s(%s)\n      %s\n", b.Name, strings.Join(names, ", "), b.Doc)
	}
	_, _ = fmt.Fprintf(out, "\nrules  %s v%d  %s\n\ntypes\n", cat.Rules.Name, cat.Rules.Version, cat.Rules.Hash)
	for _, t := range cat.Types {
		line := fmt.Sprintf("  %s  v%d", t.Type, t.Current)
		if t.Series != "" {
			line += "  series " + t.Series
		}
		if t.Closes {
			line += "  closes"
		}
		_, _ = fmt.Fprintln(out, line)
		for _, v := range t.Versions {
			ps := append([]string{}, v.Pointers...)
			sort.Strings(ps)
			_, _ = fmt.Fprintf(out, "      v%d  %s\n", v.Version, strings.Join(ps, " "))
		}
	}
	return nil
}
