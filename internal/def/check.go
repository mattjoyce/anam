package def

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrNotCompliant is returned when a payload fails its Definition — the gate's
// facet-compliant pillar (§6).
var ErrNotCompliant = errors.New("def: payload not compliant")

// Check enforces the schema half against a decoded payload, per the validation mode.
// Verbatim checks nothing here (identity and time are enforced by their own calls);
// lenient enforces required/pattern/enum and ignores unknown fields; strict also
// rejects top-level fields the schema does not name.
//
// Required means filled: a field that is present but null or an empty string is not
// there in any sense a reader cares about, and a measurement the source left blank
// must not be admitted as if it were taken. An empty container is not empty for this
// purpose; a list the source wrote as `[]` is what the source said.
func (d Definition) Check(doc any) error {
	if d.Mode == ModeVerbatim {
		return nil
	}

	for _, ptr := range d.Required {
		v, err := resolve(doc, ptr)
		if err != nil {
			return fmt.Errorf("%w: required %s: %v", ErrNotCompliant, ptr, err)
		}
		if err := filled(v); err != nil {
			return fmt.Errorf("%w: required %s: %v", ErrNotCompliant, ptr, err)
		}
	}

	for ptr, rule := range d.Properties {
		v, err := resolve(doc, ptr)
		if err != nil {
			continue // absent is required's business, not the rule's
		}
		s, err := scalarString(v)
		if err != nil {
			return fmt.Errorf("%w: %s: %v", ErrNotCompliant, ptr, err)
		}
		if rule.Pattern != nil && !rule.Pattern.MatchString(s) {
			return fmt.Errorf("%w: %s value %q does not match %s", ErrNotCompliant, ptr, s, rule.Pattern)
		}
		if len(rule.Enum) > 0 && !contains(rule.Enum, s) {
			return fmt.Errorf("%w: %s value %q is not one of %v", ErrNotCompliant, ptr, s, rule.Enum)
		}
	}

	if !d.AdditionalProperties {
		if err := d.checkNoExtras(doc); err != nil {
			return err
		}
	}
	return nil
}

// checkNoExtras rejects top-level members the schema does not name. Top-level only:
// the payload below named members is the source's own nested shape, and policing it
// would creep toward the normalisation the verbatim rule forbids.
func (d Definition) checkNoExtras(doc any) error {
	obj, ok := doc.(map[string]any)
	if !ok {
		return fmt.Errorf("%w: strict mode requires a top-level object", ErrNotCompliant)
	}
	named := map[string]bool{}
	for _, ptr := range d.Required {
		named[topToken(ptr)] = true
	}
	for ptr := range d.Properties {
		named[topToken(ptr)] = true
	}
	named[topToken(d.FactTime.Pointer)] = true
	for _, ptr := range d.IdentityKey {
		named[topToken(ptr)] = true
	}
	if d.Series != nil {
		for _, ptr := range d.Series.Key {
			named[topToken(ptr)] = true
		}
	}
	for k := range obj {
		if !named[k] {
			return fmt.Errorf("%w: unexpected field %q (strict mode)", ErrNotCompliant, k)
		}
	}
	return nil
}

func topToken(pointer string) string {
	rest := strings.TrimPrefix(pointer, "/")
	if i := strings.IndexByte(rest, '/'); i >= 0 {
		rest = rest[:i]
	}
	return strings.ReplaceAll(strings.ReplaceAll(rest, "~1", "/"), "~0", "~")
}

// FactTimeOf extracts the payload's own clock — when the fact was true in the world
// (§5). It is the only time a Definition can read: Anam's clock is stamped by the gate
// onto the AnamEvent, where a source has no say in it.
func (d Definition) FactTimeOf(doc any) (time.Time, error) {
	return d.timeAt(doc, d.FactTime)
}

func (d Definition) timeAt(doc any, tf TimeField) (time.Time, error) {
	v, err := resolve(doc, tf.Pointer)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: time %s: %v", ErrNotCompliant, tf.Pointer, err)
	}
	s, err := scalarString(v)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: time %s: %v", ErrNotCompliant, tf.Pointer, err)
	}
	t, err := time.Parse(tf.Layout, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: time %s value %q does not parse as %q", ErrNotCompliant, tf.Pointer, s, tf.Layout)
	}
	return t.UTC(), nil
}

// SourceRef composes the row's natural key from the Definition's identity_key —
// identity, never content (§6a): "<source>:<v1>[:<v2>...]" over the identity
// pointers' values, in declared order.
func (d Definition) SourceRef(source string, doc any) (string, error) {
	return compose("identity", source, d.IdentityKey, doc)
}

// SeriesOf composes the series stamp from the Definition's series key:
// "<name>:<v1>[:<v2>...]" over the key pointers' values, in declared order. It is ""
// for a type without a series, and an error when a key pointer does not resolve: a
// row that cannot say which series it belongs to is not compliant.
func (d Definition) SeriesOf(doc any) (string, error) {
	if d.Series == nil {
		return "", nil
	}
	return compose("series", d.Series.Name, d.Series.Key, doc)
}

// compose joins a prefix and the scalar values at the pointers with ':'. Both the
// source_ref and the series stamp are this shape, so they are built by one function.
func compose(what, prefix string, pointers []string, doc any) (string, error) {
	parts := make([]string, 0, len(pointers)+1)
	parts = append(parts, prefix)
	for _, ptr := range pointers {
		v, err := resolve(doc, ptr)
		if err != nil {
			return "", fmt.Errorf("%w: %s %s: %v", ErrNotCompliant, what, ptr, err)
		}
		s, err := scalarString(v)
		if err != nil {
			return "", fmt.Errorf("%w: %s %s: %v", ErrNotCompliant, what, ptr, err)
		}
		if s == "" {
			return "", fmt.Errorf("%w: %s %s is empty", ErrNotCompliant, what, ptr)
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, ":"), nil
}

// filled rejects null and the empty string.
func filled(v any) error {
	switch t := v.(type) {
	case nil:
		return fmt.Errorf("is null")
	case string:
		if t == "" {
			return fmt.Errorf("is empty")
		}
	}
	return nil
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
