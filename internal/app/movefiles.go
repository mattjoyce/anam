package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"slices"
	"strings"
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
registered, the template bodies, the admission holds, the receipts, the values they
served, the saved selections and their codes, byte for byte, and the provisioned
Definitions, including a type the catalog has since dropped. The old file may be of
schema version 9 or this build's; a version-9 file holds its template bodies under the
name inline_projection, and they arrive as template bodies. Use it when the old store
was set aside because its schema moved on.

The command runs in three steps, and says which one failed:

  1. Read. The folder is checked file by file, and the old store is read in one read
     transaction and checked: a version the carry reads, every table it copies there,
     no rows under the other version's name for the template bodies, no meta key it
     does not know, and every content row hashing to its key. Nothing is written yet.
  2. Carry (--yes). The store must be fresh: no registered row, template body, hold,
     receipt, served value, selection, code or provisioned Definition of its own, which
     is a store from init that no daemon has started on. Everything read in step 1 is
     written in one transaction, all of it or none. A failure leaves the store as it was.
  3. Folder (--yes). The folder's files are loaded one row at a time; a row already
     there is left alone.

The old file and the folder are never written. Run again after a failure in step 1 or
2 as it was. After a failure in step 3 the carry is committed: run again without
--registered-from to finish the folder, since a second carry into the same store is
refused because the store is no longer fresh.

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
		"an older operational store file, of schema version 9 or this build's, to copy what it authored from, opened read-only")
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
	// The folder and the old store are proved whole before the store is opened, let
	// alone written to.
	res, err := folder.Verify(ctx)
	if err != nil {
		return err
	}
	var src *ops.CarrySource
	if o.registeredFrom != "" {
		if src, err = o.readSource(ctx, out, cfg); err != nil {
			return err
		}
	}
	db, err := openStore(ctx, cfg)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()

	if src != nil && o.yes {
		if err := db.CarrySchema(ctx, src.Registered); err != nil {
			return fmt.Errorf("carry: %w. Nothing was copied and the folder was not loaded. If an earlier run with --registered-from --yes "+
				"finished its carry into this store, run again without --registered-from to load the folder; otherwise set this store aside "+
				"and carry into a fresh one", err)
		}
		if _, err := fmt.Fprintf(out, "copied %s\n", carriedCounts(src.Registered)); err != nil {
			return err
		}
	}
	if o.yes {
		if err := folder.Load(ctx, db, &res); err != nil {
			if src != nil {
				return fmt.Errorf("load the folder: %w. The carry from %s is committed: run again without --registered-from to finish the folder",
					err, o.registeredFrom)
			}
			return fmt.Errorf("load the folder: %w", err)
		}
	}
	writeMoved(out, res, o.yes)
	if !o.yes {
		_, err = fmt.Fprintln(out, "nothing written. Run again with --yes to load the rows.")
		return err
	}
	return nil
}

// readSource reads and checks the older store the carry copies from, read-only, and
// reports what it holds. It refuses the store this command writes into.
func (o moveOptions) readSource(ctx context.Context, out io.Writer, cfg config.Config) (*ops.CarrySource, error) {
	path, err := safepath.ResolveFile(o.registeredFrom)
	if err != nil {
		return nil, fmt.Errorf("--registered-from: %w", err)
	}
	if same, err := sameFile(path, cfg.DBPath); err != nil {
		return nil, fmt.Errorf("--registered-from: %w", err)
	} else if same {
		return nil, fmt.Errorf("--registered-from %s is the store this command writes into (db_path); carry from the file set aside", path)
	}
	src, err := ops.ReadCarrySource(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("--registered-from %s: %w; nothing was written", path, err)
	}
	left := make([]string, len(src.Left))
	for i, c := range src.Left {
		left[i] = fmt.Sprintf("%s %d", c.Table, c.Rows)
	}
	ledgerID := src.LedgerID
	if ledgerID == "" {
		ledgerID = "none pinned"
	}
	verb := "would copy"
	if o.yes {
		verb = "copying"
	}
	_, err = fmt.Fprintf(out, "%s: schema version %d, ledger %s, bucket width %s\n"+
		"%s %s\n"+
		"left behind, rebuilt by replay and minting or remade by the next resend: %s\n",
		path, src.Schema, ledgerID, src.BucketWidth, verb, carriedCounts(src.Registered), strings.Join(left, ", "))
	return &src, err
}

// sameFile reports whether a and b name one file. A b that does not exist yet is not a.
func sameFile(a, b string) (bool, error) {
	sa, err := os.Stat(a)
	if err != nil {
		return false, err
	}
	sb, err := os.Stat(b)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return os.SameFile(sa, sb), nil
}

// carriedCounts says how many rows of each authored kind reg holds.
func carriedCounts(reg ops.Registered) string {
	byKind := map[string]int{}
	for _, c := range reg.Content {
		byKind[c.Kind]++
	}
	return fmt.Sprintf("%d registered Definitions, %d registered projection templates, %d template bodies, "+
		"%d admission holds, %d receipts, %d served values, %d saved selections, %d selection codes and %d provisioned Definitions",
		len(reg.Definitions), len(reg.Projections), len(reg.TemplateBodies), len(reg.Holds),
		byKind[ops.KindReceipt], byKind[ops.KindServed], byKind[ops.KindSelection], byKind[ops.KindSelectionCode],
		len(reg.Provisioned))
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
	for _, dir := range slices.Sorted(maps.Keys(res.Legacy)) {
		_, _ = fmt.Fprintf(out, "read %d files from %s/, an older build's name for their kind; they are counted and filed under the current name\n",
			res.Legacy[dir], dir)
	}
	if res.Twins > 0 {
		_, _ = fmt.Fprintf(out, "%d digests are filed under both a kind's current and older name; each copy was checked, and the bytes are stored once\n", res.Twins)
	}
	if res.Leftovers > 0 {
		_, _ = fmt.Fprintf(out, "ignored %d temporary files left by a write that did not finish\n", res.Leftovers)
	}
}
