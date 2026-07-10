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
func (d Definition) Check(doc any) error {
	if d.Mode == ModeVerbatim {
		return nil
	}

	for _, ptr := range d.Required {
		if _, err := resolve(doc, ptr); err != nil {
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
	named[topToken(d.EventTime.Pointer)] = true
	named[topToken(d.RecordedTime.Pointer)] = true
	for _, ptr := range d.IdentityKey {
		named[topToken(ptr)] = true
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

// Times extracts the two payload clocks (§5). The recorded time defaults to the
// event time at Parse, so both always resolve or the payload is not compliant.
func (d Definition) Times(doc any) (event, recorded time.Time, err error) {
	event, err = d.timeAt(doc, d.EventTime)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	recorded, err = d.timeAt(doc, d.RecordedTime)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	return event, recorded, nil
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
	parts := make([]string, 0, len(d.IdentityKey)+1)
	parts = append(parts, source)
	for _, ptr := range d.IdentityKey {
		v, err := resolve(doc, ptr)
		if err != nil {
			return "", fmt.Errorf("%w: identity %s: %v", ErrNotCompliant, ptr, err)
		}
		s, err := scalarString(v)
		if err != nil {
			return "", fmt.Errorf("%w: identity %s: %v", ErrNotCompliant, ptr, err)
		}
		if s == "" {
			return "", fmt.Errorf("%w: identity %s is empty", ErrNotCompliant, ptr)
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, ":"), nil
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
