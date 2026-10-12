package read_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/api"
	"github.com/mattjoyce/anam/internal/fold"
	"github.com/mattjoyce/anam/internal/read"
)

func entries(n int) []fold.Entry {
	out := make([]fold.Entry, n)
	for i := range out {
		out[i] = fold.Entry{
			FacetID: "f", Type: "health.blood-pressure", FactTime: "2026-02-10T00:00:00.000000000Z",
			Source: "records", SourceRef: "records:1", Payload: json.RawMessage(`{"systolic":120,"html":"<b>&</b>"}`),
		}
	}
	return out
}

// wireSize is what the daemon's encoder writes for entries: HTML left as it is.
func wireSize(t *testing.T, v any) int {
	t.Helper()
	var sb strings.Builder
	enc := json.NewEncoder(&sb)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		t.Fatal(err)
	}
	return sb.Len() - 1
}

// The bound is measured as the daemon encodes the answer: exactly at the wire size
// it fits, one byte under it does not, and zero is no bound at all.
func TestFit_MeasuresTheWireEncoding(t *testing.T) {
	t.Parallel()
	three := entries(3)
	size := wireSize(t, three)
	tests := []struct {
		name  string
		limit int
		over  bool
	}{
		{"no bound", 0, false},
		{"exactly the answer", size, false},
		{"one byte short", size - 1, true},
		{"well under", size / 2, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := read.Fit(three, tt.limit, "narrow it")
			if (err != nil) != tt.over {
				t.Fatalf("Fit(limit %d of %d bytes) = %v, want over=%v", tt.limit, size, err, tt.over)
			}
			if !tt.over {
				return
			}
			var tl *read.TooLarge
			if !errors.Is(err, read.ErrTooLarge) || !errors.As(err, &tl) {
				t.Fatalf("Fit = %T %v, want ErrTooLarge as *TooLarge", err, err)
			}
			if tl.Matched != 3 || tl.MaxBytes != tt.limit || tl.Bytes <= tt.limit {
				t.Errorf("measure = %+v, want matched 3, max %d, bytes over it", tl.TooLarge, tt.limit)
			}
			for _, want := range []string{"3 entries", "max_read_bytes", "narrow it"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not say %q", err, want)
				}
			}
		})
	}
}

// Counting stops at the bound: the measure reported is the first count past it,
// not the whole answer's size.
func TestFit_StopsCountingAtTheBound(t *testing.T) {
	t.Parallel()
	many := entries(1000)
	one := wireSize(t, entries(1))
	err := read.Fit(many, one*2, "")
	var tl *read.TooLarge
	if !errors.As(err, &tl) {
		t.Fatalf("Fit = %v, want too large", err)
	}
	if tl.Bytes > one*4 {
		t.Errorf("bytes counted = %d, want counting to stop near the bound (%d), not the whole %d",
			tl.Bytes, one*2, wireSize(t, many))
	}
}

// The refusal names the narrowings the query has not used, and leaves out the ones
// the query already has or that its shape excludes.
func TestNarrowings(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		q    api.TimelineQuery
		want string
	}{
		{"nothing narrowed", api.TimelineQuery{}, "type from to at projection fields"},
		{"a type", api.TimelineQuery{Type: "health.blood-pressure"}, "from to at projection fields"},
		{"a window", api.TimelineQuery{From: "2026-06-01", To: "2026-06-30"}, "type projection fields"},
		{"a snapshot", api.TimelineQuery{At: "2026-06-15"}, "type from projection fields"},
		{"a projection and a window", api.TimelineQuery{Projection: "bp", From: "2026-06-01", To: "2026-06-30"}, "type fields"},
		{"everything", api.TimelineQuery{Projection: "bp", Type: "t", From: "a", At: "b"}, "fields"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := strings.Join(read.Narrowings(tt.q), " "); got != tt.want {
				t.Errorf("Narrowings = %q, want %q", got, tt.want)
			}
		})
	}
	if hint := read.NarrowHint(nil); !strings.Contains(hint, "locally") {
		t.Errorf("a fully narrowed read's hint should say to read locally: %q", hint)
	}
}

// A timeline is measured by what it answers with: entries, or verdicts when
// explained, and the refusal carries the query's open narrowings as data.
func TestFitTimeline(t *testing.T) {
	t.Parallel()
	tl := api.Timeline{Entries: entries(2), Verdicts: []api.VerdictRow{{FacetID: "f", Standing: "served"}}}
	q := api.TimelineQuery{Type: "health.blood-pressure"}
	if err := read.FitTimeline(tl, q, wireSize(t, tl.Entries)-1); err == nil {
		t.Fatal("two entries over the bound should be refused")
	} else {
		var got *read.TooLarge
		if !errors.As(err, &got) || strings.Join(got.Narrow, " ") != "from to at projection fields" {
			t.Errorf("refusal = %v, want narrow [from to at projection fields]", err)
		}
	}
	// Explained, the small verdicts are what is measured, not the entries.
	q.Explain = true
	if err := read.FitTimeline(tl, q, wireSize(t, tl.Verdicts)); err != nil {
		t.Errorf("an explain within its verdicts' size was refused: %v", err)
	}
}
