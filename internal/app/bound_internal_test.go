package app

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/client"
	"github.com/mattjoyce/anam/internal/config"
)

// boundedFixture is a daemon whose max_read_bytes admits one of the three seeded
// readings but not all three, beside a local config with no bound at all.
func boundedFixture(t *testing.T) remoteFixture {
	t.Helper()
	return newRemoteFixtureWith(t, "UTC", func(t *testing.T, f remoteFixture, cfg *config.Config) {
		whole, _, err := cli(t, f.cfgPath, "timeline", "--json")
		if err != nil {
			t.Fatal(err)
		}
		// The three readings are alike, so a third of the whole, with room for
		// the array's own bytes, holds one and never three.
		cfg.MaxReadBytes = len(whole)/3 + 16
	})
}

// tooLarge requires err to be the daemon's 413 and returns it.
func tooLarge(t *testing.T, err error, what string) *client.Error {
	t.Helper()
	var refused *client.Error
	if !errors.As(err, &refused) || refused.Status != http.StatusRequestEntityTooLarge {
		t.Fatalf("%s: got %v, want a 413 from the daemon", what, err)
	}
	if refused.Code != "result_too_large" || refused.TooLarge == nil {
		t.Fatalf("%s: a 413 without code result_too_large and its measure: %+v", what, refused)
	}
	return refused
}

// A read over the daemon's max_read_bytes is refused as 413 with its measure and the
// narrowings still open; the same read narrowed under it is answered; the same read
// on local files has no bound.
func TestTimeline_OverTheBoundIsRefusedWithWhatToNarrowBy(t *testing.T) {
	t.Parallel()
	f := boundedFixture(t)

	_, _, err := cli(t, f.remoteCfg, "timeline")
	refused := tooLarge(t, err, "the whole read")
	if refused.TooLarge.Matched != 3 || refused.TooLarge.Bytes <= refused.TooLarge.MaxBytes {
		t.Errorf("measure = %+v, want 3 matched and bytes over max", refused.TooLarge)
	}
	if got := strings.Join(refused.TooLarge.Narrow, " "); got != "type from to at projection fields" {
		t.Errorf("narrow = %q, want every narrowing open on an unnarrowed read", got)
	}
	for _, want := range []string{"result too large", "3 entries", "narrow it by type, from, to, at, projection, fields"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal %q does not say %q", err, want)
		}
	}

	// Narrowed by type alone it is still every reading; the refusal says what is left.
	_, _, err = cli(t, f.remoteCfg, "timeline", "--type", "health.blood-pressure")
	if got := strings.Join(tooLarge(t, err, "narrowed by type").TooLarge.Narrow, " "); got != "from to at projection fields" {
		t.Errorf("narrow after a type = %q, want the window, snapshot and projection", got)
	}

	// January is one reading, within the bound, and the daemon answers it.
	if out, _, err := cli(t, f.remoteCfg, "timeline", "--to", "2026-01-31"); err != nil || !strings.Contains(out, "shown=1") {
		t.Errorf("a read within the bound: %v\n%s", err, out)
	}

	// The operator at their own files is never bounded.
	if out, _, err := cli(t, f.cfgPath, "timeline"); err != nil || !strings.Contains(out, "shown=3") {
		t.Errorf("local read: %v\n%s", err, out)
	}
}

