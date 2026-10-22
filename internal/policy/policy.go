// Package policy answers every who-may question Anam asks: may this caller reach this
// route, submit or register this type, open this receipt, list these receipts, retire
// this source. The daemon's handlers, the inspector and the CLI ask it, and branch on
// nothing else about who is asking (card 223).
//
// The line it holds: the policy reads the importers file's principals and the request,
// never a payload. No facet, no ledger, no store, no projection reaches it, so access
// stays a mechanism and never becomes a reading of content (Anam is a neutral layer).
// TestPolicyReadsNoPayload holds the line on the package's imports.
//
// The rules are static Go for one person, few and visible in one file. The stated flip
// is the bundled Datalog engine over a policy base (principal/2, flag/2, request/3, a
// versioned policy.dl), never attr/3, once the rules pass about a dozen, a second class
// of principal arrives, or a rule must read beyond the request and the file. The
// handlers would not change: they ask the same questions.
package policy

import (
	"slices"

	"github.com/mattjoyce/anam/internal/config"
	"github.com/mattjoyce/anam/internal/facet"
)

// Request is the part of a request a decision reads: who asks, which roles the route
// admits, and the admission hold in force. Roles come from the daemon's route table, the
// one source for which roles reach a route (mirrored in openapi.yaml), so the policy
// reads that table and keeps no copy of it. The hold is read by the daemon inside the
// write and passed in, so the policy reads nothing but the request and the file.
type Request struct {
	// Caller is the principal the request's token names. Known is false when no
	// token names one.
	Caller config.Principal
	Known  bool
	// Roles are the roles the route admits.
	Roles []config.Role
	// Hold is the admission hold in force, for a submission.
	Hold Hold
}

// Answer is a route decision.
type Answer int

// The answers. Unknown is a request no principal is named by (401); Forbidden is a
// principal whose role the route does not admit (403).
const (
	Allowed Answer = iota
	Unknown
	Forbidden
)

// Allow decides whether req reaches its route.
func Allow(req Request) Answer {
	switch {
	case !req.Known:
		return Unknown
	case !slices.Contains(req.Roles, req.Caller.Role):
		return Forbidden
	}
	return Allowed
}

// Scopes is the authenticated pillar for one principal: what it may submit, as its
// source. It answers the gate (it has the method gate.TokenScopes asks for) and the
// handlers (through MaySubmitType) from one rule, so a snapshot's declared types and the
// gate's admission cannot drift apart.
type Scopes struct {
	source string
	any    bool
	types  []facet.Type
}

// ScopesOf is p's scopes: its source for its types, or for every type with any_type.
func ScopesOf(p config.Principal) Scopes {
	return Scopes{source: p.Source, any: p.AnyType, types: p.Types}
}

// Allowed reports whether source may submit facets of type t.
func (s Scopes) Allowed(source string, t facet.Type) bool {
	return source == s.source && (s.any || slices.Contains(s.types, t))
}

// MaySubmitType reports whether p may submit facets of type t: one of its types, or any
// with any_type. For the workbench it is the allow-list of what may be entered by hand.
func MaySubmitType(p config.Principal, t facet.Type) bool {
	return ScopesOf(p).Allowed(p.Source, t)
}

// MayRegisterType reports whether p owns type t, and so may register its Definitions:
// it lists t. An any_type importer owns nothing (card 177).
func MayRegisterType(p config.Principal, t facet.Type) bool {
	return slices.Contains(p.Types, t)
}

// DocumentType is the facet type that references a document's bytes in the library.
// It is an invariant: the library takes bytes only from a source that owns the
// references to them, and that ownership is this type.
const DocumentType facet.Type = "record.document"

// MayStoreDocument reports whether p may post a document's bytes to the library: it owns
// record.document, the same test as MayRegisterType. any_type reaches every facet type
// and not the library (card 177's audit).
func MayStoreDocument(p config.Principal) bool {
	return MayRegisterType(p, DocumentType)
}
