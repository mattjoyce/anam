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
	var w counter
	enc := json.NewEncoder(&w)
	enc.SetEscapeHTML(false)
	n := 2 // the array's brackets
	for i, e := range entries {
		if i > 0 {
			n++ // the comma
		}
		w.n = 0
		if err := enc.Encode(e); err != nil {
			// What cannot be encoded cannot be answered either; the handler finds
			// that out when it writes, and this measure is not where to say so.
			continue
		}
		n += w.n - 1 // Encode ends with a newline the array would not carry
		if n > limit {
			return n, true
		}
	}
	return n, false
}

// counter is an io.Writer that only counts.
type counter struct{ n int }

func (c *counter) Write(p []byte) (int, error) {
	c.n += len(p)
	return len(p), nil
}

// Narrowings lists the query parameters q has not used that would narrow a read:
// type, a fact-time window (from, to), a snapshot (at), and a projection. to and at
// exclude each other, so a read with either is offered neither.
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
	if q.Explain {
		return fitQuery(t.Verdicts, limit, q)
	}
	return fitQuery(t.Entries, limit, q)
}
