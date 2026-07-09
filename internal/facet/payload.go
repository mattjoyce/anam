package facet

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
)

// ErrMalformedPayload is returned when a payload is not a single well-formed JSON
// value.
var ErrMalformedPayload = errors.New("malformed payload")

// Canonicalize returns the deterministic serialisation of a JSON payload: object keys
// sorted by byte order, insignificant whitespace removed, and numbers preserved
// exactly as the source wrote them.
//
// It fixes serialisation, never values. That distinction is what lets it coexist with
// the verbatim rule (§3): a payload's content is untouched, only its byte
// representation is pinned. Without that pinning, a source row re-read through a Go
// map would serialise its keys in a different order on every capture, hash
// differently, and defeat the idempotent commit (§6a).
//
// Numbers survive as their source lexeme rather than being routed through float64, so
// a large integer or a trailing zero in "1.50" reaches the ledger as written.
func Canonicalize(raw []byte) ([]byte, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()

	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, fmt.Errorf("canonicalize: %w: %v", ErrMalformedPayload, err)
	}
	if dec.More() {
		return nil, fmt.Errorf("canonicalize: %w: trailing data after top-level value", ErrMalformedPayload)
	}

	var buf bytes.Buffer
	if err := writeCanonical(&buf, v); err != nil {
		return nil, fmt.Errorf("canonicalize: %w", err)
	}
	return buf.Bytes(), nil
}

// ContentHash returns "sha256:<hex>" over canonical payload bytes. A changed source
// row hashes differently, which is what the gate uses to tell a no-op re-ingest from a
// supersession (§6a).
func ContentHash(canonical []byte) string {
	sum := sha256.Sum256(canonical)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func writeCanonical(buf *bytes.Buffer, v any) error {
	switch t := v.(type) {
	case nil:
		buf.WriteString("null")
	case bool:
		if t {
			buf.WriteString("true")
		} else {
			buf.WriteString("false")
		}
	case json.Number:
		buf.WriteString(t.String())
	case string:
		return writeCanonicalString(buf, t)
	case []any:
		buf.WriteByte('[')
		for i, elem := range t {
			if i > 0 {
				buf.WriteByte(',')
			}
			if err := writeCanonical(buf, elem); err != nil {
				return err
			}
		}
		buf.WriteByte(']')
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		buf.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				buf.WriteByte(',')
			}
			if err := writeCanonicalString(buf, k); err != nil {
				return err
			}
			buf.WriteByte(':')
			if err := writeCanonical(buf, t[k]); err != nil {
				return err
			}
		}
		buf.WriteByte('}')
	default:
		return fmt.Errorf("%w: unsupported JSON value of type %T", ErrMalformedPayload, v)
	}
	return nil
}

// writeCanonicalString encodes s as a JSON string without HTML escaping, so a payload
// containing "<" reaches the ledger as the source wrote it.
func writeCanonicalString(buf *bytes.Buffer, s string) error {
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(s); err != nil {
		return fmt.Errorf("encode string: %w", err)
	}
	buf.Truncate(buf.Len() - 1) // Encode appends a newline.
	return nil
}
