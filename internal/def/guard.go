package def

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/mattjoyce/anam/internal/facet"
)

// ErrKeyed is returned when a Definition would key or place facets differently from
// the version the ledger's facets were admitted under.
var ErrKeyed = errors.New("def: definition changes what existing facets are")

// Stored is a Definition as the operational store last saw it: the manifest body by
// type and version. The store keeps them so that this check has something to
// compare against.
type Stored struct {
	Type    facet.Type
	Version int
	Body    []byte
}

// Guard compares the bundled Definitions against the stored ones and refuses any
// whose identity rule or fact-time rule has changed since the newest stored version
// of that type.
//
// Those rules are what a facet *is*: identity_key and identity_template compose the
// source ref the facet's id derives from, and fact_time (pointer, layout, zone)
// places it on the timeline. A changed identity rule would re-key every facet of the
// type, so the next ingest admits duplicates beside the originals with nothing
// superseding anything. A changed fact-time rule would leave every existing facet
// where the old rule put it, with no event able to move it: the payload is unchanged,
// so the gate sees a no-op. Neither is a version two; both are a new ledger. The
// schema half may change freely, and so may classification, assurance and series:
// they govern admission and reading, not what was admitted. Two widenings are not
// changes, because they key and place every admitted facet as before (widens).
//
// A type with nothing stored is new, and passes.
func Guard(stored []Stored, bundled *Registry) error {
	held := map[facet.Type][]Stored{}
	for _, s := range stored {
		held[s.Type] = append(held[s.Type], s)
	}
	types := make([]facet.Type, 0, len(bundled.byType))
	for t := range bundled.byType {
		types = append(types, t)
	}
	sort.Slice(types, func(i, j int) bool { return types[i] < types[j] })

	for _, t := range types {
		prior, ok := held[t]
		if !ok {
			continue
		}
		now := bundled.byType[t]
		prev, err := holdsKeying(prior, keyingOfDef(now))
		if errors.Is(err, ErrBadManifest) {
			return fmt.Errorf("def: %s: %w", t, err)
		}
		if err != nil {
			return fmt.Errorf("%w: %s v%d → v%d: %v. The ledger's %s facets were admitted under the stored rule; "+
				"a new rule needs a new ledger from the sources, not a new definition version",
				ErrKeyed, t, prev, now.Version, err, t)
		}
	}
	return nil
}

// keying is the part of a Definition that says what its facets are.
type keying struct {
	IdentityKey      []string
	FactTime         TimeField
	IdentityTemplate string
}

func keyingOfDef(d Definition) keying {
	return keying{IdentityKey: d.IdentityKey, FactTime: d.FactTime, IdentityTemplate: d.IdentityTemplate}
}

