package api

import (
	"fmt"
	"net/url"
	"strconv"

	"github.com/mattjoyce/anam/internal/facet"
)

// The read queries travel as URL query parameters, under their JSON names.

// Values encodes q for a GET.
func (q TimelineQuery) Values() url.Values {
	v := url.Values{}
	set(v, "from", q.From)
	set(v, "to", q.To)
	set(v, "at", q.At)
	set(v, "as_of", q.AsOf)
	if q.W != 0 {
		v.Set("w", strconv.FormatInt(q.W, 10))
	}
	set(v, "type", q.Type)
	set(v, "projection", q.Projection)
	if q.Explain {
		v.Set("explain", "true")
	}
	for _, f := range q.Fields {
		v.Add("fields", f)
	}
	set(v, "zone", q.Zone)
	return v
}

// TimelineQueryOf decodes a timeline query from a GET's parameters.
func TimelineQueryOf(v url.Values) (TimelineQuery, error) {
	explain := false
	if s := v.Get("explain"); s != "" {
		b, err := strconv.ParseBool(s)
		if err != nil {
			return TimelineQuery{}, fmt.Errorf("explain: %w", err)
		}
		explain = b
	}
	var w int64
	if s := v.Get("w"); s != "" {
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return TimelineQuery{}, fmt.Errorf("w %q: %w", s, err)
		}
		w = n
	}
	if t := v.Get("type"); t != "" && !facet.ValidPattern(t) {
		return TimelineQuery{}, fmt.Errorf("type %q: want a facet type, or a prefix ending in a dot (activity.exercise-session.)", t)
	}
	return TimelineQuery{
		From: v.Get("from"), To: v.Get("to"), At: v.Get("at"), AsOf: v.Get("as_of"), W: w,
		Type: v.Get("type"), Projection: v.Get("projection"), Explain: explain, Fields: v["fields"],
		Zone: v.Get("zone"),
	}, nil
}

// Values encodes q for a GET.
func (q ChangesQuery) Values() url.Values {
	v := url.Values{}
	set(v, "since", q.Since)
	set(v, "until", q.Until)
	set(v, "zone", q.Zone)
	return v
}

// ChangesQueryOf decodes a changes query from a GET's parameters.
func ChangesQueryOf(v url.Values) ChangesQuery {
	return ChangesQuery{Since: v.Get("since"), Until: v.Get("until"), Zone: v.Get("zone")}
}

func set(v url.Values, key, val string) {
	if val != "" {
		v.Set(key, val)
	}
}
