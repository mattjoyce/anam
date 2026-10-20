package read

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/mattjoyce/anam/internal/api"
)

// ErrTooLarge marks a read refused for its size: what matched encodes to more than
// the bound the daemon answers within. Narrowing the query is the remedy; asking
// again unchanged will fail again.
var ErrTooLarge = errors.New("result too large")

// TooLarge is the refusal itself, carrying its measure for the API's error body and
// what to do instead, in words.
type TooLarge struct {
	api.TooLarge
	Hint string
}

func (e *TooLarge) Error() string {
	if e.FacetID != "" {
		return fmt.Sprintf("result too large: facet %s alone encodes to more than %d bytes (max_read_bytes); %s",
			e.FacetID, e.MaxBytes, e.Hint)
	}
	return fmt.Sprintf("result too large: %d entries encode to more than %d bytes (max_read_bytes); %s",
		e.Matched, e.MaxBytes, e.Hint)
}

func (e *TooLarge) Is(target error) bool { return target == ErrTooLarge }

// Fit reports whether entries, encoded as the API answers with them, fit within limit
// bytes, and refuses with a *TooLarge saying hint when they do not. A limit of zero is
// no bound. Encoding stops at the first entry that crosses the limit, so a refusal
// costs at most the limit's worth of encoding, never the whole answer.
func Fit[E any](entries []E, limit int, hint string) error {
	if limit <= 0 {
		return nil
	}
	n, over := encodedSize(entries, limit)
	if !over {
		return nil
	}
	return &TooLarge{
		TooLarge: api.TooLarge{Matched: len(entries), Bytes: n, MaxBytes: limit},
		Hint:     hint,
	}
}

// encodedSize counts the bytes entries encode to as a JSON array, in the daemon's
// own encoding (HTML left as it is), stopping once the count passes limit. It returns
// the count so far and whether it passed.
func encodedSize[E any](entries []E, limit int) (int, bool) {
	s := newSizer()
	n := 2 // the array's brackets
	for i, e := range entries {
		if i > 0 {
			n++ // the comma
		}
		n += s.size(e)
		if n > limit {
			return n, true
		}
	}
	return n, false
}

// pageEnd is where the page of entries that starts at offset ends: past the most
// whole entries from offset that encode, as a JSON array, within limit, and never
// fewer than one; to the end when limit is zero. A page never cuts a facet, so an
// entry over the limit on its own is refused with a *TooLarge naming its id from ids
// (aligned with entries) and saying hint. The measure is Fit's.
func pageEnd[E any](entries []E, ids []string, offset, limit int, hint string) (int, error) {
	if limit <= 0 {
		return len(entries), nil
	}
	s := newSizer()
	n := 2
	for i := offset; i < len(entries); i++ {
		add := s.size(entries[i])
		if i > offset {
			add++
		}
		if n+add <= limit {
			n += add
			continue
		}
		if i > offset {
			return i, nil
		}
		return 0, &TooLarge{
			TooLarge: api.TooLarge{Matched: 1, Bytes: n + add, MaxBytes: limit, FacetID: ids[i]},
			Hint:     hint,
		}
	}
	return len(entries), nil
}

// fitEach refuses entries when any one of them, alone, encodes past limit: the one
// thing paging cannot answer. It names the first such entry by its id in ids and
// says hint. A limit of zero is no bound.
func fitEach[E any](entries []E, ids []string, limit int, hint string) error {
	if limit <= 0 {
		return nil
	}
	s := newSizer()
	for i, e := range entries {
		if n := 2 + s.size(e); n > limit {
			return &TooLarge{
				TooLarge: api.TooLarge{Matched: 1, Bytes: n, MaxBytes: limit, FacetID: ids[i]},
				Hint:     hint,
			}
		}
	}
	return nil
}

// sizer measures one value as the daemon encodes it, without its trailing newline.
type sizer struct {
	w   counter
	enc *json.Encoder
}

func newSizer() *sizer {
	s := &sizer{}
	s.enc = json.NewEncoder(&s.w)
	s.enc.SetEscapeHTML(false)
	return s
}

// size is e's encoded length. What cannot be encoded cannot be answered either; the
// handler finds that out when it writes, and this measure is not where to say so, so
// it counts as nothing.
func (s *sizer) size(e any) int {
	s.w.n = 0
	if err := s.enc.Encode(e); err != nil {
		return 0
	}
	return s.w.n - 1 // Encode ends with a newline the array would not carry
}

// counter is an io.Writer that only counts.
type counter struct{ n int }

func (c *counter) Write(p []byte) (int, error) {
	c.n += len(p)
	return len(p), nil
}

// Narrowings lists the query parameters q has not used that would narrow a read:
// type, a fact-time window (from, to), a snapshot (at), a projection, and fields.
// to and at exclude each other, so a read with either is offered neither.
func Narrowings(q api.TimelineQuery) []string {
	var out []string
	if q.Type == "" {
		out = append(out, "type")
	}
	if q.From == "" {
		out = append(out, "from")
	}
	if q.To == "" && q.At == "" {
		out = append(out, "to", "at")
	}
	if q.Projection == "" {
		out = append(out, "projection")
	}
	if len(q.Fields) == 0 {
		out = append(out, "fields")
	}
	return out
}

// NarrowHint is the words for Narrowings: what the reader can still add.
func NarrowHint(narrow []string) string {
	if len(narrow) == 0 {
		return "the query is as narrow as it can be asked; read it locally"
	}
	return "narrow it by " + strings.Join(narrow, ", ")
}

// fitQuery is Fit with the refusal's narrowings and hint taken from q.
func fitQuery[E any](entries []E, limit int, q api.TimelineQuery) error {
	narrow := Narrowings(q)
	err := Fit(entries, limit, NarrowHint(narrow))
	var tl *TooLarge
	if errors.As(err, &tl) {
		tl.Narrow = narrow
	}
	return err
}

// FitTimeline is Fit over what t answers with, its entries or, explained, its
// verdicts, with the narrowings q leaves open. The daemon calls it on a timeline it
// is about to answer; a local read never does.
func FitTimeline(t api.Timeline, q api.TimelineQuery, limit int) error {
	switch {
	case q.Explain:
		return fitQuery(t.Verdicts, limit, q)
	case t.Frame != nil:
		return fitQuery(t.Frame.Rows, limit, q)
	}
	return fitQuery(t.Entries, limit, q)
}
