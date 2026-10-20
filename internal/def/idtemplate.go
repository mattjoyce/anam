package def

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// A type's id rule, declared (card 188, S1).
//
// Most types let a writer choose the identity field's text, and say how in
// `handling.guidance` prose. A form, a chat writer and an interpreter cannot all
// read prose, and two statements of one rule drift until one dose is two facets.
// A Definition may instead declare `meta.identity_template`: the one rule, in the
// version, that every writer builds the id from (ComposeID) and the gate checks.
//
// Grammar. A template is literal text with placeholders in braces:
//
//	{source}              the envelope's source, as the token names it
//	{/pointer}            the scalar at a JSON Pointer in the payload
//	{/pointer|slug}       the same, lower-cased, with each run of characters outside
//	                      a-z and 0-9 turned into one hyphen and the ends trimmed
//	{/pointer|<layout>}   a time field, read in the fact-time layout and zone and
//	                      written in the Go time layout given; allowed only on the
//	                      fact-time pointer
//
// So `{source}:med/{/drug|slug}/dose/{/taken_at|2006-01-02T15:04}` composes
// `conversation:med/melatonin/dose/2026-10-17T22:00`. There is no escape: a brace is
// always a placeholder, and a pointer or layout may not hold `{`, `}` or `|`.
//
// The template composes the value of the type's one identity_key field, which must
// be a top-level field the template does not read. The composed text is the whole
// id, so the source prefix is part of it; the gate then composes source_ref from
// identity_key as for any type.

// ErrIdentityField is returned when a payload's identity field disagrees with what
// the Definition's identity_template composes from the rest of it. The message
// shows both.
var ErrIdentityField = errors.New("def: identity field disagrees with identity_template")

// ErrNoTemplate is returned by ComposeID for a Definition that declares no
// identity_template.
var ErrNoTemplate = errors.New("def: definition declares no identity_template")

// idPart is one piece of a parsed template: literal text, the source, or a pointer
// with its transform.
type idPart struct {
	lit       string
	source    bool
	pointer   string
	slug      bool
	timeStyle string // the output layout; "" when none
}

// parseTemplate splits a template into parts. It checks the shape only; what the
// pointers must be against the Definition is checkTemplate's.
func parseTemplate(tpl string) ([]idPart, error) {
	var parts []idPart
	rest := tpl
	for rest != "" {
		open := strings.IndexAny(rest, "{}")
		if open < 0 {
			parts = append(parts, idPart{lit: rest})
			break
		}
		if rest[open] == '}' {
			return nil, fmt.Errorf("a '}' with no '{' before it")
		}
		if open > 0 {
			parts = append(parts, idPart{lit: rest[:open]})
		}
		end := strings.IndexByte(rest[open:], '}')
		if end < 0 {
			return nil, fmt.Errorf("a '{' is never closed")
		}
		body := rest[open+1 : open+end]
		if strings.ContainsRune(body, '{') {
			return nil, fmt.Errorf("a '{' inside a placeholder: {%s}", body)
		}
		p, err := parsePlaceholder(body)
		if err != nil {
			return nil, err
		}
		parts = append(parts, p)
		rest = rest[open+end+1:]
	}
	return parts, nil
}

func parsePlaceholder(body string) (idPart, error) {
	ptr, transform, hasTransform := strings.Cut(body, "|")
	if strings.Contains(transform, "|") {
		return idPart{}, fmt.Errorf("{%s} has more than one transform", body)
	}
	switch {
	case ptr == "source" && !hasTransform:
		return idPart{source: true}, nil
	case ptr == "source":
		return idPart{}, fmt.Errorf("{source} takes no transform")
	}
	if err := checkPointerSyntax(ptr); err != nil {
		return idPart{}, fmt.Errorf("{%s}: %v", body, err)
	}
	p := idPart{pointer: ptr}
	switch {
	case !hasTransform:
	case transform == "slug":
		p.slug = true
	case layoutShowsTime(transform):
		p.timeStyle = transform
	default:
		return idPart{}, fmt.Errorf("{%s}: %q is neither slug nor a time layout", body, transform)
	}
	return p, nil
}

// layoutShowsTime says a string is a Go time layout rather than a word: formatting
// an instant with it changes it. The instant is not Go's own reference time, which
// every layout formats to itself.
func layoutShowsTime(layout string) bool {
	return layout != "" && time.Date(2001, 2, 3, 16, 5, 6, 0, time.UTC).Format(layout) != layout
}

