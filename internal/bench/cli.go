package bench

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/mattjoyce/anam/internal/safepath"
	"github.com/spf13/cobra"
)

// Main is the anam-bench command tree.
func Main(ctx context.Context, build string) error {
	root := &cobra.Command{
		Use:           "anam-bench",
		Short:         "Anam's read performance bench",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(genCmd(), runCmd(build), mintCmd(), sizesCmd(), compareCmd())
	return root.ExecuteContext(ctx)
}

func genCmd() *cobra.Command {
	var shape, dir string
	var seed int64
	budget := 0
	cmd := &cobra.Command{
		Use:   "gen",
		Short: "build a synthetic record of a named shape in an empty directory",
		Long: `Gen builds a record through the real gate: readings spread over monthly buckets,
corrections and retractions over the history, a mint of every bucket, then a tail
of new readings and late corrections after the mint. The same shape always yields
the same facts.

Shapes: ` + shapeList(),
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			sh, ok := Shapes()[shape]
			if !ok {
				return fmt.Errorf("no shape %q: %s", shape, shapeList())
			}
			sh.Budget = budget
			if _, err := os.Stat(PathsIn(dir).Ledger); err == nil {
				return fmt.Errorf("%s already holds a ledger", dir)
			}
			start := time.Now()
			err := Generate(cmd.Context(), dir, sh, func(s string) {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "%6.1fs %s\n", time.Since(start).Seconds(), s)
			})
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "built shape %s in %s (%.1fs)\n", shape, dir, time.Since(start).Seconds())
			return err
		},
	}
	cmd.Flags().StringVar(&shape, "shape", "S", "the shape to build")
	cmd.Flags().StringVar(&dir, "dir", "", "an empty directory to build it in")
	cmd.Flags().Int64Var(&seed, "seed", 42, "random seed for deterministic generation")
	cmd.Flags().IntVar(&budget, "budget", 0, "checkpoint_budget in bytes for the mint; 0 is the fixed monthly width")
	_ = cmd.MarkFlagRequired("dir")
	return cmd
}

func shapeList() string {
	var out []string
	for _, s := range Shapes() {
		out = append(out, fmt.Sprintf("%s (%d months x %d)", s.Name, s.Months, s.PerMonth))
	}
	sort.Strings(out)
	return strings.Join(out, ", ")
}

func runCmd(build string) *cobra.Command {
	o := Options{Build: build}
	var dir, out string
	cmd := &cobra.Command{
		Use:   "run",
		Short: "time every read scenario against the record in a directory",
		Long: `Run reads the record in --dir (ledger.db, tracking.db, artifacts/) with every
scenario, --reps times each, and prints the timings. With --out it also writes them
as JSON, for compare. The index is caught up first, untimed. Point it at a copy of
a record, never at one a daemon holds.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			o.Paths = PathsIn(dir)
			rep, err := Run(cmd.Context(), o, func(s string) {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "  %s\n", s)
			})
			if err != nil {
				return err
			}
			if err := WriteTable(cmd.OutOrStdout(), rep); err != nil {
				return err
			}
			if out == "" {
				return nil
			}
			b, err := json.MarshalIndent(rep, "", "  ")
			if err != nil {
				return err
			}
			return os.WriteFile(out, append(b, '\n'), 0o600)
		},
	}
	cmd.Flags().StringVar(&dir, "dir", "", "the record's directory")
	cmd.Flags().StringVar(&o.Name, "name", "", "the dataset's name, for the report")
	cmd.Flags().StringVar(&o.Label, "label", "baseline", "what this run is, for the report")
	cmd.Flags().StringVar(&o.Person, "person", Person, "whose ledger it is (synthetic records are "+Person+")")
	cmd.Flags().IntVar(&o.Reps, "reps", 5, "runs of each scenario")
	cmd.Flags().DurationVar(&o.Timeout, "timeout", 10*time.Minute, "bound on one run of one scenario")
	cmd.Flags().StringVar(&out, "out", "", "write the report as JSON here")
	cmd.Flags().StringArrayVar(&o.Only, "only", nil, "run only scenarios whose names contain this (repeatable)")
	cmd.Flags().IntVar(&o.Budget, "budget", 0, "checkpoint_budget for the dry-run mint scenario")
	_ = cmd.MarkFlagRequired("dir")
	return cmd
}

func mintCmd() *cobra.Command {
	var dir, person string
	budget := 0
	cmd := &cobra.Command{
		Use:   "mint",
		Short: "mint the record in a directory, as anam checkpoint does, recording the run",
		Long: `Mint catches the index up and mints every bucket that changed, recording the run, so
a composed read can start from it. Use it on a copy of a real record, whose checkpoints
were minted before runs were recorded.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			n, w, err := MintRecord(cmd.Context(), PathsIn(dir), person, budget)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "minted %d buckets, run recorded at W=%d\n", n, w)
			return err
		},
	}
	cmd.Flags().StringVar(&dir, "dir", "", "the record's directory")
	cmd.Flags().StringVar(&person, "person", Person, "whose ledger it is")
	cmd.Flags().IntVar(&budget, "budget", 0, "checkpoint_budget in bytes; 0 is the fixed monthly width")
	_ = cmd.MarkFlagRequired("dir")
	return cmd
}

func sizesCmd() *cobra.Command {
	var dir string
	cmd := &cobra.Command{
		Use:   "sizes",
		Short: "record the facet count of every checkpoint that has none, keeping the tail",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			n, err := BackfillSizes(cmd.Context(), PathsIn(dir))
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "recorded %d checkpoint sizes\n", n)
			return err
		},
	}
	cmd.Flags().StringVar(&dir, "dir", "", "the record's directory")
	_ = cmd.MarkFlagRequired("dir")
	return cmd
}

func compareCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "compare <before.json> <after.json>",
		Short: "print two runs' medians side by side",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			var reps [2]Report
			for i, p := range args {
				b, err := readReport(p)
				if err != nil {
					return err
				}
				if err := json.Unmarshal(b, &reps[i]); err != nil {
					return fmt.Errorf("%s: %w", p, err)
				}
			}
			return WriteComparison(cmd.OutOrStdout(), reps[0], reps[1])
		},
	}
}

// readReport reads a report file the operator named.
func readReport(path string) ([]byte, error) {
	f, err := safepath.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return io.ReadAll(f)
}
