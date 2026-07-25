package projection

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/facet"
)

// The bundled templates: small, static, versioned with the binary (AGENTS.md,
// Embedded assets). A consumer selects a projection and never defines one (§9), so
// there is nowhere for a template to come from at runtime — and that is the point,
// not a limitation waiting to be lifted.
//
//go:embed manifests/*.json
var manifestFS embed.FS

// manifest mirrors the JSON layout. The required knobs are pointers so that "absent"
// and "empty" stay distinguishable — see requireEveryKnob.
type manifest struct {
	Name              *string   `json:"name"`
	Version           *int      `json:"version"`
	Description       string    `json:"description"`
	Types             *[]string `json:"types"`
	Statuses          *[]string `json:"statuses"`
	MaxClassification *string   `json:"max_classification"`
	From              string    `json:"from"`
	To                string    `json:"to"`
}

// Parse reads and validates one template manifest. A manifest is config, and config is
// external input: unknown keys, missing knobs and unparseable enums fail here, at
// load, rather than at the first serve.
func Parse(raw []byte) (Template, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var m manifest
	if err := dec.Decode(&m); err != nil {
		return Template{}, fmt.Errorf("%w: %v", ErrBadTemplate, err)
	}
	if err := m.requireEveryKnob(); err != nil {
		return Template{}, err
	}

	t := Template{
		Name:              *m.Name,
		Version:           *m.Version,
		Description:       m.Description,
		MaxClassification: facet.Classification(*m.MaxClassification),
		From:              m.From,
		To:                m.To,
		typeSet:           map[facet.Type]bool{},
		statusSet:         map[facet.Status]bool{},
	}
	if err := t.adoptFilters(*m.Types, *m.Statuses); err != nil {
		return Template{}, err
	}
	if err := t.checkWindow(); err != nil {
		return Template{}, err
	}
	return t, nil
}

// requireEveryKnob rejects a manifest that leaves a knob unstated. Absent and empty are
// different: an absent statuses list is an oversight, an empty one is a template that
// deliberately serves nothing, and only the second is allowed to be silent about why.
func (m manifest) requireEveryKnob() error {
	switch {
	case m.Name == nil || *m.Name == "":
		return fmt.Errorf("%w: name is required", ErrBadTemplate)
	case m.Version == nil || *m.Version < 1:
		return fmt.Errorf("%w: %s: version must be a positive integer", ErrBadTemplate, deref(m.Name))
	case m.Types == nil:
		return fmt.Errorf("%w: %s: types is required (an empty list serves nothing, and must say so)", ErrBadTemplate, *m.Name)
	case m.Statuses == nil:
		return fmt.Errorf("%w: %s: statuses is required", ErrBadTemplate, *m.Name)
	case m.MaxClassification == nil:
		return fmt.Errorf("%w: %s: max_classification is required", ErrBadTemplate, *m.Name)
	}
	return nil
}

// adoptFilters converts the manifest's strings into the domain's enums, refusing any
// the domain does not know.
func (t *Template) adoptFilters(types, statuses []string) error {
	for _, s := range types {
		ft := facet.Type(s)
		if !ft.Valid() {
			return fmt.Errorf("%w: %s: %q is not a valid namespaced facet type", ErrBadTemplate, t.Name, s)
		}
		t.Types = append(t.Types, ft)
		t.typeSet[ft] = true
	}
	for _, s := range statuses {
		st := facet.Status(s)
		if !st.Valid() {
			return fmt.Errorf("%w: %s: %q is not a facet status", ErrBadTemplate, t.Name, s)
		}
		t.Statuses = append(t.Statuses, st)
		t.statusSet[st] = true
	}
	if !t.MaxClassification.Valid() {
		return fmt.Errorf("%w: %s: max_classification %q", ErrBadTemplate, t.Name, t.MaxClassification)
	}
	return nil
}

