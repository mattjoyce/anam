package def

import (
	"encoding/json"
	"fmt"
	"strings"
)

// A type's label, declared (card 199, slice 3).
//
// A page that lists facets by the hundred reads best when a row says what the facet
// is ("melatonin 5 mg", "80.4 kg") and not only its type and time. A Definition may
// declare that as `handling.label`, a short template, and a page that draws a facet
// as a thing renders it with Label. A Definition without one has no label, and the
// page falls back to the type and the time. The label is for people: no writer builds
// anything from it and the gate does not read it, which is why it lives in `handling`
// and not in `meta`. It is part of the version all the same, so a changed label is a
// new Definition version, as a changed guidance is.
//
// Grammar. A label is literal text with fields in braces, and nothing else:
//
//	{/pointer}   the scalar at a JSON Pointer in the payload, as its text
//
// There is no `{source}`, no transform and no optional mark: every field is optional,
// because a label is never allowed to fail. A pointer the payload lacks, or that
// reaches an object, an array or null, renders as nothing; a text field renders as its
// first line, so a long note reads as its opening. Runs of white space collapse to one
// space and the ends are trimmed, so `{/drug} {/amount} {/unit}` reads well with the
// unit missing. When the template names fields and none of them rendered anything the
// label is empty, and the page uses its fallback. A pointer must name a field the
// schema declares, so a typo fails at load, not as a blank row.

// labelPart is one piece of a parsed label: literal text, or a pointer.
type labelPart struct {
	lit     string
	pointer string
}

// parseLabel splits a label into parts. It checks the shape only; checkLabel holds the
// pointers to the schema.
func parseLabel(tpl string) ([]labelPart, error) {
	var parts []labelPart
	rest := tpl
	for rest != "" {
		open := strings.IndexAny(rest, "{}")
		if open < 0 {
			parts = append(parts, labelPart{lit: rest})
			break
		}
		if rest[open] == '}' {
			return nil, fmt.Errorf("a '}' with no '{' before it")
		}
		if open > 0 {
			parts = append(parts, labelPart{lit: rest[:open]})
		}
		end := strings.IndexByte(rest[open:], '}')
		if end < 0 {
			return nil, fmt.Errorf("a '{' is never closed")
		}
		body := rest[open+1 : open+end]
		if strings.ContainsRune(body, '{') {
			return nil, fmt.Errorf("a '{' inside a field: {%s}", body)
		}
		if err := checkPointerSyntax(body); err != nil {
			return nil, fmt.Errorf("{%s}: a label field is a JSON Pointer such as {/drug}", body)
		}
		if strings.ContainsAny(body, "|?") {
			return nil, fmt.Errorf("{%s}: a label field takes no transform and no optional mark; every field is optional", body)
		}
		parts = append(parts, labelPart{pointer: body})
		rest = rest[open+end+1:]
	}
	return parts, nil
}

// labelFrom reads handling.label out of a handling block: "" and no parts when it
// declares none. Parse has already checked that handling is an object.
func labelFrom(handling json.RawMessage) (string, []labelPart, error) {
	var block map[string]json.RawMessage
	if err := json.Unmarshal(handling, &block); err != nil {
		return "", nil, nil // Parse refuses a handling that is not an object
	}
	raw, ok := block["label"]
	if !ok {
		return "", nil, nil
	}
	var tpl string
	if err := json.Unmarshal(raw, &tpl); err != nil || strings.TrimSpace(tpl) == "" {
		return "", nil, fmt.Errorf("%w: handling.label must be a non-empty string", ErrBadManifest)
	}
	parts, err := parseLabel(tpl)
	if err != nil {
		return "", nil, fmt.Errorf("%w: handling.label %q: %v", ErrBadManifest, tpl, err)
	}
	reads := 0
	for _, p := range parts {
		if p.pointer != "" {
			reads++
		}
	}
	if reads == 0 {
		return "", nil, fmt.Errorf("%w: handling.label %q reads no payload field, so every row would read the same", ErrBadManifest, tpl)
	}
	return tpl, parts, nil
}

// checkLabel holds the label's pointers to the schema.
func (d Definition) checkLabel() error {
	for _, p := range d.labelParts {
		if p.pointer == "" {
			continue
		}
		if err := d.HasPointer(p.pointer); err != nil {
			return fmt.Errorf("%w: handling.label %q: %v", ErrBadManifest, d.LabelTemplate, err)
		}
	}
	return nil
}

// Label is the short text a payload of this type reads as, or "" when the Definition
// declares no label, the payload cannot be read, or none of the label's fields hold
// anything. The caller falls back to the type and the time. It never fails: a label is
// a convenience, not a check.
func (d Definition) Label(payload []byte) string {
	if len(d.labelParts) == 0 {
		return ""
	}
	doc, err := Decode(payload)
	if err != nil {
		return ""
	}
	var b strings.Builder
	filled := false
	for _, p := range d.labelParts {
		if p.pointer == "" {
			b.WriteString(p.lit)
			continue
		}
		if s := labelField(doc, p.pointer); s != "" {
			b.WriteString(s)
			filled = true
		}
	}
	if !filled {
		return ""
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// labelField is the scalar at the pointer as text, its first line, or "" for anything
// that is not a scalar or is not there.
func labelField(doc any, pointer string) string {
	v, err := resolve(doc, pointer)
	if err != nil {
		return ""
	}
	s, err := scalarString(v)
	if err != nil {
		return ""
	}
	first, _, _ := strings.Cut(s, "\n")
	return strings.TrimSpace(first)
}
