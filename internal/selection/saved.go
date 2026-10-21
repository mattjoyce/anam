package selection

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/mattjoyce/anam/internal/query"
)

// Saved selections (card 206). A program is saved as an artifact of its own kind,
// addressed by the SHA-256 of its canonical text, and named by a short code: the
// first CodeLen hex digits of that hash, written sel:<code>. The same program always
// has the same code, and a code never changes meaning: it names the program, not
// its results, so as the record grows the same code may select more.
//
// Two programs whose hashes share the first CodeLen digits are told apart at save.
// The first one saved keeps its CodeLen digits; the later one's code lengthens to
// the shortest prefix of its hash no other saved selection shares, and that code is
// filed as a second small artifact (CodeKind), since which of the two came first is
// a fact the hashes cannot say. A code is written once and never rewritten.

// The artifact kinds, and the code's shape.
const (
	// Kind files a selection's canonical text.
	Kind = "selection"
	// CodeKind files a code lengthened past CodeLen at save.
	CodeKind = "selection-code"
	// CodeLen is how many hex digits a code has unless it lengthened. It is an
	// invariant, not a default: every code already handed out is this long, and a
	// shorter or longer one would name a different selection or none.
	CodeLen = 8
	// CodePrefix is how a code is written where it could be mistaken for
	// something else: sel:1a2b3c4d.
	CodePrefix = "sel:"
)

// hashLen is the digits of a SHA-256 in hex, the longest a code can be.
const hashLen = 2 * sha256.Size

// Artifacts is the content-addressed store a saved selection is filed in, as
// internal/store/artifact's Store provides it.
type Artifacts interface {
	Put(kind string, b []byte) (string, error)
	Get(kind, digest string) ([]byte, error)
	List(kind string) ([]string, error)
}

// The refusals of saving and opening.
var (
	// ErrNoCode is returned by Open for a code no saved selection has.
	ErrNoCode = errors.New("selection: no saved selection has that code")
	// ErrBadCode is returned for a string that cannot be a code.
	ErrBadCode = errors.New("selection: not a selection code")
	// ErrAmbiguous is returned by Open for a code that names more than one saved
	// selection, which a code given out at save never does.
	ErrAmbiguous = errors.New("selection: the code names more than one saved selection")
	// ErrNotProgram is returned by Save for text that is not a Datalog program.
	ErrNotProgram = errors.New("selection: not a program")
)

// Saved is one saved selection: its code, the hash of its canonical text, and the
// text.
type Saved struct {
	Code    string
	Hash    string
	Program string
}

// Ref is the code as it is written to be read: sel:<code>.
func (s Saved) Ref() string { return CodePrefix + s.Code }

// CanonicalText is the text a program is saved and hashed as: what Program writes for
// the filter Parse reads off it when it is within the chip grammar, so a program
// rewritten in another layout or order is the same selection, and otherwise the text
// as given, trimmed.
func CanonicalText(text string) string {
	if f, err := Parse(text); err == nil {
		if p, err := f.Program(); err == nil {
			return p
		}
	}
	return strings.TrimSpace(text)
}

// ParseCode reads a code as a person or a tool writes one: with or without sel:,
// in either case. It answers the code's hex digits, CodeLen of them or more.
func ParseCode(s string) (string, error) {
	c := strings.ToLower(strings.TrimSpace(s))
	c = strings.TrimPrefix(c, CodePrefix)
	if len(c) < CodeLen || len(c) > hashLen {
		return "", fmt.Errorf("%w: %q has %d hex digits; a code has %d to %d", ErrBadCode, s, len(c), CodeLen, hashLen)
	}
	if _, err := hex.DecodeString(c + strings.Repeat("0", len(c)%2)); err != nil {
		return "", fmt.Errorf("%w: %q is not hex", ErrBadCode, s)
	}
	return c, nil
}

// Store saves and opens selections in an artifact store.
type Store struct{ a Artifacts }

// NewStore returns a Store over a.
func NewStore(a Artifacts) *Store { return &Store{a: a} }

// Save files a program and answers its code. Saving the same program again writes
// nothing new and answers the same code.
func (s *Store) Save(text string) (Saved, error) {
	if strings.TrimSpace(text) == "" {
		return Saved{}, fmt.Errorf("%w: the text is empty", ErrNotProgram)
	}
	canon := CanonicalText(text)
	if _, err := query.Parse(canon); err != nil {
		return Saved{}, fmt.Errorf("%w: %v", ErrNotProgram, err)
	}
	digest := digestOf([]byte(canon))
	code, err := s.codeFor(digest)
	if err != nil {
		return Saved{}, err
	}
	if _, err := s.a.Put(Kind, []byte(canon)); err != nil {
		return Saved{}, fmt.Errorf("selection: save: %w", err)
	}
	return Saved{Code: code, Hash: digest, Program: canon}, nil
}