// keyingOf reads only the keying from a stored manifest. A stored body is whatever
// format was current when it was stored, and the schema half's format has changed
// before (card 52), so the guard does not parse what it does not compare.
func keyingOf(body []byte) (keying, error) {
	var m struct {
		Meta struct {
			IdentityKey   []string `json:"identity_key"`
			TimeSemantics struct {
				FactTime TimeField `json:"fact_time"`
			} `json:"time_semantics"`
			IdentityTemplate string `json:"identity_template"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(body, &m); err != nil {
		return keying{}, fmt.Errorf("%w: %v", ErrBadManifest, err)
	}
	if len(m.Meta.IdentityKey) == 0 || m.Meta.TimeSemantics.FactTime.Pointer == "" {
		return keying{}, fmt.Errorf("%w: no identity_key or fact_time", ErrBadManifest)
	}
	return keying{IdentityKey: m.Meta.IdentityKey, FactTime: m.Meta.TimeSemantics.FactTime,
		IdentityTemplate: m.Meta.IdentityTemplate}, nil
}

// holdsKeying checks now against the newest of prior, every body held for the type,
// and returns that newest version for the caller's message.
func holdsKeying(prior []Stored, now keying) (int, error) {
	newest := prior[0]
	for _, s := range prior[1:] {
		if s.Version > newest.Version {
			newest = s
		}
	}
	old, err := keyingOf(newest.Body)
	if err != nil {
		return newest.Version, fmt.Errorf("stored definition v%d: %w", newest.Version, err)
	}
	return newest.Version, sameKeying(old, now, prior)
}

// sameKeying refuses now unless it keys and places facets as old does. prior is every
// body held for the type, which the two widenings are proved against.
func sameKeying(old, now keying, prior []Stored) error {
	if !equalStrings(old.IdentityKey, now.IdentityKey) {
		return fmt.Errorf("identity_key was %v, is %v", old.IdentityKey, now.IdentityKey)
	}
	if old.FactTime != now.FactTime {
		added, ok := widensFactTime(old.FactTime, now.FactTime)
		if !ok {
			return fmt.Errorf("fact_time was %+v, is %+v", old.FactTime, now.FactTime)
		}
		if err := neverHeld(prior, added); err != nil {
			return fmt.Errorf("fact_time adds %v: %v", added, err)
		}
	}
	if old.IdentityTemplate != now.IdentityTemplate {
		added, ok := widensTemplate(old.IdentityTemplate, now.IdentityTemplate)
		if !ok {
			return fmt.Errorf("identity_template was %q, is %q", old.IdentityTemplate, now.IdentityTemplate)
		}
		if err := neverHeld(prior, added); err != nil {
			return fmt.Errorf("identity_template appends %v: %v", added, err)
		}
	}
	return nil
}

// The two widenings (card 198). Each lets a later version read a field the earlier
// ones did not, and each holds only because no fact admitted before carries that
// field: the fact-time rule then falls back to the Definition's zone for every one of
// them, and the optional id segment renders nothing, so every admitted facet reads the
// same instant and composes the same id as it did. neverHeld proves "carries no such
// field" from the stored versions, not from hope.
//
//   - fact_time: pointer, layout and zone unchanged, and a zone_pointer goes from
//     absent to present. Changing or dropping it is refused: facts admitted with a
//     zone would move.
//   - identity_template: the new template is the old one with one or more optional
//     placeholders ({/pointer?}) appended, and no literal text. Any other edit, a
//     template where there was none, or none where there was one, is refused.

// widensFactTime reports whether now is old with a zone pointer added, and which.
func widensFactTime(old, now TimeField) ([]string, bool) {
	if old.Pointer != now.Pointer || old.Layout != now.Layout || old.Zone != now.Zone ||
		old.ZonePointer != "" || now.ZonePointer == "" {
		return nil, false
	}
	return []string{now.ZonePointer}, true
}

// widensTemplate reports whether now is old with only optional placeholders appended,
// and the pointers they read.
func widensTemplate(old, now string) ([]string, bool) {
	tail, ok := strings.CutPrefix(now, old)
	if old == "" || !ok || tail == "" {
		return nil, false
	}
	parts, err := parseTemplate(tail)
	if err != nil {
		return nil, false
	}
	var added []string
	for _, p := range parts {
		if !p.optional {
			return nil, false
		}
		added = append(added, p.pointer)
	}
	return added, true
}

// neverHeld refuses a widening unless no version held for the type could have
// admitted a payload with a value at one of the pointers. A strict version's schema is
// closed, so one that does not declare the pointer's top-level field refused every
// payload carrying it. A verbatim version checked nothing, so it proves nothing. A
// body in a format from before card 52 did not close its fields; it proves only that
// no payload held source_time, the field card 197 named long after that format was
// retired.
func neverHeld(prior []Stored, pointers []string) error {
	for _, s := range prior {
		var m struct {
			Meta struct {
				Mode string `json:"validation_mode"`
			} `json:"meta"`
			Schema struct {
				Properties map[string]json.RawMessage `json:"properties"`
			} `json:"schema"`
		}
		if err := json.Unmarshal(s.Body, &m); err != nil {
			return fmt.Errorf("stored definition v%d: %w: %v", s.Version, ErrBadManifest, err)
		}
		for _, p := range pointers {
			top := topToken(p)
			switch ValidationMode(m.Meta.Mode) {
			case ModeStrict:
				if _, declared := m.Schema.Properties[top]; declared {
					return fmt.Errorf("v%d already declares %s, so facets admitted under it may hold %s", s.Version, top, p)
				}
			case ModeVerbatim:
				return fmt.Errorf("v%d is verbatim, so facets admitted under it may hold %s", s.Version, p)
			default:
				if top != SourceTimeField {
					return fmt.Errorf("v%d is in a format that did not close its fields, so facets admitted under it may hold %s",
						s.Version, p)
				}
			}
		}
	}
	return nil
}

// Keying is the part of a Definition that says what its facets are: the identity key
// that composes the id and the fact-time rule that places them. Guard holds both
// across versions of one type. A rename (card 140) compares the identity key across
// two types, since a facet carried into a type keyed differently would not meet its
// own id, and reports a fact-time rule that differs (card 195).
type Keying struct {
	IdentityKey      []string
	FactTime         TimeField
	IdentityTemplate string
}

// KeyingOf reads only the keying from a stored manifest body, as Guard does, so a
// body in an older format than Parse accepts still answers.
func KeyingOf(body []byte) (Keying, error) {
	k, err := keyingOf(body)
	return Keying(k), err
}

// SameIdentity reports whether d composes a facet's id from the identity key k does;
// the error names the key that differs. A rename (card 140) needs this and no more for
// a carried facet to meet its own id. The fact-time rule may differ (card 195): it
// places a facet and does not name it, and the gate reads each carried facet's fact
// time afresh under the type it arrives at.
func (d Definition) SameIdentity(k Keying) error {
	if !equalStrings(k.IdentityKey, d.IdentityKey) {
		return fmt.Errorf("identity_key was %v, is %v", k.IdentityKey, d.IdentityKey)
	}
	return nil
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// All returns every Definition in the registry, in type order.
func (r *Registry) All() []Definition {
	out := make([]Definition, 0, len(r.byType))
	for _, d := range r.byType {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Type < out[j].Type })
	return out
}

// Manifests returns every bundled manifest's type, version and raw bytes, the current
// ones and the history (card 111), which is what the operational store keeps: Guard
// compares the newest stored against the current, and the inspector and the API read
// an old facet against the version it was admitted under.
func Manifests() ([]Stored, error) {
	entries, err := manifestFS.ReadDir("manifests")
	if err != nil {
		return nil, fmt.Errorf("def: read bundled manifests: %w", err)
	}
	out := make([]Stored, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		raw, err := manifestFS.ReadFile("manifests/" + e.Name())
		if err != nil {
			return nil, fmt.Errorf("def: read bundled manifest %s: %w", e.Name(), err)
		}
		d, err := Parse(raw)
		if err != nil {
			return nil, fmt.Errorf("def: bundled manifest %s: %w", e.Name(), err)
		}
		out = append(out, Stored{Type: d.Type, Version: d.Version, Body: raw})
	}
	history, err := History()
	if err != nil {
		return nil, err
	}
	return append(out, history...), nil
}

// History returns every earlier Definition version the binary carries under
// manifests/history, as manifests/history/<type>.v<n>.json. A history body is the
// words a facet was admitted under, kept verbatim: it is checked to be JSON whose
// meta names the type and version its file name says, and not parsed as a contract,
// since the lint of today need not have held then.
func History() ([]Stored, error) {
	entries, err := manifestFS.ReadDir("manifests/history")
	if err != nil {
		return nil, fmt.Errorf("def: read definition history: %w", err)
	}
	out := make([]Stored, 0, len(entries))
	for _, e := range entries {
		raw, err := manifestFS.ReadFile("manifests/history/" + e.Name())
		if err != nil {
			return nil, fmt.Errorf("def: read definition history %s: %w", e.Name(), err)
		}
		var m struct {
			Meta struct {
				Type    facet.Type `json:"facet_type"`
				Version int        `json:"definition_version"`
			} `json:"meta"`
		}
		if err := json.Unmarshal(raw, &m); err != nil {
			return nil, fmt.Errorf("%w: history %s: %v", ErrBadManifest, e.Name(), err)
		}
		if want := fmt.Sprintf("%s.v%d.json", m.Meta.Type, m.Meta.Version); e.Name() != want || m.Meta.Version < 1 {
			return nil, fmt.Errorf("%w: history %s declares %s v%d (file must be named %s)",
				ErrBadManifest, e.Name(), m.Meta.Type, m.Meta.Version, want)
		}
		out = append(out, Stored{Type: m.Meta.Type, Version: m.Meta.Version, Body: raw})
	}
	return out, nil
}
