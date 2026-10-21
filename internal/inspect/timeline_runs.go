package inspect

import (
	"fmt"
	"net/url"
	"time"

	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/facet"
)

// Density by pixels, not by scope levels (card 135). A lane (one type, one
// liveness) draws a lone facet as its symbol; marks closer together than the eye
// can part become a run: first and last joined by a line, with a count, and the
// run's link is the window that spreads it out. The page then tracks the screen,
// not the record: a daily measure over years is one run until the window is weeks.

const (
	// runGap is how close, in viewBox units, two neighbouring marks must be to
	// join a run; at the band's natural width a unit is about a pixel.
	runGap = 3.0
	// runMin is the smallest run worth drawing as one: two marks draw as marks.
	runMin = 3
	// runLabelMin is the narrowest run that carries its count as text; a narrower
	// one says it in its title alone, so sub-lanes do not pile labels up.
	runLabelMin = 40.0
)

// tlRun is a dense stretch of one lane drawn as its two ends and a line.
type tlRun struct {
	X1, X2, Y     float64
	First, Last   string // the two end marks' paths
	Hue           string
	Live          bool
	Picked, Faded bool // a saved selection's, or not, drawn among the rest (card 206)
	Count         int
	Label         string // the count, or "" when the run is too narrow to carry it
	Title, Href   string
	LabelX        float64
	LabelBaseline float64
}

// laneMark is a facet placed on the band, before runs are found. below says the
// lane sits in the lower half of its row, so a run's count goes under the line
// rather than into the lane above.
type laneMark struct {
	point tlPoint
	at    time.Time
	typ   facet.Type
	below bool
}

// laneRuns splits one lane's marks, in time order, into lone points and runs. The
// view rides the run's link, so zooming into a run keeps the filters.
func laneRuns(marks []laneMark, view tlView) ([]tlPoint, []tlRun) {
	var points []tlPoint
	var runs []tlRun
	flush := func(group []laneMark) {
		if len(group) < runMin {
			for _, m := range group {
				points = append(points, m.point)
			}
			return
		}
		runs = append(runs, makeRun(group, view))
	}
	start := 0
	for i := 1; i <= len(marks); i++ {
		if i < len(marks) && marks[i].point.X-marks[i-1].point.X <= runGap {
			continue
		}
		flush(marks[start:i])
		start = i
	}
	return points, runs
}

// makeRun describes a group of three or more marks as one run. Its window is the
// group's span padded by a tenth each side (an hour at least), or a day about a
// group at one instant.
func makeRun(group []laneMark, view tlView) tlRun {
	first, last := group[0], group[len(group)-1]
	span := last.at.Sub(first.at)
	pad := max(span/10, time.Hour)
	if span == 0 {
		pad = 12 * time.Hour
	}
	r := tlRun{
		X1: first.point.X, X2: last.point.X, Y: first.point.Y,
		First: first.point.Path, Last: last.point.Path,
		Hue: first.point.Hue, Live: first.point.Live, Count: len(group),
		Picked: first.point.Picked, Faded: first.point.Faded,
		Href: spanLink(first.at.Add(-pad), last.at.Add(pad), view),
		Title: fmt.Sprintf("%d × %s · %s to %s · click to zoom in", len(group), first.typ,
			first.at.UTC().Format("2006-01-02 15:04Z"), last.at.UTC().Format("2006-01-02 15:04Z")),
	}
	if span == 0 {
		// Marks at one instant never come apart however far the view zooms, so a
		// zoom would only draw the same run again. List them instead: the selection
		// page at that one instant (from included, to not) shows each with its link.
		r.Href = momentLink(first.typ, first.at)
		r.Title = fmt.Sprintf("%d × %s · all at %s · click to list them", len(group), first.typ,
			first.at.UTC().Format("2006-01-02 15:04Z"))
	}
	if r.X2-r.X1 >= runLabelMin {
		r.Label = fmt.Sprintf("%d", len(group))
		r.LabelX = (r.X1 + r.X2) / 2
		r.LabelBaseline = r.Y - 5
		if first.below {
			r.LabelBaseline = r.Y + 11
		}
	}
	return r
}

// momentLink is the selection page listing the facets of one type at one instant.
func momentLink(t facet.Type, at time.Time) string {
	q := url.Values{}
	q.Set("type", string(t))
	q.Set("from", clock.Format(at))
	q.Set("to", clock.Format(at.Add(time.Nanosecond)))
	return "/select?" + q.Encode()
}
