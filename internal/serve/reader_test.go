package serve_test

import (
	"context"
	"testing"
	"time"

	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/serve"
)

// A serve through Deps that name a reader files a receipt that names it, and that
// receipt verifies like any other. Deps that name nobody file the receipt they always
// did, with no reader in it.
func TestReader_NamedOnTheReceiptAndVerifies(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.seed(t)
	ctx := context.Background()

	anon, err := serve.Run(ctx, h.deps, "bp", false)
	if err != nil {
		t.Fatal(err)
	}
	named := h.deps
	named.Reader = "lifeos"
	read, err := serve.Run(ctx, named, "bp", false)
	if err != nil {
		t.Fatal(err)
	}
	if anon.Receipt.Principal != "" || read.Receipt.Principal != "lifeos" {
		t.Fatalf("principals %q and %q", anon.Receipt.Principal, read.Receipt.Principal)
	}
	for name, s := range map[string]serve.Served{"anonymous": anon, "named": read} {
		_, res, err := serve.Verify(ctx, h.deps, s.ReceiptHash)
		if err != nil {
			t.Fatalf("%s: Verify: %v", name, err)
		}
		if !res.OK() {
			t.Errorf("%s receipt does not verify: %s", name, res.Summary())
		}
	}
}

// The listing filters by the reader's name and by served time, each part alone and
// together, and keeps its order.
func TestList_ByPrincipalAndServedTime(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.seed(t)
	ctx := context.Background()

	serveAs := func(reader string, at time.Time) {
		t.Helper()
		d := h.deps
		d.Reader = reader
		d.Clock = clock.Fixed{Instant: at}
		if _, err := serve.Run(ctx, d, "bp", false); err != nil {
			t.Fatalf("Run as %q: %v", reader, err)
		}
	}
	day := func(n int) time.Time { return time.Date(2026, 10, n, 9, 0, 0, 0, time.UTC) }
	serveAs("lifeos", day(1))
	serveAs("lifeos", day(3))
	serveAs("chat", day(2))
	serveAs("", day(4))

	at := func(n int) string { return clock.Format(day(n)) }
	cases := []struct {
		name string
		f    serve.ListFilter
		want []string // principal of each receipt listed, newest first
	}{
		{"everything", serve.ListFilter{}, []string{"", "lifeos", "chat", "lifeos"}},
		{"one reader", serve.ListFilter{Principal: "lifeos"}, []string{"lifeos", "lifeos"}},
		{"a reader nobody was", serve.ListFilter{Principal: "nobody"}, nil},
		{"since is inclusive", serve.ListFilter{Since: at(3)}, []string{"", "lifeos"}},
		{"until is exclusive", serve.ListFilter{Until: at(3)}, []string{"chat", "lifeos"}},
		{"a window and a reader", serve.ListFilter{Principal: "lifeos", Since: at(2), Until: at(4)}, []string{"lifeos"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, _, err := serve.List(ctx, h.deps, c.f)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != len(c.want) {
				t.Fatalf("listed %d receipts, want %d", len(got), len(c.want))
			}
			for i, r := range got {
				if r.Principal != c.want[i] {
					t.Errorf("receipt %d names %q, want %q", i, r.Principal, c.want[i])
				}
			}
		})
	}
}
