package app

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/mattjoyce/anam/internal/frame"
)

// writeFrame renders a frame as a table: the columns as the header, one line per
// row. A string cell is shown bare, any other cell as its JSON; a fact_time cell is
// shown in the reader's zone, as the entries table shows it. --json prints the frame
// untouched, so nothing is lost here.
func writeFrame(out io.Writer, f *frame.Frame, loc *time.Location) error {
	w := tabwriter.NewWriter(out, 2, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, strings.ToUpper(strings.Join(f.Columns, "\t")))
	for _, row := range f.Rows {
		cells := make([]string, 0, len(row))
		for i, cell := range row {
			s := cellText(cell)
			if i < len(f.Columns) && f.Columns[i] == "fact_time" && loc != nil {
				s = showIn(loc, s)
			}
			cells = append(cells, s)
		}
		_, _ = fmt.Fprintln(w, strings.Join(cells, "\t"))
	}
	return w.Flush()
}

// cellText is a cell for the eye: a string without its quotes, anything else as
// its JSON, clipped to a terminal line.
func cellText(cell json.RawMessage) string {
	var s string
	if err := json.Unmarshal(cell, &s); err == nil {
		return s
	}
	return clipPayload(cell)
}
