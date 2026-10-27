package bench

import (
	"fmt"
	"io"
	"text/tabwriter"
)

// WriteTable prints a report for a person to read.
func WriteTable(w io.Writer, r Report) error {
	d := r.Dataset
	_, _ = fmt.Fprintf(w, "%s on %s (%s, %d CPUs), build %s, %s\n", r.Label, r.Host, r.Platform, r.CPUs, r.Build, r.Started)
	_, _ = fmt.Fprintf(w, "dataset %s: W=%d, %d facets, %d live, %d buckets, %d checkpoints, last mint run W=%d, ledger %.1f MB\n\n",
		d.Name, d.W, d.Facets, d.Live, d.Buckets, d.Checkpoints, d.LastRun, float64(d.LedgerBytes)/(1<<20))
	tw := tabwriter.NewWriter(w, 2, 0, 2, ' ', tabwriter.AlignRight)
	_, _ = fmt.Fprintln(tw, "SCENARIO\tMIN ms\tMEDIAN ms\tMAX ms\tROWS\t")
	for _, s := range r.Scenarios {
		_, _ = fmt.Fprintf(tw, "%s\t%.1f\t%.1f\t%.1f\t%d\t\n", s.Name, s.MinMS, s.MedianMS, s.MaxMS, s.Rows)
	}
	return tw.Flush()
}

// WriteComparison prints two reports' medians side by side, scenario by scenario.
func WriteComparison(w io.Writer, before, after Report) error {
	_, _ = fmt.Fprintf(w, "before: %s, %s on %s, W=%d\n", before.Label, before.Build, before.Host, before.Dataset.W)
	_, _ = fmt.Fprintf(w, "after:  %s, %s on %s, W=%d\n\n", after.Label, after.Build, after.Host, after.Dataset.W)
	was := map[string]Scenario{}
	for _, s := range before.Scenarios {
		was[s.Name] = s
	}
	tw := tabwriter.NewWriter(w, 2, 0, 2, ' ', tabwriter.AlignRight)
	_, _ = fmt.Fprintln(tw, "SCENARIO\tBEFORE ms\tAFTER ms\tSPEED-UP\tROWS\t")
	for _, s := range after.Scenarios {
		b, ok := was[s.Name]
		if !ok {
			if s.Steps > 0 || b.Steps > 0 {
				_, _ = fmt.Fprintf(tw, "%s\t-\t%.1f\t-\t%d\t\n", s.Name+"  steps "+fmt.Sprint(b.Steps)+" -> "+fmt.Sprint(s.Steps), s.MedianMS, s.Rows)
				continue
			}
			_, _ = fmt.Fprintf(tw, "%s\t-\t%.1f\t-\t%d\t\n", s.Name, s.MedianMS, s.Rows)
			continue
		}
		rows := fmt.Sprint(s.Rows)
		if b.Rows != s.Rows {
			rows = fmt.Sprintf("%d (was %d)", s.Rows, b.Rows)
		}
		name := s.Name
		if b.Steps > 0 || s.Steps > 0 {
			name = s.Name + "  steps " + fmt.Sprint(b.Steps) + " -> " + fmt.Sprint(s.Steps)
		}
		_, _ = fmt.Fprintf(tw, "%s\t%.1f\t%.1f\t%.1fx\t%s\t\n", name, b.MedianMS, s.MedianMS, b.MedianMS/max(s.MedianMS, 0.001), rows)
	}
	return tw.Flush()
}
