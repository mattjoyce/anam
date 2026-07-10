package def

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// A minimal RFC 6901 JSON Pointer: exactly enough to address fields in a decoded
// payload. Anam addresses a handful of pointers per facet, once, at admission —
// a dependency would be more code than this file.

// ErrPointer is returned when a pointer is malformed or does not resolve.
var ErrPointer = errors.New("json pointer")

func checkPointerSyntax(p string) error {
	if p == "" || !strings.HasPrefix(p, "/") {
		return fmt.Errorf("%w: %q must start with '/'", ErrPointer, p)
	}
	return nil
}

// resolve walks a decoded JSON value (maps, slices, scalars) to the pointer's target.
func resolve(doc any, pointer string) (any, error) {
	if err := checkPointerSyntax(pointer); err != nil {
		return nil, err
	}
	cur := doc
	for _, token := range strings.Split(pointer[1:], "/") {
		token = strings.ReplaceAll(strings.ReplaceAll(token, "~1", "/"), "~0", "~")
		switch node := cur.(type) {
		case map[string]any:
			next, ok := node[token]
			if !ok {
				return nil, fmt.Errorf("%w: %q has no member %q", ErrPointer, pointer, token)
			}
			cur = next
		case []any:
			i, err := strconv.Atoi(token)
			if err != nil || i < 0 || i >= len(node) {
				return nil, fmt.Errorf("%w: %q index %q out of range", ErrPointer, pointer, token)
			}
			cur = node[i]
		default:
			return nil, fmt.Errorf("%w: %q hit a scalar before %q", ErrPointer, pointer, token)
		}
	}
	return cur, nil
}

// scalarString renders a resolved leaf as its source text: strings as-is, numbers as
// their source lexeme (UseNumber preserves it), booleans literally. Containers have
// no place in an identity or a rule and are rejected.
func scalarString(v any) (string, error) {
	switch t := v.(type) {
	case string:
		return t, nil
	case json.Number:
		return t.String(), nil
	case bool:
		return strconv.FormatBool(t), nil
	case nil:
		return "", fmt.Errorf("%w: value is null", ErrPointer)
	default:
		return "", fmt.Errorf("%w: value is a container (%T), not a scalar", ErrPointer, v)
	}
}

// Decode parses payload bytes the way every def check expects: numbers preserved as
// their source lexemes.
func Decode(payload []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(payload))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, fmt.Errorf("def: decode payload: %w", err)
	}
	return v, nil
}

// bytesReader exists so def.go need not import bytes for one call.
func bytesReader(b []byte) *bytes.Reader { return bytes.NewReader(b) }
