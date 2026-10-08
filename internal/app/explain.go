package app

import (
	"fmt"
	"text/tabwriter"

	"github.com/mattjoyce/anam/internal/api"
	"github.com/spf13/cobra"
)

// writeExplain renders an explained read: the header, the verdict counts, and a row
// per artifact version with its standing.
func writeExplain(cmd *cobra.Command, t api.Timeline) error {
	out := cmd.OutOrStdout()
	_, _ = fmt.Fprint(out, timelineHeader(t))
	_, _ = fmt.Fprintf(out, "verdicts   %d served, %d excluded, %d superseded\n",
		t.Counts.Served, t.Counts.Excluded, t.Counts.Superseded)
	if t.Projection == nil {
		_, _ = fmt.Fprintln(cmd.ErrOrStderr(),
			"\nno --projection: every live facet is 'served' by definition; the reasons appear once a template is named")
	}

	w := tabwriter.NewWriter(out, 2, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "FACT TIME\tTYPE\tV\tSTANDING\tWHY\tSOURCE REF")
	for _, r := range t.Verdicts {
		_, _ = fmt.Fprintf(w, "%s\t%s\tv%d\t%s\t%s\t%s\n",
			r.FactTime, r.Type, r.Version, r.Standing, r.Reason, r.SourceRef)
	}
	return w.Flush()
}