// codeFor is the code of digest: the one filed for it, its CodeLen digits when it is
// already saved without one, or, for a new selection, the shortest prefix no other
// saved selection shares, filed when it is longer than CodeLen. The code is filed
// before the text, so a save that dies between the two leaves a code whose program
// is missing, which the next save of the same program completes, rather than a
// program whose code the store cannot say.
func (s *Store) codeFor(digest string) (string, error) {
	codes, err := s.codes()
	if err != nil {
		return "", err
	}
	if c, ok := codes.byHash[digest]; ok {
		return c, nil
	}
	stored, err := s.a.List(Kind)
	if err != nil {
		return "", fmt.Errorf("selection: list saved selections: %w", err)
	}
	h := hexOf(digest)
	others := make([]string, 0, len(stored)+len(codes.byHash))
	for _, d := range stored {
		if d == digest {
			return h[:CodeLen], nil
		}
		others = append(others, hexOf(d))
	}
	for d := range codes.byHash {
		others = append(others, hexOf(d))
	}
	code := shortestUnique(h, others)
	if len(code) == CodeLen {
		return code, nil
	}
	rec, err := json.Marshal(codeRecord{Code: code, Hash: digest})
	if err != nil {
		return "", fmt.Errorf("selection: code record: %w", err)
	}
	if _, err := s.a.Put(CodeKind, rec); err != nil {
		return "", fmt.Errorf("selection: file code %s: %w", code, err)
	}
	return code, nil
}

// shortestUnique is the shortest prefix of h, CodeLen digits at least, that starts
// none of others. Every hash differs from h somewhere, so one is found by the whole.
func shortestUnique(h string, others []string) string {
	for n := CodeLen; n < len(h); n++ {
		if !anyHasPrefix(others, h[:n], h) {
			return h[:n]
		}
	}
	return h
}

func anyHasPrefix(xs []string, prefix, self string) bool {
	for _, x := range xs {
		if x == self {
			continue
		}
		if strings.HasPrefix(x, prefix) {
			return true
		}
	}
	return false
}

// Open answers the saved selection a code names. A code filed at save names its
// selection; a code of CodeLen digits names the one saved selection that starts with
// them and did not lengthen; a longer code that was never filed is read as a prefix
// of a hash and names the one selection it starts, if there is one.
func (s *Store) Open(code string) (Saved, error) {
	c, err := ParseCode(code)
	if err != nil {
		return Saved{}, err
	}
	codes, err := s.codes()
	if err != nil {
		return Saved{}, err
	}
	if d, ok := codes.byCode[c]; ok {
		return s.get(d, c)
	}
	stored, err := s.a.List(Kind)
	if err != nil {
		return Saved{}, fmt.Errorf("selection: list saved selections: %w", err)
	}
	var found []string
	for _, d := range stored {
		if !strings.HasPrefix(hexOf(d), c) {
			continue
		}
		if _, lengthened := codes.byHash[d]; lengthened && len(c) == CodeLen {
			continue
		}
		found = append(found, d)
	}
	switch len(found) {
	case 0:
		return Saved{}, fmt.Errorf("%w: %s%s", ErrNoCode, CodePrefix, c)
	case 1:
		own, ok := codes.byHash[found[0]]
		if !ok {
			own = hexOf(found[0])[:CodeLen]
		}
		return s.get(found[0], own)
	}
	return Saved{}, fmt.Errorf("%w: %s%s starts %d; give more of the hash", ErrAmbiguous, CodePrefix, c, len(found))
}

func (s *Store) get(digest, code string) (Saved, error) {
	b, err := s.a.Get(Kind, digest)
	if err != nil {
		return Saved{}, fmt.Errorf("%w: %s%s is filed, but its program is not: %v", ErrNoCode, CodePrefix, code, err)
	}
	return Saved{Code: code, Hash: digest, Program: string(b)}, nil
}

// codeRecord is a code that lengthened, as CodeKind files it.
type codeRecord struct {
	Code string `json:"code"`
	Hash string `json:"hash"`
}

// codeIndex is the filed codes both ways.
type codeIndex struct {
	byCode map[string]string
	byHash map[string]string
}

// codes reads every filed code. There is one only for each pair of saved selections
// whose hashes share CodeLen digits, so this is few reads, usually none.
func (s *Store) codes() (codeIndex, error) {
	idx := codeIndex{byCode: map[string]string{}, byHash: map[string]string{}}
	digests, err := s.a.List(CodeKind)
	if err != nil {
		return idx, fmt.Errorf("selection: list codes: %w", err)
	}
	for _, d := range digests {
		b, err := s.a.Get(CodeKind, d)
		if err != nil {
			return idx, fmt.Errorf("selection: read code %s: %w", d, err)
		}
		var r codeRecord
		if err := json.Unmarshal(b, &r); err != nil {
			return idx, fmt.Errorf("selection: code %s: %w", d, err)
		}
		if len(r.Code) <= CodeLen || !strings.HasPrefix(hexOf(r.Hash), r.Code) {
			return idx, fmt.Errorf("selection: code %s files %q for %s, which is not a lengthened prefix of it", d, r.Code, r.Hash)
		}
		idx.byCode[r.Code], idx.byHash[r.Hash] = r.Hash, r.Code
	}
	return idx, nil
}

// digestOf is the content address the artifact store files b under.
func digestOf(b []byte) string {
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func hexOf(digest string) string { return strings.TrimPrefix(digest, "sha256:") }
