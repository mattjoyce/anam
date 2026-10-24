package inspect

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/read"
)

// The reader's zone (card 236). A bare date typed on the select page, the projection
// page, the timeline or the location page is a day in the reader's zone, the rule the
// API, the CLI and anam-mcp already keep (clock.ParseBound). The zone is a `zone`
// parameter naming an IANA zone, shown beside the dates and editable. A request
// without one reads in the zone the daemon is configured with (Deps.Zone), as the
// pages did in UTC before; the page's script fills the field with the browser's zone
// on a page that has nothing to misread, and the form sends it from then on, the way
// the entry form fills a fact's zone.

// defaultZone is the zone a Server reads dates in when its Deps name none.
const defaultZone = "UTC"

// readZone is the zone a request reads its bare dates in.
type readZone struct {
	Name string
	Loc  *time.Location
	// Given says the request named the zone. Dated says it already carries a date,
	// a saved selection or a program, read in this zone: the page's script must not
	// swap the zone under what was read.
	Given, Dated bool
}

// Fillable says the page's script may replace the zone with the browser's: the
// request named none and carries nothing read in one.
func (z readZone) Fillable() bool { return !z.Given && !z.Dated }

// zoneNamed is the zone a name loads as, or the 400's reason: an unknown name, or
// Local, which would mean the daemon's machine and not the reader's.
func zoneNamed(name string) (readZone, error) {
	loc, err := read.NamedLocation(name)
	if err != nil {
		return readZone{}, fmt.Errorf("%w: %v", errBadZone, err)
	}
	return readZone{Name: name, Loc: loc}, nil
}

// errBadZone is a zone the request names that cannot be read.
var errBadZone = fmt.Errorf("bad zone")

// zoneOf reads the request's zone: its zone parameter, else the server's default.
func (s *Server) zoneOf(q url.Values) (readZone, error) {
	name := strings.TrimSpace(q.Get("zone"))
	given := name != ""
	if !given {
		name = s.zone
	}
	z, err := zoneNamed(name)
	if err != nil {
		return z, err
	}
	z.Given = given
	z.Dated = q.Get("from") != "" || q.Get("to") != "" || q.Get("sel") != "" || q.Get("program") != ""
	return z, nil
}

// zoneOrFail is zoneOf for a handler: a zone it cannot read is answered 400.
func (s *Server) zoneOrFail(w http.ResponseWriter, q url.Values) (readZone, bool) {
	z, err := s.zoneOf(q)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return z, false
	}
	return z, true
}

// dayIn writes t as the date it starts in loc, when t is midnight there: a to bound
// as the day before, since a to date is the whole day. ok is false off midnight.
func dayIn(t time.Time, loc *time.Location, upper bool) (date string, ok bool) {
	t = t.In(loc)
	if h, m, sec := t.Clock(); h != 0 || m != 0 || sec != 0 || t.Nanosecond() != 0 {
		return "", false
	}
	if upper {
		t = t.AddDate(0, 0, -1)
	}
	return t.Format(time.DateOnly), true
}

// boundIn writes a bound for an address: the date when the lexeme is midnight in loc,
// else the instant as RFC 3339, which the timeline reads exactly.
func boundIn(t time.Time, loc *time.Location, upper bool) string {
	if d, ok := dayIn(t, loc, upper); ok {
		return d
	}
	return t.UTC().Format(time.RFC3339Nano)
}

// lexemeIn is the same bound for the select page, whose instants are fact-time
// lexemes.
func lexemeIn(t time.Time, loc *time.Location, upper bool) string {
	if d, ok := dayIn(t, loc, upper); ok {
		return d
	}
	return clock.Format(t)
}
