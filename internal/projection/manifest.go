package projection

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/facet"
)

// The bundled templates: small, static, versioned with the binary (AGENTS.md,
// Embedded assets). They are the floor; the operator registers others over the API
// (Register, WithRegistered), and a consumer still only selects (§9).
//
// manifests/<name>.json is each template's current version, the one a consumer
// selecting by name is answered with. manifests/history/<name>.v<n>.json is every
// earlier bundled version a receipt may name (card 147): held beside the current one,
// so a receipt cut under it still verifies against exactly the template that served it.
//
//go:embed manifests/*.json manifests/history/*.json
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
	Shape             string    `json:"shape"`
}

// Parse reads and validates one template manifest. A manifest is config, and config is
// external input: unknown keys, missing knobs and unparseable enums fail here, at
// load, rather than at the first serve.
func Parse(raw []byte) (Template, error) {
	m, err := decodeManifest(raw)
	if err != nil {
		return Template{}, err
	}
	if err := m.requireEveryKnob(); err != nil {
		return Template{}, err
	}
	return m.template(raw)
}

// InlineName and InlineVersion are what an inline template is called in a receipt and
// in an answer: it has no name or version of its own, it is known by its hash (card
// 211). They only fill the fields a Template has; the receipt's inline marker, not
// this name, says a template was inline, so a registered template may still be called
// "inline".
const (
	InlineName    = "inline"
	InlineVersion = 1
)

// ParseInline reads and validates a template a query carried in its request (card 211),
// through the same checks as Parse: a manifest without a name or a version, which an
// inline template has none of. Naming either is refused, as is a description, so the
// body is only the knobs that prune. Its hash is taken the way a registered template's
// is, over the body as canonical JSON, so the same body is the same template wherever
// it is sent from.
func ParseInline(raw []byte) (Template, error) {
	m, err := decodeManifest(raw)
	if err != nil {
		return Template{}, err
	}
	switch {
	case m.Name != nil:
		return Template{}, fmt.Errorf("%w: name: an inline projection has none; it is known by its content hash", ErrBadTemplate)
	case m.Version != nil:
		return Template{}, fmt.Errorf("%w: version: an inline projection has none; it is known by its content hash", ErrBadTemplate)
	case m.Description != "":
		return Template{}, fmt.Errorf("%w: description: an inline projection has none", ErrBadTemplate)
	}
	name, version := InlineName, InlineVersion
	m.Name, m.Version = &name, &version
	if err := m.requireEveryKnob(); err != nil {
		return Template{}, err
	}
	t, err := m.template(raw)
	if err != nil {
		return Template{}, err
	}
	t.Inline = true
	return t, nil
}

// decodeManifest reads the JSON layout, refusing a key it does not know.
func decodeManifest(raw []byte) (manifest, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var m manifest
	if err := dec.Decode(&m); err != nil {
		return manifest{}, fmt.Errorf("%w: %v", ErrBadTemplate, err)
	}
	return m, nil
}

// template forms the Template a decoded manifest describes, once its knobs are all
// stated. raw is the manifest as written; its hash is over the canonical form.
func (m manifest) template(raw []byte) (Template, error) {
	// Shape is the one knob with a safe default. A template that says nothing serves
	// the log, which is exactly what every template served before the knob existed;
	// a snapshot only ever narrows that, so absent cannot over-serve.
	if m.Shape == "" {
		m.Shape = string(ShapeLog)
	}
	canon, err := facet.Canonicalize(raw)
	if err != nil {
		return Template{}, fmt.Errorf("%w: %v", ErrBadTemplate, err)
	}
	sum := sha256.Sum256(canon.Bytes())

	t := Template{
		Hash:              "sha256:" + hex.EncodeToString(sum[:]),
		Body:              canon.Bytes(),
		Name:              *m.Name,
		Version:           *m.Version,
		Description:       m.Description,
		MaxClassification: facet.Classification(*m.MaxClassification),
		From:              m.From,
		To:                m.To,
		Shape:             Shape(m.Shape),
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
	case m.Shape != "" && !Shape(m.Shape).Valid():
		return fmt.Errorf("%w: %s: shape: %q is not log or snapshot", ErrBadTemplate, *m.Name, m.Shape)
	}
	return nil
}

// adoptFilters converts the manifest's strings into the domain's enums, refusing any
// the domain does not know.
func (t *Template) adoptFilters(types, statuses []string) error {
	for _, s := range types {
		if !facet.ValidPattern(s) {
			return fmt.Errorf("%w: %s: types: %q is not a valid namespaced facet type or type prefix", ErrBadTemplate, t.Name, s)
		}
		if facet.IsPrefixPattern(s) {
			t.Prefixes = append(t.Prefixes, s)
			continue
		}
		ft := facet.Type(s)
		t.Types = append(t.Types, ft)
		t.typeSet[ft] = true
	}
	for _, s := range statuses {
		st := facet.Status(s)
		if !st.Valid() {
			return fmt.Errorf("%w: %s: statuses: %q is not a facet status", ErrBadTemplate, t.Name, s)
		}
		t.Statuses = append(t.Statuses, st)
		t.statusSet[st] = true
	}
	if !t.MaxClassification.Valid() {
		return fmt.Errorf("%w: %s: max_classification: %q is not a classification", ErrBadTemplate, t.Name, t.MaxClassification)
	}
	return nil
}

// checkWindow requires each bound to be a fixed-width clock.Layout instant, so the
// comparison against an entry's fact-time is the lexical one the canonical is already
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
		return fmt.Errorf("%w: %s: from: %s is not before to %s", ErrBadTemplate, t.Name, t.From, t.To)
	}
	return nil
}

