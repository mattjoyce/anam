package daemon

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
)

// ErrForeignOrigin is returned when a browser on another site made an unsafe request.
// Loopback binding says where the daemon listens, not which page caused a browser to
// send something there; a token the browser cached for basic auth, or no token at all
// on loopback, is an ambient credential a foreign page could spend (card 108).
var ErrForeignOrigin = errors.New("daemon: request comes from a browser on another site")

// browserGuard refuses, before any work is queued or written, an unsafe request that
// a browser made from another site. Requests without a browser's fetch metadata (the
// CLI, importers, curl) carry no Origin and no Sec-Fetch-Site, and pass; the
// inspector's own forms are same-origin, and pass; anything cross-site is 403.
func (d *Daemon) browserGuard(next http.Handler) http.Handler {
	tokenless := d.token == nil
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := foreignBrowserRequest(r, tokenless); err != nil {
			writeError(w, http.StatusForbidden, err)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// foreignBrowserRequest decides whether r is an unsafe request from a browser on
// another site. Reads are left alone: the same-origin policy keeps a foreign page
// from reading the answer, and a GET does nothing. For the rest, the browser's own
// word is taken: Sec-Fetch-Site other than same-origin or none, or an Origin whose
// host is not this host, is foreign. Without a token the daemon is on loopback, and
// a Host that is not a loopback name came through a DNS name pointed at this machine
// (rebinding) or a proxy, so it is refused too.
func foreignBrowserRequest(r *http.Request, tokenless bool) error {
	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return nil
	}
	if tokenless && !loopbackHost(r.Host) {
		return fmt.Errorf("%w: host %q is not a loopback name on a tokenless daemon", ErrForeignOrigin, r.Host)
	}
	if site := r.Header.Get("Sec-Fetch-Site"); site != "" && site != "same-origin" && site != "none" {
		return fmt.Errorf("%w: Sec-Fetch-Site %s", ErrForeignOrigin, site)
	}
	origin := r.Header.Get("Origin")
	if origin == "" {
		return nil
	}
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" || !strings.EqualFold(u.Host, r.Host) {
		return fmt.Errorf("%w: Origin %q, host %q", ErrForeignOrigin, origin, r.Host)
	}
	return nil
}

// loopbackHost reports whether a Host header names this machine by a loopback name,
// with or without a port.
func loopbackHost(host string) bool {
	h := host
	if hp, _, err := net.SplitHostPort(host); err == nil {
		h = hp
	}
	h = strings.Trim(h, "[]")
	if strings.EqualFold(h, "localhost") {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}
