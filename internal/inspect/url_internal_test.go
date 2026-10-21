package inspect

import (
	"net/url"
	"testing"
)

// An address is a path on this daemon with a query behind it. Every key and value
// is percent-encoded, so what a reader put in the query stays a query: it cannot
// become a scheme, a host or a path of its own. The keys are written in order, so
// the same query always gives the same address.
func TestAddress(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name, path string
		query      url.Values
		want       string
	}{
		{"nothing at all", "", nil, ""},
		{"a path with no query", "/select", nil, "/select?"},
		{"one pair", "/select", url.Values{"type": {"health.blood-pressure"}}, "/select?type=health.blood-pressure"},
		{"keys in order", "/", url.Values{"to": {"2026-02-01"}, "from": {"2026-01-01"}}, "/?from=2026-01-01&to=2026-02-01"},
		{"every value of a key", "/", url.Values{"type": {"health.weight", "health.sleep"}}, "/?type=health.weight&type=health.sleep"},
		{"a query alone", "", url.Values{"type": {"health.weight"}}, "type=health.weight"},
		{
			"a reader cannot smuggle a url",
			"/select",
			url.Values{"program": {"https://elsewhere.example/x"}},
			"/select?program=https%3A%2F%2Felsewhere.example%2Fx",
		},
		{
			"a reader cannot smuggle a path",
			"/select",
			url.Values{"edit": {"../../etc/passwd"}},
			"/select?edit=..%2F..%2Fetc%2Fpasswd",
		},
		{
			"spaces and quotes are escaped",
			"/select",
			url.Values{"program": {`x("a b")`}},
			"/select?program=x%28%22a+b%22%29",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := address(tc.path, tc.query); got != tc.want {
				t.Errorf("address(%q, %v) = %q, want %q", tc.path, tc.query, got, tc.want)
			}
		})
	}
}