// checkWindow requires each bound to be a fixed-width clock.Layout instant, so the
// comparison against an entry's event-time is the lexical one the canonical is already
// ordered by, and requires the window to be non-empty.
func (t Template) checkWindow() error {
	for label, v := range map[string]string{"from": t.From, "to": t.To} {
		if v == "" {
			continue
		}
		if _, err := clock.Parse(v); err != nil {
			return fmt.Errorf("%w: %s: %s: %v", ErrBadTemplate, t.Name, label, err)
		}
	}
	if t.From != "" && t.To != "" && t.From >= t.To {
		return fmt.Errorf("%w: %s: from %s is not before to %s", ErrBadTemplate, t.Name, t.From, t.To)
	}
	return nil
}

// Registry holds the loaded templates a consumer selects from.
type Registry struct {
	byName map[string]Template
}

// NewRegistry collects templates, rejecting duplicate names: two templates called the
// same thing is a configuration error, not a precedence question. Bundled is the only
// caller in production; this exists so a test can stand up an alternative set — for
// instance, one whose bp is a later version — without reaching into the package.
func NewRegistry(templates ...Template) (*Registry, error) {
	r := &Registry{byName: make(map[string]Template, len(templates))}
	for _, t := range templates {
		if t.Name == "" {
			return nil, fmt.Errorf("%w: a template must be named", ErrBadTemplate)
		}
		if _, dup := r.byName[t.Name]; dup {
			return nil, fmt.Errorf("%w: duplicate template %q", ErrBadTemplate, t.Name)
		}
		r.byName[t.Name] = t
	}
	return r, nil
}

// Bundled parses every embedded manifest into a Registry. Like the definition set, it
// fails on any bad manifest rather than loading the rest: a binary that silently
// dropped a projection would answer "unknown projection" for a name that is in the
// source tree, which is the confusing failure, not the safe one.
func Bundled() (*Registry, error) {
	entries, err := manifestFS.ReadDir("manifests")
	if err != nil {
		return nil, fmt.Errorf("projection: read bundled manifests: %w", err)
	}

	r := &Registry{byName: make(map[string]Template, len(entries))}
	for _, e := range entries {
		raw, err := manifestFS.ReadFile("manifests/" + e.Name())
		if err != nil {
			return nil, fmt.Errorf("projection: read bundled manifest %s: %w", e.Name(), err)
		}
		t, err := Parse(raw)
		if err != nil {
			return nil, fmt.Errorf("projection: bundled manifest %s: %w", e.Name(), err)
		}
		if want := t.Name + ".json"; e.Name() != want {
			return nil, fmt.Errorf("%w: manifest %s declares name %q (file must be named %s)",
				ErrBadTemplate, e.Name(), t.Name, want)
		}
		if _, dup := r.byName[t.Name]; dup {
			return nil, fmt.Errorf("%w: duplicate template %q", ErrBadTemplate, t.Name)
		}
		r.byName[t.Name] = t
	}
	return r, nil
}

// Lookup returns the template a consumer selected. An unknown name lists what is
// available: the set is small, closed and shipped with the binary, so the answer to
// "which ones are there?" is always in hand and there is no reason to make the
// operator go looking.
func (r *Registry) Lookup(name string) (Template, error) {
	t, ok := r.byName[name]
	if !ok {
		return Template{}, fmt.Errorf("%w: %q (available: %s)", ErrUnknown, name, strings.Join(r.Names(), ", "))
	}
	return t, nil
}

// Names lists the available template names, sorted.
func (r *Registry) Names() []string {
	names := make([]string, 0, len(r.byName))
	for n := range r.byName {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// All returns every template, in name order.
func (r *Registry) All() []Template {
	all := make([]Template, 0, len(r.byName))
	for _, n := range r.Names() {
		all = append(all, r.byName[n])
	}
	return all
}

func deref(s *string) string {
	if s == nil {
		return "<unnamed>"
	}
	return *s
}
