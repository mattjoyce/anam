package daemon

import (
	"context"
	"sort"

	"github.com/mattjoyce/anam/internal/inspect"
)

// rosterView is the daemon seen from the Workbench's importers page (card 228): who it
// admits, its route table, and what GET /v1/status knows. It reads what the daemon
// loaded at Open and the sources it has seen, and adds no source of truth.
type rosterView struct{ d *Daemon }

// Principals are the principals the daemon admits, by name: the operator and the
// importers file's tokened principals with their stored hashes, then the Workbench's,
// which has no token. A tokenless daemon has no operator entry, since its operator is
// whoever reaches it on loopback.
func (v rosterView) Principals() []inspect.RosterEntry {
	var out []inspect.RosterEntry
	for hash, p := range v.d.principals {
		out = append(out, inspect.RosterEntry{Principal: p, TokenSHA256: hash})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Principal.Name < out[j].Principal.Name })
	if v.d.workbench != nil {
		out = append(out, inspect.RosterEntry{Principal: *v.d.workbench})
	}
	return out
}

// Routes are the daemon's routes and the roles each admits.
func (v rosterView) Routes() []inspect.RouteRoles {
	table := v.d.routeTable(nil)
	out := make([]inspect.RouteRoles, len(table))
	for i, rt := range table {
		out[i] = inspect.RouteRoles{Pattern: rt.pattern, Roles: rt.roles}
	}
	return out
}

// Sources are what GET /v1/status answers for the sources and the hold.
func (v rosterView) Sources(ctx context.Context) (inspect.Seen, error) {
	held, err := heldNames(ctx, v.d.readOps)
	if err != nil {
		return inspect.Seen{}, err
	}
	return inspect.Seen{Sources: v.d.sources.statuses(), Held: held}, nil
}
