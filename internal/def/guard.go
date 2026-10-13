package def

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"

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
// Those two rules are what a facet *is*: identity_key composes the source ref the
// facet's id derives from, and fact_time (pointer, layout, zone) places it on the
// timeline. A changed identity key would re-key every facet of the type, so the next
// ingest admits duplicates beside the originals with nothing superseding anything. A
// changed fact-time rule would leave every existing facet where the old rule put it,
// with no event able to move it: the payload is unchanged, so the gate sees a no-op.
// Neither is a version two; both are a new ledger. The schema half may change freely,
// and so may classification, assurance and series: they govern admission and reading,
// not what was admitted.
//
// A type with nothing stored is new, and passes.
func Guard(stored []Stored, bundled *Registry) error {
	newest := map[facet.Type]Stored{}
	for _, s := range stored {
		if cur, ok := newest[s.Type]; !ok || s.Version > cur.Version {
			newest[s.Type] = s
		}
	}
	types := make([]facet.Type, 0, len(bundled.byType))
	for t := range bundled.byType {
		types = append(types, t)
	}
	sort.Slice(types, func(i, j int) bool { return types[i] < types[j] })

	for _, t := range types {
		prev, ok := newest[t]
		if !ok {
			continue
		}
		old, err := keyingOf(prev.Body)
		if err != nil {
			return fmt.Errorf("def: stored definition %s v%d: %w", t, prev.Version, err)
		}
		now := bundled.byType[t]
		if err := sameKeying(old, keying{IdentityKey: now.IdentityKey, FactTime: now.FactTime}); err != nil {
			return fmt.Errorf("%w: %s v%d → v%d: %v. The ledger's %s facets were admitted under the stored rule; "+
				"a new rule needs a new ledger from the sources, not a new definition version",
				ErrKeyed, t, prev.Version, now.Version, err, t)
		}
	}
	return nil
}

// keying is the part of a Definition that says what its facets are.
type keying struct {
	IdentityKey []string
	FactTime    TimeField
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
		} `json:"meta"`
	}
	if err := json.Unmarshal(body, &m); err != nil {
		return keying{}, fmt.Errorf("%w: %v", ErrBadManifest, err)
	}
	if len(m.Meta.IdentityKey) == 0 || m.Meta.TimeSemantics.FactTime.Pointer == "" {
		return keying{}, fmt.Errorf("%w: no identity_key or fact_time", ErrBadManifest)
	}
	return keying{IdentityKey: m.Meta.IdentityKey, FactTime: m.Meta.TimeSemantics.FactTime}, nil
}

func sameKeying(old, now keying) error {
	if !equalStrings(old.IdentityKey, now.IdentityKey) {
		return fmt.Errorf("identity_key was %v, is %v", old.IdentityKey, now.IdentityKey)
	}
	if old.FactTime != now.FactTime {
		return fmt.Errorf("fact_time was %+v, is %+v", old.FactTime, now.FactTime)
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
