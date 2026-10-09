package def

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mattjoyce/anam/internal/facet"
)

// ErrNotCompliant is returned when a payload fails its Definition — the gate's
// facet-compliant pillar (§6).
var ErrNotCompliant = errors.New("def: payload not compliant")

// Check holds a decoded payload to the Definition's schema. Verbatim checks nothing
// here (identity and time are enforced by their own calls).
//
// The schema is closed and its required fields refuse null and "" (schema.go), so a
// passing payload names no field the Definition does not declare and leaves no
// required field blank.
func (d Definition) Check(doc any) error {
	if d.schema == nil {
		return nil
	}
	if err := d.schema.Validate(doc); err != nil {
		return fmt.Errorf("%w: %s", ErrNotCompliant, oneLine(err))
	}
	return nil
}

// oneLine flattens the validator's indented report into one line for a log or an
// API response.
func oneLine(err error) string {
	return strings.Join(strings.Fields(err.Error()), " ")
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
	// The source's clock is read in the Definition's zone. A layout that carries its
	// own offset keeps it; a zoneless one, which is what every local export writes,
	// lands at the instant the person lived it rather than ten hours away.
	t, err := time.ParseInLocation(tf.Layout, s, d.loc)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: time %s value %q does not parse as %q", ErrNotCompliant, tf.Pointer, s, tf.Layout)
	}
	return t.UTC(), nil
}

// SourceRef composes the row's natural key from the Definition's identity_key —
// identity, never content (§6a): facet.SourceRef over the identity pointers' values,
// in declared order.
func (d Definition) SourceRef(source string, doc any) (string, error) {
	return compose("identity", source, d.IdentityKey, doc)
}

// SeriesOf composes the series stamp from the Definition's series key: facet.Stamp
// over the key pointers' values, in declared order. It is ""
// for a type without a series, and an error when a key pointer does not resolve: a
// row that cannot say which series it belongs to is not compliant.
func (d Definition) SeriesOf(doc any) (string, error) {
	if d.Series == nil {
		return "", nil
	}
	return compose("series", d.Series.Name, d.Series.Key, doc)
}

// compose resolves the scalar values at the pointers and stamps them after the
// prefix (facet.Stamp). Both the source_ref and the series stamp are this shape, so
// they are built by one function.
func compose(what, prefix string, pointers []string, doc any) (string, error) {
	parts := make([]string, 0, len(pointers))
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
	return facet.Stamp(prefix, parts...), nil
}
