package daemon

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/mattjoyce/anam/internal/config"
	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/policy"
)

// principals are the clients the daemon admits, by the hex SHA-256 of their token:
// the operator from token_file, and everyone the importers file names. The daemon
// never holds a client's token.
type principals map[string]config.Principal

// roster is who the daemon admits, read once at Open: the principals by token hash, the
// workbench, and the importers file's whole list, which the policy reads for what the
// file declares.
type roster struct {
	byToken   principals
	workbench *config.Principal
	file      []config.Principal
}

// loadPrincipals reads the importers file at path, and adds the operator whose token
// is operator, when there is one. A principal in the file with the operator's token
// is refused: that token would both run the daemon and act as the principal.
//
// The workbench principal, when the file declares one, is returned apart and is not in
// the map: it has no token, so no request authenticates as it. The daemon submits as it
// for the operator's entries (POST /entry), which is how an importer's token never
// reaches a browser.
func loadPrincipals(path string, operator []byte, provisioned *def.Registry) (roster, error) {
	out := roster{byToken: principals{}}
	if operator != nil {
		out.byToken[hashOf(operator)] = config.Principal{Name: config.OperatorName, Role: config.RoleOperator}
	}
	if path == "" {
		return out, nil
	}
	list, err := config.LoadPrincipals(path, provisioned.Has)
	if err != nil {
		return roster{}, fmt.Errorf("daemon: %w", err)
	}
	out.file = list
	for _, p := range list {
		if p.Source == config.WorkbenchSource {
			wb := p
			out.workbench = &wb
			continue
		}
		if _, taken := out.byToken[p.TokenSHA256]; taken {
			return roster{}, fmt.Errorf("daemon: %w: %s has the operator's token", config.ErrInvalidImporters, p.Name)
		}
		out.byToken[p.TokenSHA256] = p
	}
	return out, nil
}

func hashOf(token []byte) string {
	sum := sha256.Sum256(token)
	return hex.EncodeToString(sum[:])
}

// who is the principal whose token the request presents. A map lookup by the token's
// hash leaks nothing an attacker could use: matching a hash means knowing a preimage.
// Without a token_file the daemon is on loopback and a request with no known token
// is the operator's.
func (d *Daemon) who(r *http.Request) (config.Principal, bool) {
	if p := presented(r); len(p) > 0 {
		if who, ok := d.principals[hashOf(p)]; ok {
			return who, true
		}
	}
	if d.tokenless {
		return config.Principal{Name: config.OperatorName, Role: config.RoleOperator}, true
	}
	return config.Principal{}, false
}

// admit lets a request through when the policy allows it, with the principal in its
// context: 401 for no token or one the daemon does not know, 403 for a token whose role
// the route does not admit. roles are the route table's, which the policy reads.
func (d *Daemon) admit(roles []config.Role, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		who, ok := d.who(r)
		switch policy.Allow(policy.Request{Caller: who, Known: ok, Roles: roles}) {
		case policy.Unknown:
			// Basic, so a browser opening the inspector prompts for the token.
			w.Header().Set("WWW-Authenticate", `Basic realm="anam"`)
			writeError(w, http.StatusUnauthorized, fmt.Errorf("unauthorized"))
		case policy.Forbidden:
			writeError(w, http.StatusForbidden, fmt.Errorf("%s holds role %s, and %s %s admits %s",
				who.Name, who.Role, r.Method, r.URL.Path, rolesText(roles)))
		default:
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), principalKey{}, who)))
		}
	})
}

func rolesText(roles []config.Role) string {
	s := make([]string, len(roles))
	for i, r := range roles {
		s[i] = string(r)
	}
	return strings.Join(s, " or ")
}

type principalKey struct{}

// principalOf is the principal admit let through.
func principalOf(r *http.Request) config.Principal {
	p, _ := r.Context().Value(principalKey{}).(config.Principal)
	return p
}

// presented is the token a request carries, as a bearer token or as a basic-auth
// password (any user name). Basic is accepted so a browser can open the inspector:
// it prompts for it, and it cannot send a bearer header.
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
