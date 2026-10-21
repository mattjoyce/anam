package inspect

import (
	"net/http"
	"net/url"
	"sort"
)

// The default view (card 135). Views are URLs, so "make this my default" keeps the
// query in a cookie, and a bare "/" opens it. Remembering the last view
// automatically was considered and dropped: "/" would then change between visits.
// The cookie holds a query string and nothing else; the reader's zone, token and
// ledger are not in it.

// viewCookie names the cookie; viewKeys are the query keys a view may carry.
const viewCookie = "anam_view"

var viewKeys = map[string]bool{"from": true, "to": true, "type": true, "status": true, "sel": true, "selmode": true}

// cleanView keeps the known keys of a query, in a fixed order, so a stored view is
// exactly a timeline view and nothing a page could be sent to. "view=whole" has
// no keys left, and the empty string means no default.
func cleanView(raw string) string {
	q, err := url.ParseQuery(raw)
	if err != nil {
		return ""
	}
	out := url.Values{}
	for k, vs := range q {
		if !viewKeys[k] {
			continue
		}
		for _, v := range vs {
			out.Add(k, v)
		}
	}
	for k := range out {
		sort.Strings(out[k])
	}
	return out.Encode()
}

// defaultView reads the stored default, cleaned; "" when there is none.
func defaultView(r *http.Request) string {
	c, err := r.Cookie(viewCookie)
	if err != nil {
		return ""
	}
	raw, err := url.QueryUnescape(c.Value)
	if err != nil {
		return ""
	}
	return cleanView(raw)
}

// setDefaultView is POST /view: the form's "view" is the query to open at "/"; an
// empty one forgets the default. Either way the browser lands on "/" and sees the
// result.
func (s *Server) setDefaultView(w http.ResponseWriter, r *http.Request) {
	// A view is a short query; a body past 4 KiB is not one.
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	view := cleanView(r.PostForm.Get("view"))
	c := &http.Cookie{
		Name: viewCookie, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode,
		Value: url.QueryEscape(view), MaxAge: 365 * 24 * 60 * 60,
	}
	if view == "" {
		c.MaxAge = -1
	}
	http.SetCookie(w, c)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
