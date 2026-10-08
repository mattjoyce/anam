package app

import (
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/api"
)

// The header is how an operator ties what they are looking at to what serve published,
// so the served hash has to be on it whenever a projection is in force — and absent
// when one is not, rather than silently repeating the canonical's.
func TestTimelineHeader(t *testing.T) {
	t.Parallel()
	base := api.Timeline{W: 893, Live: 1, Shown: 1, Canonical: "canon-hash"}

	t.Run("canonical", func(t *testing.T) {
		t.Parallel()
		h := timelineHeader(base)
		for _, want := range []string{"W=893", "live=1", "shown=1", "canonical  canon-hash"} {
			if !strings.Contains(h, want) {
				t.Errorf("header omits %q:\n%s", want, h)
			}
		}
		if strings.Contains(h, "projection") {
			t.Errorf("a canonical read claimed a projection:\n%s", h)
		}
		if strings.Contains(h, "as-of") {
			t.Errorf("a head read claimed an as-of:\n%s", h)
		}
	})

	t.Run("through a projection", func(t *testing.T) {
		t.Parallel()
		v := base
		v.Shown = 0
		v.Projection = &api.ProjectionRead{Name: "bp", Version: 1, Served: 0, Hash: "served-hash"}

		h := timelineHeader(v)
		for _, want := range []string{"served=0", "projection bp v1  served-hash"} {
			if !strings.Contains(h, want) {
				t.Errorf("header omits %q:\n%s", want, h)
			}
		}
		// Both hashes must appear: the canonical says what was pruned from, the served
		// says what came out, and conflating them would hide the pruning.
		if !strings.Contains(h, "canon-hash") {
			t.Errorf("header lost the canonical hash:\n%s", h)
		}
	})

	t.Run("as-of", func(t *testing.T) {
		t.Parallel()
		v := base
		v.AsOf = "2026-07-10T20:00:00.000000000Z"
		h := timelineHeader(v)
		if !strings.Contains(h, "as-of      2026-07-10T20:00:00.000000000Z (anam-time)") {
			t.Errorf("as-of is not labelled as anam-time:\n%s", h)
		}
	})
}
