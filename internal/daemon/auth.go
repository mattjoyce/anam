package daemon

import (
	"crypto/subtle"
	"fmt"
	"net"
	"net/http"
	"strings"
)

// authorize lets a request through when it presents the token, as a bearer token or
// as a basic-auth password (any user name). Basic is accepted so a browser can open
// the inspector: it prompts for it, and it cannot send a bearer header. With no token
// configured everything passes, which Open allows only on loopback.
func (d *Daemon) authorize(next http.Handler) http.Handler {
	if d.token == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if subtle.ConstantTimeCompare(presented(r), d.token) != 1 {
			w.Header().Set("WWW-Authenticate", `Basic realm="anam"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func presented(r *http.Request) []byte {
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return []byte(strings.TrimPrefix(h, "Bearer "))
	}
	if _, pass, ok := r.BasicAuth(); ok {
		return []byte(pass)
	}
	return nil
}

// loopbackOnly refuses any listen address that a host other than this one can reach.
// An empty host means every interface, so it is refused too.
func loopbackOnly(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("daemon: listen %q: %w", addr, err)
	}
	if host == "localhost" {
		return nil
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return nil
	}
	return fmt.Errorf("%w: %q (set token_file to listen beyond this host)", ErrNotLoopback, addr)
}
