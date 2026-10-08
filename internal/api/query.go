package api

import (
	"net/url"
	"strconv"
)

// The read queries travel as URL query parameters, under their JSON names.

// Values encodes q for a GET.
func (q TimelineQuery) Values() url.Values {
	v := url.Values{}
	set(v, "from", q.From)
	set(v, "to", q.To)
	set(v, "at", q.At)
	set(v, "as_of", q.AsOf)
	set(v, "type", q.Type)
	set(v, "projection", q.Projection)
	if q.Explain {
		v.Set("explain", "true")
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
			return TimelineQuery{}, err
		}
		explain = b
	}
	return TimelineQuery{
		From: v.Get("from"), To: v.Get("to"), At: v.Get("at"), AsOf: v.Get("as_of"),
		Type: v.Get("type"), Projection: v.Get("projection"), Explain: explain, Zone: v.Get("zone"),
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
