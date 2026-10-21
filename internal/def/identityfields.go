package def

// ComposedField is the top-level field the identity_template composes, as a JSON
// Pointer, or "" for a Definition with no template. A form leaves it out, since the
// gate fills it; a writer that sent it would only repeat what the rule already says.
func (d Definition) ComposedField() string {
	if d.IdentityTemplate == "" {
		return ""
	}
	return d.IdentityKey[0]
}

// IdentityFields are the payload pointers that decide which facet a payload is: the
// identity_key pointers the writer supplies, and every pointer the identity_template
// reads. Changing any of them names a different facet, so a form that corrects a
// facet it holds shows them and does not let them move. The composed field is not
// among them, as it is not the writer's to supply.
func (d Definition) IdentityFields() []string {
	composed := d.ComposedField()
	var out []string
	seen := map[string]bool{}
	add := func(p string) {
		if p != "" && p != composed && !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	for _, p := range d.IdentityKey {
		add(p)
	}
	for _, p := range d.idParts {
		add(p.pointer) // an optional placeholder's too: its value names the facet when it is there
	}
	return out
}
