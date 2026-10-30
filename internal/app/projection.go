package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strconv"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/mattjoyce/anam/internal/api"
	"github.com/mattjoyce/anam/internal/catalog"
	"github.com/mattjoyce/anam/internal/config"
	"github.com/mattjoyce/anam/internal/projection"
	"github.com/mattjoyce/anam/internal/store/ops"
)

// projectionCmd is the operator's view of the projection templates (card 125): what
// is held, one manifest as held, and registering a new one over the daemon.
func projectionCmd(cfgPath *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "projection",
		Short: "list, show and register projection templates",
		Long: `A projection template prunes the canonical for a purpose: a type allow-list, a
status filter, a classification ceiling, a fact-time window and a shape. Templates
are bundled with the binary, the floor, or registered by the operator; a consumer
selects one and never defines one. Every version ever held stays readable, because a
receipt names a version and verify re-prunes through exactly that one.`,
	}
	cmd.AddCommand(projectionListCmd(cfgPath), projectionShowCmd(cfgPath), projectionRegisterCmd(cfgPath))
	return cmd
}

func projectionListCmd(cfgPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "list the templates held, newest version of each",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := loadRoutedConfig(*cfgPath, cmd)
			if err != nil {
				return err
			}
			if cfg.Server != "" {
				c, err := newClient(cfg)
				if err != nil {
					return err
				}
				list, err := c.Projections(cmd.Context())
				if err != nil {
					return err
				}
				return writeProjections(cmd, list)
			}
			templates, err := localProjections(cmd.Context(), cfg)
			if err != nil {
				return err
			}
			return listProjections(cmd, templates)
		},
	}
}

func projectionShowCmd(cfgPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "show <name> [version]",
		Short: "print one template's manifest as held, the newest version unless one is named",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadRoutedConfig(*cfgPath, cmd)
			if err != nil {
				return err
			}
			version := 0
			if len(args) == 2 {
				if version, err = strconv.Atoi(args[1]); err != nil || version < 1 {
					return fmt.Errorf("version %q: want a positive integer", args[1])
				}
			}
			var body []byte
			if cfg.Server != "" {
				c, err := newClient(cfg)
				if err != nil {
					return err
				}
				raw, err := c.Projection(cmd.Context(), args[0], version)
				if err != nil {
					return err
				}
				body = raw
			} else {
				templates, err := localProjections(cmd.Context(), cfg)
				if err != nil {
					return err
				}
				var t projection.Template
				if version == 0 {
					t, err = templates.Lookup(args[0])
				} else {
					t, err = templates.Version(args[0], version)
				}
				if err != nil {
					return err
				}
				body = t.Body
			}
			var pretty json.RawMessage = body
			out, err := json.MarshalIndent(pretty, "", "  ")
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), string(out))
			return err
		},
	}
}

func projectionRegisterCmd(cfgPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "register <manifest.json>",
		Short: "register a template with the daemon; the same body again is unchanged",
		Long: `Register sends a template manifest to the daemon under the operator's token. Every
type it names must have a Definition in force. A held version is immutable: the same
body again is reported unchanged, a different body at that version is refused, and a
change is a new version. Registering needs the daemon (server, ANAM_SERVER): the
record's operational store is its to write.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadRoutedConfig(*cfgPath, cmd)
			if err != nil {
				return err
			}
			if cfg.Server == "" {
				return errors.New("projection register needs the daemon: set server (ANAM_SERVER or --server)")
			}
			body, err := os.ReadFile(args[0]) //nolint:gosec // the operator names the file to register
			if err != nil {
				return err
			}
			// Parse first, so a bad manifest is refused here with the line that is
			// wrong, before a round trip.
			if _, err := projection.Parse(body); err != nil {
				return err
			}
			c, err := newClient(cfg)
			if err != nil {
				return err
			}
			reg, err := c.RegisterProjection(cmd.Context(), body)
			if err != nil {
				return err
			}
			verb := "registered"
			if !reg.New {
				verb = "already held (unchanged)"
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s %s v%d  %s\n", verb, reg.Name, reg.Version, reg.Hash)
			return err
		},
	}
}

// writeProjections prints the daemon's list in the shape listProjections prints a
// local registry, with the origin beside each.
func writeProjections(cmd *cobra.Command, list api.Projections) error {
	w := tabwriter.NewWriter(cmd.OutOrStdout(), 2, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "PROJECTION\tV\tORIGIN\tSHAPE\tTYPES\tVERSIONS")
	for _, p := range list.Projections {
		versions := make([]string, len(p.Versions))
		for i, v := range p.Versions {
			versions[i] = strconv.Itoa(v)
		}
		_, _ = fmt.Fprintf(w, "%s\t%d\t%s\t%s\t%s\t%s\n",
			p.Name, p.Current, p.Origin, p.Shape, joinStrings(p.Types), joinStrings(versions))
	}
	if err := w.Flush(); err != nil {
		return err
	}
	_, err := fmt.Fprintf(cmd.ErrOrStderr(), "\nselect one: anam timeline --projection <name>; show one: anam projection show <name> [v]\n")
	return err
}

func joinStrings(ss []string) string {
	if len(ss) == 0 {
		return "(none)"
	}
	out := ss[0]
	for _, s := range ss[1:] {
		out += "," + s
	}
	return out
}

// projectionsOf is the registry a local read selects from: the catalog's templates
// and those registered in db.
func projectionsOf(ctx context.Context, cat *catalog.Catalog, db *ops.DB) (*projection.Registry, error) {
	bundled := cat.Projections()
	rows, err := db.RegisteredProjections(ctx)
	if err != nil {
		return nil, err
	}
	stored := make([]projection.Stored, len(rows))
	for i, r := range rows {
		stored[i] = projection.Stored{Name: r.Name, Version: r.Version, Body: []byte(r.Body)}
	}
	return projection.WithRegistered(bundled, stored)
}

// localProjections is the registry the local files hold. Without an operational store
// yet (no anam init) it is the catalog's set alone: listing the menu must not create
// a store as a side effect.
func localProjections(ctx context.Context, cfg config.Config) (*projection.Registry, error) {
	cat, err := loadCatalog(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(cfg.DBPath); errors.Is(err, fs.ErrNotExist) {
		return cat.Projections(), nil
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
	return projectionsOf(ctx, cat, db)
}
