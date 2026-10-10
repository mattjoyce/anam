package app

import (
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
	if got := strings.Join(refused.TooLarge.Narrow, " "); got != "type from to at projection" {
		t.Errorf("narrow = %q, want every narrowing open on an unnarrowed read", got)
	}
	for _, want := range []string{"result too large", "3 entries", "narrow it by type, from, to, at, projection"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal %q does not say %q", err, want)
		}
	}

	// Narrowed by type alone it is still every reading; the refusal says what is left.
	_, _, err = cli(t, f.remoteCfg, "timeline", "--type", "health.blood-pressure")
	if got := strings.Join(tooLarge(t, err, "narrowed by type").TooLarge.Narrow, " "); got != "from to at projection" {
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

// A publish over the bound is refused before it files anything: no receipt appears,
// and a narrower publish of the same projection goes through and verifies.
func TestPublish_OverTheBoundFilesNothing(t *testing.T) {
	t.Parallel()
	f := boundedFixture(t)

	_, _, err := cli(t, f.remoteCfg, "timeline", "--projection", "bp", "--publish")
	refused := tooLarge(t, err, "the whole publish")
	if got := strings.Join(refused.TooLarge.Narrow, " "); got != "type from to at" {
		t.Errorf("narrow = %q, want what a projected read can still add", got)
	}
	if out, _, err := cli(t, f.remoteCfg, "receipt", "list"); err != nil || strings.Contains(out, "sha256:") {
		t.Fatalf("a refused publish filed a receipt: %v\n%s", err, out)
	}

	_, stderr, err := cli(t, f.remoteCfg, "timeline", "--projection", "bp", "--to", "2026-01-31", "--publish")
	if err != nil {
		t.Fatalf("a publish within the bound was refused: %v", err)
	}
	if out := f.sameOutput(t, "receipt", "verify", receiptOf(t, stderr)); !strings.Contains(out, "OK   reproduced") {
		t.Errorf("the narrower publish did not verify:\n%s", out)
	}
}

// A receipt published locally, with no bound, can be too large to show over the
// daemon; it is refused with where to read it, and it still verifies there.
func TestReceipt_ShowOverTheBoundIsRefusedVerifyIsNot(t *testing.T) {
	t.Parallel()
	f := boundedFixture(t)

	_, stderr, err := cli(t, f.cfgPath, "timeline", "--projection", "bp", "--publish")
	if err != nil {
		t.Fatalf("local publish: %v", err)
	}
	hash := receiptOf(t, stderr)

	_, _, err = cli(t, f.remoteCfg, "receipt", "show", hash)
	refused := tooLarge(t, err, "show over the daemon")
	if refused.TooLarge.Narrow != nil || !strings.Contains(err.Error(), "locally") {
		t.Errorf("a receipt's refusal should say to show it locally, not what to narrow: %v", err)
	}
	if out, _, err := cli(t, f.remoteCfg, "receipt", "verify", hash); err != nil || !strings.Contains(out, "OK   reproduced") {
		t.Errorf("verify over the daemon: %v\n%s", err, out)
	}
	if _, _, err := cli(t, f.cfgPath, "receipt", "show", hash); err != nil {
		t.Errorf("show locally: %v", err)
	}
}
