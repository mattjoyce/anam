// Package presentation compiles deployment preferences into immutable inspector values.
// It has no IO and knows nothing about catalog identity or ledger state.
package presentation

import (
	"fmt"
	"slices"
	"strings"

	"github.com/mattjoyce/anam/internal/facet"
)

// Spec is the optional inspector configuration.
type Spec struct {
	TrackType facet.Type   `yaml:"track_type"`
	Domains   []DomainSpec `yaml:"domains"`
	Families  []Family     `yaml:"families"`
	Types     []Override   `yaml:"types"`
}

// DomainSpec names an ordered domain with an optional palette slot.
type DomainSpec struct {
	ID  string `yaml:"id"`
	Hue *int   `yaml:"hue"`
}

// Domain is a compiled domain. Hue is -1 for neutral, or 0..7.
type Domain struct {
	ID  string
	Hue int
}

// Family names an ordered family and its label.
type Family struct {
	ID    string `yaml:"id"`
	Label string `yaml:"label"`
}

// Override supplies a shape and optional historical metadata for one exact type.
type Override struct {
	Type         facet.Type `yaml:"type"`
	Shape        string     `yaml:"shape"`
	LegacyKind   string     `yaml:"legacy_kind"`
	LegacyFamily string     `yaml:"legacy_family"`
}

// Error identifies an invalid effective setting without containing its value.
type Error struct {
	Pointer string
	Message string
}

func (e *Error) Error() string { return e.Pointer + ": " + e.Message }
func invalid(section string, i int, key, message string) error {
	return &Error{fmt.Sprintf("/inspector/%s/%d/%s", section, i, key), message}
}

// Value is immutable. Its zero value gives neutral hues and no preferred ordering.
type Value struct {
	trackType facet.Type
	domains   []Domain
	families  []Family
	types     []Override
}

// Compile validates the whole spec and copies its data. Errors return no partial value.
func Compile(s Spec) (Value, error) {
	var v Value
	if s.TrackType != "" && !s.TrackType.Valid() {
		return Value{}, &Error{"/inspector/track_type", "invalid type"}
	}
	v.trackType = s.TrackType
	seen := map[string]bool{}
	for i, d := range s.Domains {
		if !identifier(d.ID) || seen[d.ID] {
			return Value{}, invalid("domains", i, "id", "invalid or duplicate identifier")
		}
		seen[d.ID] = true
		hue := -1
		if d.Hue != nil {
			hue = *d.Hue
		}
		if hue < -1 || hue > 7 {
			return Value{}, invalid("domains", i, "hue", "want -1 or 0..7")
		}
		v.domains = append(v.domains, Domain{d.ID, hue})
	}
	seen = map[string]bool{}
	for i, f := range s.Families {
		if !identifier(f.ID) || f.ID == "other" || seen[f.ID] {
			return Value{}, invalid("families", i, "id", "invalid, reserved or duplicate identifier")
		}
		seen[f.ID] = true
		if f.Label == "" {
			f.Label = f.ID
		}
		if strings.TrimSpace(f.Label) == "" {
			return Value{}, invalid("families", i, "label", "want nonblank label")
		}
		v.families = append(v.families, f)
	}
	seen = map[string]bool{}
	for i, o := range s.Types {
		if !o.Type.Valid() || seen[string(o.Type)] {
			return Value{}, invalid("types", i, "type", "invalid or duplicate type")
		}
		seen[string(o.Type)] = true
		if !slices.Contains([]string{"circle", "square", "diamond", "tri-up", "tri-down", "cross", "star", "alert"}, o.Shape) {
			return Value{}, invalid("types", i, "shape", "unknown shape")
		}
		if o.LegacyKind != "" && !slices.Contains([]string{"measure", "reading", "event"}, o.LegacyKind) {
			return Value{}, invalid("types", i, "legacy_kind", "unknown kind")
		}
		if o.LegacyFamily != "" && (o.LegacyKind == "" || !identifier(o.LegacyFamily) || o.LegacyFamily == "other") {
			return Value{}, invalid("types", i, "legacy_family", "want identifier and legacy_kind; other is reserved")
		}
		v.types = append(v.types, o)
	}
	return v, nil
}

func identifier(s string) bool {
	if s == "" || s[0] == '-' || s[len(s)-1] == '-' {
		return false
	}
	for _, c := range s {
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '-' {
			return false
		}
	}
	return true
}

// Domains returns the configured domain order and hues, as a copy.
func (v Value) Domains() []Domain { return slices.Clone(v.domains) }

// Families returns the configured family order and labels, as a copy.
func (v Value) Families() []Family { return slices.Clone(v.families) }

// Overrides returns the complete override set, as a copy.
func (v Value) Overrides() []Override { return slices.Clone(v.types) }

// Lookup returns an exact type override, if configured.
func (v Value) Lookup(t facet.Type) (Override, bool) {
	for _, o := range v.types {
		if o.Type == t {
			return o, true
		}
	}
	return Override{}, false
}

// Hue returns a domain's CSS palette class. Unconfigured domains remain neutral.
func (v Value) Hue(id string) string {
	for _, d := range v.domains {
		if d.ID == id && d.Hue >= 0 {
			return fmt.Sprintf("h%d", d.Hue)
		}
	}
	return "hx"
}

// TrackType returns the selected hourly track type, or empty when disabled.
func (v Value) TrackType() facet.Type { return v.trackType }
