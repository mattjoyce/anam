package app

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mattjoyce/anam/internal/api"
)

// holdCmd is the operator's admission hold: hold a source's admission, release it, or
// see what is held.
func holdCmd(cfgPath *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "hold [source...]",
		Short: "hold, release or list the admission holds",
		Long: `A hold stops a source's admission: its envelopes and its Definition registrations
are refused with 503 and a Retry-After, and nothing of it is admitted. Reads, the
operator's own writes and the workbench carry on. A hold is the operator's, is kept in
the operational store, and survives a restart of the daemon.

anam hold garmin bp-csv     hold those sources
anam hold --all             hold every source
anam hold --release         release every hold
anam hold --release garmin  release one source
anam hold                   list what is held`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadRoutedConfig(*cfgPath, cmd)
			if err != nil {
				return err
			}
			if cfg.Server == "" {
				return errors.New("hold needs the daemon: set server (ANAM_SERVER or --server)")
			}
			c, err := newClient(cfg)
			if err != nil {
				return err
			}
			release, err := cmd.Flags().GetBool("release")
			if err != nil {
				return err
			}
			all, err := cmd.Flags().GetBool("all")
			if err != nil {
				return err
			}
			switch {
			case release:
				h, err := c.ReleaseHold(cmd.Context(), api.HoldReleaseRequest{Sources: args, All: all})
				if err != nil {
					return err
				}
				return writeHolds(cmd, h.Held)
			case len(args) == 0 && !all:
				s, err := c.Status(cmd.Context())
				if err != nil {
					return err
				}
				return writeHolds(cmd, s.Held)
			default:
				h, err := c.Hold(cmd.Context(), api.HoldRequest{Sources: args, All: all})
				if err != nil {
					return err
				}
				return writeHolds(cmd, h.Held)
			}
		},
	}
	cmd.Flags().Bool("release", false, "release the holds named, or every hold when none is named")
	cmd.Flags().Bool("all", false, "hold every source")
	return cmd
}

// writeHolds prints what the daemon holds.
func writeHolds(cmd *cobra.Command, held []string) error {
	if len(held) == 0 {
		_, err := fmt.Fprintln(cmd.OutOrStdout(), "holds   (none)")
		return err
	}
	_, err := fmt.Fprintf(cmd.OutOrStdout(), "holds   %s\n", strings.Join(held, " "))
	return err
}
