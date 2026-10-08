package daemon

import (
	"crypto/subtle"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/mattjoyce/anam/internal/safepath"
)

// ErrWeakToken is returned by Open for a token file whose token is too short to
// stand in for a password.
var ErrWeakToken = errors.New("daemon: token is shorter than 32 characters")

// minTokenLen is 32 characters: `openssl rand -hex 16` and up.
const minTokenLen = 32

// readToken reads the bearer token from path: the file's contents, trimmed. It is
// never logged.
func readToken(path string) ([]byte, error) {
	f, err := safepath.Open(path)
	if err != nil {
		return nil, fmt.Errorf("daemon: token file: %w", err)
	}
	defer func() { _ = f.Close() }()
	b, err := io.ReadAll(io.LimitReader(f, 4096))
	if err != nil {
		return nil, fmt.Errorf("daemon: read token file: %w", err)
	}
	tok := []byte(strings.TrimSpace(string(b)))
	if len(tok) < minTokenLen {
		return nil, fmt.Errorf("%w (token_file %s)", ErrWeakToken, path)
	}
	return tok, nil
}

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
