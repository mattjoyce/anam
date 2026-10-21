package inspect

import (
	"net/url"
	"sort"
	"strings"
)

// address writes a query as an address carries it: the path, a question mark, and
// every key and value percent-encoded, keys in order. An empty path writes the
// query alone.
//
// It is url.Values.Encode with the escaping written out, so what reaches a
// browser is visibly a path on this daemon. A query is never a place to put a URL
// of its own: the reader's own bytes cannot become a scheme, a host or a path.
func address(path string, q url.Values) string {
	keys := make([]string, 0, len(q))
	for k := range q {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	if path != "" {
		b.WriteString(path)
		b.WriteByte('?')
	}
	first := true
	for _, k := range keys {
		for _, v := range q[k] {
			if !first {
				b.WriteByte('&')
			}
			first = false
			b.WriteString(url.QueryEscape(k))
			b.WriteByte('=')
			b.WriteString(url.QueryEscape(v))
		}
	}
	return b.String()
}