// A publish over the bound is filed once, whole, and answered a page at a time
// (card 92): the CLI shows the first page and says where the rest is, the receipt
// shows whole over the daemon as it does locally, and it verifies.
func TestPublish_OverTheBoundPublishesOnceAndPages(t *testing.T) {
	t.Parallel()
	f := boundedFixture(t)

	out, stderr, err := cli(t, f.remoteCfg, "timeline", "--projection", "bp", "--publish", "--json")
	if err != nil {
		t.Fatalf("a publish over the bound was refused: %v", err)
	}
	if !strings.Contains(stderr, "showing facets 1 to 1 of 3") {
		t.Errorf("the publish does not say it shows the first page of three:\n%s", stderr)
	}
	if n := strings.Count(out, `"facet_id"`); n != 1 {
		t.Errorf("the first page carries %d entries, want the one that fits:\n%s", n, out)
	}
	// --json puts the read's own hashes on stderr first; the receipt follows "receipt".
	hash := receiptOf(t, stderr[strings.Index(stderr, ": receipt "):])
	if list, _, err := cli(t, f.remoteCfg, "receipt", "list"); err != nil || strings.Count(list, "sha256:") != 1 {
		t.Fatalf("want one receipt filed: %v\n%s", err, list)
	}
	whole := f.sameOutput(t, "receipt", "show", "--json", hash)
	if n := strings.Count(whole, `"facet_id"`); n != 3 {
		t.Errorf("show over the daemon read %d entries, want all 3 across its pages", n)
	}
	if out := f.sameOutput(t, "receipt", "verify", hash); !strings.Contains(out, "OK   reproduced") {
		t.Errorf("the paged publish did not verify:\n%s", out)
	}

	// --cursor shows the one page it names, and says where the next is.
	cfg, err := config.Load(f.remoteCfg)
	if err != nil {
		t.Fatal(err)
	}
	c, err := newClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	first, err := c.ReceiptPage(context.Background(), hash, "")
	if err != nil || first.Page == nil || first.Page.NextCursor == "" {
		t.Fatalf("first page: %v %+v", err, first.Page)
	}
	page, stderr, err := cli(t, f.remoteCfg, "receipt", "show", "--json", "--cursor", first.Page.NextCursor, hash)
	if err != nil {
		t.Fatalf("receipt show --cursor: %v", err)
	}
	if n := strings.Count(page, `"facet_id"`); n != 1 || !strings.Contains(stderr, "facets 2 to 2 of 3") ||
		!strings.Contains(stderr, "next cursor: ") {
		t.Errorf("--cursor showed %d entries, stderr:\n%s", n, stderr)
	}
}

// One facet over the bound alone is what no page can carry, so a publish holding one
// is still refused, naming the facet, before anything is filed.
func TestPublish_OneFacetOverTheBoundFilesNothing(t *testing.T) {
	t.Parallel()
	f := newRemoteFixtureWith(t, "UTC", func(t *testing.T, f remoteFixture, cfg *config.Config) {
		cfg.MaxReadBytes = 40
	})

	_, _, err := cli(t, f.remoteCfg, "timeline", "--projection", "bp", "--publish")
	refused := tooLarge(t, err, "a publish with a facet over the bound")
	if refused.TooLarge.Matched != 1 || refused.TooLarge.FacetID == "" || !strings.Contains(err.Error(), "alone") {
		t.Errorf("the refusal should name the one facet: %+v %v", refused.TooLarge, err)
	}
	if out, _, err := cli(t, f.remoteCfg, "receipt", "list"); err != nil || strings.Contains(out, "sha256:") {
		t.Fatalf("a refused publish filed a receipt: %v\n%s", err, out)
	}
}

// A receipt published locally, with no bound, is still read whole over the daemon,
// page by page, and still verifies there.
func TestReceipt_ShowOverTheBoundWalksEveryPage(t *testing.T) {
	t.Parallel()
	f := boundedFixture(t)

	_, stderr, err := cli(t, f.cfgPath, "timeline", "--projection", "bp", "--publish")
	if err != nil {
		t.Fatalf("local publish: %v", err)
	}
	hash := receiptOf(t, stderr)

	f.sameOutput(t, "receipt", "show", hash)
	if n := strings.Count(f.sameOutput(t, "receipt", "show", "--json", hash), `"facet_id"`); n != 3 {
		t.Errorf("show read %d entries, want 3", n)
	}
	if out, _, err := cli(t, f.remoteCfg, "receipt", "verify", hash); err != nil || !strings.Contains(out, "OK   reproduced") {
		t.Errorf("verify over the daemon: %v\n%s", err, out)
	}
}
