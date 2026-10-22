package app

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/mattjoyce/anam/internal/artifactdir"
	"github.com/mattjoyce/anam/internal/config"
	"github.com/mattjoyce/anam/internal/safepath"
	"github.com/mattjoyce/anam/internal/store/ops"
)

func moveArtifactsCmd(cfgPath *string) *cobra.Command {
	var o moveOptions
	cmd := &cobra.Command{
		Use:   "move-artifacts --from <folder> [--registered-from <old tracking.db>] [--yes]",
		Short: "read the old artifact folder into the operational store",
		Long: `Move-artifacts reads the folder that checkpoint values, receipts, served values
and saved selections used to be kept in, and writes each file into the operational
store as a row. A file is named by the SHA-256 of its bytes, and every file is read and
checked against its name before anything is written: if one does not match, nothing
is moved and the command says which.

A checkpoint value is loaded only if the checkpoint log names it. They are derived, so a
store whose log was rebuilt loads none and the next mint writes the ones it wants; every
checkpoint file is still checked.

Without --yes it only reads and reports: how many files of each kind, how many bytes, and
what it would load. With --yes it loads the rows. A row already in the store is left
alone, so running it again is safe. The folder is never changed.

With --registered-from, what an older operational store authored is first copied into
this one, read-only from the old file: the Definitions and projection templates
registered, the inline projection bodies, the admission holds, and the receipts, the
values they served, the saved selections and their codes, byte for byte. The store must
hold none of its own: that is what a fresh store from init is. Use it when the old store
was set aside because its schema moved on.

Like the other maintenance commands it opens the files itself, so stop the daemon first.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := loadConfig(*cfgPath, cmd)
			if err != nil {
				return err
			}
			return o.run(cmd.Context(), cmd.OutOrStdout(), cfg)
		},
	}
	f := cmd.Flags()
	f.StringVar(&o.from, "from", "", "the old artifact folder, read and never changed")
	f.StringVar(&o.registeredFrom, "registered-from", "",
		"an older operational store file to copy registered Definitions and projections from, opened read-only")
	f.BoolVar(&o.yes, "yes", false, "write the rows; without it the command only reports")
	_ = cmd.MarkFlagRequired("from")
	return cmd
}

type moveOptions struct {
	from, registeredFrom string
	yes                  bool
}

func (o moveOptions) run(ctx context.Context, out io.Writer, cfg config.Config) error {
	dir, err := safepath.Expand(o.from)
	if err != nil {
		return fmt.Errorf("--from: %w", err)
	}
	folder, err := artifactdir.Open(dir)
	if err != nil {
		return err
	}
	defer func() { _ = folder.Close() }()
	// The folder is proved whole before the store is opened, let alone written to.
	res, err := folder.Verify(ctx)
	if err != nil {
		return err
	}
	db, err := openStore(ctx, cfg)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()

	if o.registeredFrom != "" {
		if err := o.carryRegistered(ctx, out, db); err != nil {
			return err
		}
	}
	if o.yes {
		if err := folder.Load(ctx, db, &res); err != nil {
			return err
		}
	}
	writeMoved(out, res, o.yes)
	if !o.yes {
		_, err = fmt.Fprintln(out, "nothing written. Run again with --yes to load the rows.")
		return err
	}
	return nil
}

// carryRegistered copies the registered rows of the older store into db. With no --yes
// it reads them and reports what it would copy.
func (o moveOptions) carryRegistered(ctx context.Context, out io.Writer, db *ops.DB) error {
	path, err := safepath.ResolveFile(o.registeredFrom)
	if err != nil {
		return fmt.Errorf("--registered-from: %w", err)
	}
	old, err := ops.OpenReadOnly(ctx, path)
	if err != nil {
		return err
	}
	defer func() { _ = old.Close() }()
	reg, err := old.ReadRegistered(ctx)
	if err != nil {
		return fmt.Errorf("--registered-from %s: %w", path, err)
	}
	if !o.yes {
		_, err = fmt.Fprintf(out, "would copy %s from %s\n", carriedCounts(reg), path)
		return err
	}
	if err := db.CarryRegistered(ctx, reg); err != nil {
		return fmt.Errorf("copy the registered rows: %w (the store must hold none of its own)", err)
	}
	_, err = fmt.Fprintf(out, "copied %s from %s\n", carriedCounts(reg), path)
	return err
}

// carriedCounts says how many rows of each authored kind reg holds.
func carriedCounts(reg ops.Registered) string {
	byKind := map[string]int{}
	for _, c := range reg.Content {
		byKind[c.Kind]++
	}
	return fmt.Sprintf("%d registered Definitions, %d registered projection templates, %d inline projection bodies, "+
		"%d admission holds, %d receipts, %d served values, %d saved selections and %d selection codes",
		len(reg.Definitions), len(reg.Projections), len(reg.Inline), len(reg.Holds),
		byKind[ops.KindReceipt], byKind[ops.KindServed], byKind[ops.KindSelection], byKind[ops.KindSelectionCode])
}

// writeMoved prints the counts by kind. LOADED and PRESENT are what a write did; on a
// report they are zero, and the files and bytes are what the folder holds.
func writeMoved(out io.Writer, res artifactdir.Result, wrote bool) {
	_, _ = fmt.Fprintf(out, "folder %s: every file hashes to its name\n", res.Dir)
	w := tabwriter.NewWriter(out, 2, 0, 2, ' ', 0)
	if wrote {
		_, _ = fmt.Fprintln(w, "KIND\tFILES\tBYTES\tLOADED\tALREADY THERE\tNOT LOADED")
	} else {
		_, _ = fmt.Fprintln(w, "KIND\tFILES\tBYTES")
	}
	for _, c := range append(res.Counts, res.Total()) {
		if wrote {
			_, _ = fmt.Fprintf(w, "%s\t%d\t%d\t%d\t%d\t%d\n", c.Kind, c.Files, c.Bytes, c.Loaded, c.Present, c.Unused)
		} else {
			_, _ = fmt.Fprintf(w, "%s\t%d\t%d\n", c.Kind, c.Files, c.Bytes)
		}
	}
	_ = w.Flush()
	if wrote {
		_, _ = fmt.Fprintln(out, "NOT LOADED are checkpoint values the checkpoint log does not name; minting writes the ones it wants.")
	}
	if res.Leftovers > 0 {
		_, _ = fmt.Fprintf(out, "ignored %d temporary files left by a write that did not finish\n", res.Leftovers)
	}
}