// Registry holds every template version held, by name: the bundled ones and those
// the operator registered. A consumer selects by name and is answered with the newest
// version; a receipt names a version and is answered with exactly that one.
type Registry struct {
	held map[string][]Template // by name, versions ascending
}

// NewRegistry collects templates. Two versions of one name are the same template at
// two points in time and both are held; the same name and version twice is one
// template when the bodies agree and a configuration error when they do not.
func NewRegistry(templates ...Template) (*Registry, error) {
	r := &Registry{held: make(map[string][]Template, len(templates))}
	for _, t := range templates {
		if err := r.add(t); err != nil {
			return nil, err
		}
	}
	return r, nil
}

func (r *Registry) add(t Template) error {
	if t.Name == "" {
		return fmt.Errorf("%w: a template must be named", ErrBadTemplate)
	}
	for _, h := range r.held[t.Name] {
		if h.Version != t.Version {
			continue
		}
		if h.Hash == t.Hash {
			return nil
		}
		return fmt.Errorf("%w: two bodies for template %s v%d", ErrBadTemplate, t.Name, t.Version)
	}
	held := append(r.held[t.Name], t)
	sort.Slice(held, func(i, j int) bool { return held[i].Version < held[j].Version })
	r.held[t.Name] = held
	return nil
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

	r := &Registry{held: make(map[string][]Template, len(entries))}
	current := make(map[string]int, len(entries)) // each bundled template's version, before history joins it
	for _, e := range entries {
		if e.IsDir() {
			continue // history, read below against the current versions
		}
		t, err := bundledFile("manifests/" + e.Name())
		if err != nil {
			return nil, err
		}
		if want := t.Name + ".json"; e.Name() != want {
			return nil, fmt.Errorf("%w: manifest %s declares name %q (file must be named %s)",
				ErrBadTemplate, e.Name(), t.Name, want)
		}
		if _, dup := r.held[t.Name]; dup {
			return nil, fmt.Errorf("%w: duplicate template %q", ErrBadTemplate, t.Name)
		}
		r.held[t.Name] = []Template{t}
		current[t.Name] = t.Version
	}
	history, err := manifestFS.ReadDir("manifests/history")
	if err != nil {
		return nil, fmt.Errorf("projection: read bundled history: %w", err)
	}
	for _, e := range history {
		t, err := bundledFile("manifests/history/" + e.Name())
		if err != nil {
			return nil, err
		}
		if want := fmt.Sprintf("%s.v%d.json", t.Name, t.Version); e.Name() != want {
			return nil, fmt.Errorf("%w: history %s declares %s v%d (file must be named %s)",
				ErrBadTemplate, e.Name(), t.Name, t.Version, want)
		}
		cur, ok := current[t.Name]
		if !ok || t.Version >= cur {
			return nil, fmt.Errorf("%w: history %s is not an earlier version of a bundled template", ErrBadTemplate, e.Name())
		}
		if err := r.add(t); err != nil {
			return nil, err
		}
	}
	return r, nil
}

// bundledFile parses one embedded manifest.
func bundledFile(path string) (Template, error) {
	raw, err := manifestFS.ReadFile(path)
	if err != nil {
		return Template{}, fmt.Errorf("projection: read bundled manifest %s: %w", path, err)
	}
	t, err := Parse(raw)
	if err != nil {
		return Template{}, fmt.Errorf("projection: bundled manifest %s: %w", path, err)
	}
	return t, nil
}

// Lookup returns the newest version of the template a consumer selected. An unknown
// name lists what is available: the set is small and in hand, so there is no reason
// to make the operator go looking.
func (r *Registry) Lookup(name string) (Template, error) {
	held, ok := r.held[name]
	if !ok {
		return Template{}, fmt.Errorf("%w: %q (available: %s)", ErrUnknown, name, strings.Join(r.Names(), ", "))
	}
	return held[len(held)-1], nil
}

// Version returns one held version of a template, the one a receipt names.
func (r *Registry) Version(name string, version int) (Template, error) {
	held, ok := r.held[name]
	if !ok {
		return Template{}, fmt.Errorf("%w: %q (available: %s)", ErrUnknown, name, strings.Join(r.Names(), ", "))
	}
	for _, t := range held {
		if t.Version == version {
			return t, nil
		}
	}
	return Template{}, fmt.Errorf("%w: %s v%d (held: %s)", ErrUnknown, name, version, versionsOf(held))
}

// Versions returns every held version of a template, oldest first; nil for an
// unknown name.
func (r *Registry) Versions(name string) []Template {
	return append([]Template(nil), r.held[name]...)
}

// Names lists the available template names, sorted.
func (r *Registry) Names() []string {
	names := make([]string, 0, len(r.held))
	for n := range r.held {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// All returns the newest version of every template, in name order.
func (r *Registry) All() []Template {
	all := make([]Template, 0, len(r.held))
	for _, n := range r.Names() {
		held := r.held[n]
		all = append(all, held[len(held)-1])
	}
	return all
}

// Held returns every version of every template, in name then version order.
func (r *Registry) Held() []Template {
	var all []Template
	for _, n := range r.Names() {
		all = append(all, r.held[n]...)
	}
	return all
}

func versionsOf(held []Template) string {
	parts := make([]string, len(held))
	for i, t := range held {
		parts[i] = fmt.Sprintf("v%d", t.Version)
	}
	return strings.Join(parts, ", ")
}

func deref(s *string) string {
	if s == nil {
		return "<unnamed>"
	}
	return *s
}