// checkTemplate holds a parsed template to the Definition it sits in. The identity
// field must be one top-level pointer, so the gate can fill it, and the template
// must read other fields and not itself; a time layout belongs to the fact-time
// pointer, the one time field whose input layout the Definition declares.
func (d Definition) checkTemplate(parts []idPart) error {
	if len(d.IdentityKey) != 1 || strings.Count(d.IdentityKey[0], "/") != 1 {
		return fmt.Errorf("identity_template needs identity_key to be one top-level pointer, not %v", d.IdentityKey)
	}
	reads := 0
	for _, p := range parts {
		if p.pointer == "" {
			continue
		}
		reads++
		if p.pointer == d.IdentityKey[0] {
			return fmt.Errorf("identity_template reads %s, the field it composes", p.pointer)
		}
		if p.timeStyle != "" && p.pointer != d.FactTime.Pointer {
			return fmt.Errorf("identity_template formats %s as a time, but only the fact-time field %s has a declared layout",
				p.pointer, d.FactTime.Pointer)
		}
	}
	if reads == 0 {
		return fmt.Errorf("identity_template reads no payload field, so it would name one id")
	}
	return nil
}

// templatePointers is the payload pointers the template reads.
func templatePointers(parts []idPart) []string {
	var out []string
	for _, p := range parts {
		if p.pointer != "" {
			out = append(out, p.pointer)
		}
	}
	return out
}

// ComposeID is the id a writer gives a payload of Definition d under source: the
// Definition's identity_template filled from the payload. Every writer of a type
// that declares a template builds its id here, so there is one statement of the
// rule. The payload need not hold the identity field, which is what is being
// composed. A Definition with no template returns ErrNoTemplate; a payload that
// lacks a field the template reads, or holds one it cannot format, returns
// ErrNotCompliant.
func ComposeID(d Definition, source string, payload []byte) (string, error) {
	if d.IdentityTemplate == "" {
		return "", fmt.Errorf("%w: %s v%d", ErrNoTemplate, d.Type, d.Version)
	}
	doc, err := Decode(payload)
	if err != nil {
		return "", err
	}
	return d.composeID(source, doc)
}

func (d Definition) composeID(source string, doc any) (string, error) {
	var b strings.Builder
	for _, p := range d.idParts {
		switch {
		case p.lit != "":
			b.WriteString(p.lit)
		case p.source:
			if source == "" {
				return "", fmt.Errorf("%w: identity_template needs a source", ErrNotCompliant)
			}
			b.WriteString(source)
		default:
			s, err := d.idValue(p, doc)
			if err != nil {
				return "", err
			}
			b.WriteString(s)
		}
	}
	return b.String(), nil
}

// idValue reads and transforms one pointer placeholder.
func (d Definition) idValue(p idPart, doc any) (string, error) {
	v, err := resolve(doc, p.pointer)
	if err != nil {
		return "", fmt.Errorf("%w: identity_template %s: %v", ErrNotCompliant, p.pointer, err)
	}
	s, err := scalarString(v)
	if err != nil {
		return "", fmt.Errorf("%w: identity_template %s: %v", ErrNotCompliant, p.pointer, err)
	}
	switch {
	case p.slug:
		s = slugify(s)
	case p.timeStyle != "":
		t, err := time.ParseInLocation(d.FactTime.Layout, s, d.loc)
		if err != nil {
			return "", fmt.Errorf("%w: identity_template %s: %q is not a time in layout %q",
				ErrNotCompliant, p.pointer, s, d.FactTime.Layout)
		}
		s = t.Format(p.timeStyle)
	}
	if s == "" {
		return "", fmt.Errorf("%w: identity_template %s is empty", ErrNotCompliant, p.pointer)
	}
	return s, nil
}

// slugify lower-cases s and turns each run of characters outside a-z and 0-9 into
// one hyphen, trimming hyphens from the ends.
func slugify(s string) string {
	var b strings.Builder
	pending := false
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			if pending && b.Len() > 0 {
				b.WriteByte('-')
			}
			pending = false
			b.WriteRune(r)
			continue
		}
		pending = true
	}
	return b.String()
}

// WithID returns the payload with its identity field settled by the Definition's
// identity_template. A field the payload omits is added, composed from the rest; a
// field it carries must equal the composition, or the result is ErrIdentityField
// showing both. A Definition with no template returns the payload untouched. The
// gate calls it before it checks the payload against the schema, which requires the
// field.
func (d Definition) WithID(source string, payload []byte) ([]byte, error) {
	if d.IdentityTemplate == "" {
		return payload, nil
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(payload, &obj); err != nil || obj == nil {
		return nil, fmt.Errorf("%w: a payload under identity_template must be a JSON object", ErrNotCompliant)
	}
	doc, err := Decode(payload)
	if err != nil {
		return nil, err
	}
	want, err := d.composeID(source, doc)
	if err != nil {
		return nil, err
	}
	key := topToken(d.IdentityKey[0])
	have, carried := obj[key]
	if !carried {
		quoted, _ := json.Marshal(want) // a string always marshals
		obj[key] = quoted
		out, err := json.Marshal(obj)
		if err != nil {
			return nil, fmt.Errorf("def: write the composed %s: %w", d.IdentityKey[0], err)
		}
		return out, nil
	}
	var got string
	if err := json.Unmarshal(have, &got); err != nil || got != want {
		return nil, fmt.Errorf("%w: %s says %s, identity_template composes %q", ErrIdentityField, d.IdentityKey[0], have, want)
	}
	return payload, nil
}
