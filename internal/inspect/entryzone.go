package inspect

import (
	"encoding/json"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// A zoned form (card 198): the Definition's fact time reads the zone a fact names, so
// the form sends source_time beside the drawn fields, from hidden fields the page's
// script fills with the browser's zone and the offset of the wall clock typed.

// The fields a zoned form posts beside the drawn ones (f.<name>): the zone, hidden,
// and the offset, which the page fills and the person may correct.
const (
	zoneField   = "zone"
	offsetField = "offset"
)

// offsetText is the shape of an offset the form takes, +HH:MM or -HH:MM; the gate
// checks it against the zone again.
var offsetText = regexp.MustCompile(`^[+-]([01][0-9]|2[0-3]):[0-5][0-9]$`)

// sourceTime is the source_time a zoned form sends: the fact-time value as typed, the
// zone the browser posted, the offset that wall clock had in it, and basis source,
// since the person's own browser named the zone. The offset is what places the fact,
// so the form always sends one, and with none it sends nothing and says so beside the
// field: the page's script fills it from the browser when the browser can tell it, and
// leaves it to the person for a time their clocks repeat or skip, or with no script.
// With no zone posted, the offset names the zone too. ok is false with OffsetProblem
// set when no offset was given.
func (f *entryForm) sourceTime(vals url.Values, payload map[string]json.RawMessage) (json.RawMessage, bool) {
	f.Zone = strings.TrimSpace(vals.Get(zoneField))
	f.Offset = strings.TrimSpace(vals.Get(offsetField))
	f.OffsetProblem = ""
	raw, has := payload[strings.TrimPrefix(f.timeAt, "/")]
	if !f.Zoned || !has {
		return nil, true
	}
	var value string
	if json.Unmarshal(raw, &value) != nil {
		return nil, true // the fact-time field is always encoded as text
	}
	if !offsetText.MatchString(f.Offset) {
		f.OffsetProblem = "Anam needs the offset from UTC this time had where you were, +HH:MM or -HH:MM " +
			"(+01:00 for London in summer). Your browser did not give one: the page runs no script, or your clocks " +
			"repeat or skip this time. Nothing was sent; type the offset."
		return nil, false
	}
	zone := f.Zone
	if zone == "" {
		zone = f.Offset
	}
	out, err := json.Marshal(map[string]string{"value": value, "zone": zone, "offset": f.Offset, "basis": "source"})
	if err != nil {
		return nil, false // a map of strings always marshals
	}
	return out, true
}

// wallLoc is the zone the form reads and writes the fact-time control in. A zoneless
// layout writes a wall clock, so its digits are carried as typed, in UTC, which moves
// none of them: read in the Definition's zone, a wall clock it skips would be moved on
// an hour before the gate saw it. A layout with an offset is written in the
// Definition's zone, as before.
func (f *entryForm) wallLoc() *time.Location {
	wall := time.Date(2001, 2, 3, 4, 5, 6, 0, time.UTC)
	if wall.Format(f.layout) == time.Date(2001, 2, 3, 4, 5, 6, 0, time.FixedZone("", 5*3600+7*60)).Format(f.layout) {
		return time.UTC
	}
	return f.loc
}
