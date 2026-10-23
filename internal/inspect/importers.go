package inspect

import (
	"context"
	"net/http"
	"slices"
	"strings"

	"github.com/mattjoyce/anam/internal/api"
	"github.com/mattjoyce/anam/internal/config"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/policy"
)

// The importers page (card 228): who the daemon admits, and what the policy package
// answers for each. Every rule on it is read by calling internal/policy for the
// principal in front of it, so the page shows what the daemon enforces and holds no
// copy of the importers file. The page is the operator's like every other, since the
// daemon serves the inspector to the operator's token alone.

// Roster is what the importers page needs from the daemon, declared here where it is
// used. Principals are who the daemon admits, Routes its route table (the policy reads
// it for which roles reach a route), and Sources what GET /v1/status knows.
type Roster interface {
	Principals() []RosterEntry
	Routes() []RouteRoles
	Sources(ctx context.Context) (Seen, error)
}

// RosterEntry is one principal and the hex SHA-256 of its token, empty for one that has
// none (the Workbench's). The page shows the first redactedHashLen characters of the
// hash and nothing more of it.
type RosterEntry struct {
	Principal   config.Principal
	TokenSHA256 string
}

// RouteRoles is one pattern of the daemon's route table and the roles whose tokens open it.
type RouteRoles struct {
	Pattern string
	Roles   []config.Role
}

// Seen is what GET /v1/status knows of the importers: each source's last batch since the
// daemon started, and the sources held ("*" for every source).
type Seen struct {
	Sources []api.SourceStatus
	Held    []string
}

// redactedHashLen is how much of a token's stored hash the page shows. It is an
// invariant: eight hex characters match a token file by hashing it, and a longer prefix
// would narrow a guess at the token for nobody's benefit.
const redactedHashLen = 8

// redact is the first redactedHashLen characters of a stored token hash, in lower
// case, or "" when hash is not a hex digest that long. Nothing but this reaches the page's data.
func redact(hash string) string {
	if len(hash) < redactedHashLen {
		return ""
	}
	prefix := strings.ToLower(hash[:redactedHashLen])
	for _, c := range prefix {
		if !strings.ContainsRune("0123456789abcdef", c) {
			return ""
		}
	}
	return prefix
}

// principalRow is one principal as the page shows it. It holds the redacted token and
// the policy's answers, never the hash or the Principal itself.
type principalRow struct {
	Name, Role, Source string
	Token              string
	NoToken            bool
	Types              []facet.Type
	AnyType            bool
	Records            bool
	Routes             []string
	Registers          []facet.Type
	StoresDocuments    bool
	Lists              string
	Opens              string
	Holdable           bool
	HeldNow            bool
	Seen               bool
	LastScan           string
	LastAdmission      string
}

func (s *Server) importersPage(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), s.timeout)
	defer cancel()
	seen, err := s.roster.Sources(ctx)
	if err != nil {
		httpError(w, err)
		return
	}
	routes := s.roster.Routes()
	var rows []principalRow
	for _, e := range s.roster.Principals() {
		rows = append(rows, rowOf(e, routes, seen))
	}
	s.render(w, "importers.html", struct{ Principals []principalRow }{rows})
}

// rowOf asks the policy what e's principal may do. The routes it may call are those the
// policy's Allow admits for it against the daemon's route table; a principal with no
// token calls nothing, since no request authenticates as it.
func rowOf(e RosterEntry, routes []RouteRoles, seen Seen) principalRow {
	p := e.Principal
	row := principalRow{
		Name: p.Name, Role: string(p.Role), Source: p.Source, Token: redact(e.TokenSHA256),
		NoToken: e.TokenSHA256 == "", AnyType: p.AnyType, Records: policy.Records(p),
		StoresDocuments: policy.MayStoreDocument(p), Holdable: policy.Holdable(p),
	}
	for _, t := range p.Types {
		if policy.MaySubmitType(p, t) {
			row.Types = append(row.Types, t)
		}
		if policy.MayRegisterType(p, t) {
			row.Registers = append(row.Registers, t)
		}
	}
	if !row.NoToken {
		for _, rt := range routes {
			if policy.Allow(policy.Request{Caller: p, Known: true, Roles: rt.Roles}) == policy.Allowed {
				row.Routes = append(row.Routes, routeName(rt.Pattern))
			}
		}
	}
	row.Lists, row.Opens = listsAndOpens(p)
	row.HeldNow = policy.Held(policy.Request{Caller: p, Hold: holdOf(seen.Held)})
	for _, st := range seen.Sources {
		if p.Source != "" && st.Source == p.Source {
			row.Seen, row.LastScan, row.LastAdmission = true, st.LastScanAt, st.LastAdmissionAt
		}
	}
	return row
}

// routeName is a route pattern as the page names it: the catch-all is the Workbench's pages.
func routeName(pattern string) string {
	if pattern == "/" {
		return "/ (the Workbench pages)"
	}
	return pattern
}

// holdOf is the admission hold GET /v1/status reports, as the policy reads it.
func holdOf(held []string) policy.Hold {
	h := policy.Hold{All: slices.Contains(held, "*")}
	for _, s := range held {
		if s != "*" {
			h.Sources = append(h.Sources, s)
		}
	}
	return h
}

// listsAndOpens says in words what p's lists show and which receipts it opens, from the
// policy's ListScope and OpensAnyReceipt.
func listsAndOpens(p config.Principal) (lists, opens string) {
	sc := policy.ListScope(p)
	lists = "every receipt"
	if sc.Reader != "" {
		lists = "receipts naming " + sc.Reader
	}
	lists += ", "
	if sc.Source != "" {
		lists += "snapshots of " + sc.Source
	} else {
		lists += "every source's snapshots"
	}
	opens = "any receipt"
	if !policy.OpensAnyReceipt(p) {
		opens = "only receipts naming it"
	}
	return lists, opens
}
