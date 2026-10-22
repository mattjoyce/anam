package app

import (
	"strings"
	"testing"
)

// The operator holds a source over the daemon, sees what is held, and releases it.
func TestHoldCommand(t *testing.T) {
	t.Parallel()
	f := newRemoteFixture(t)

	out, _, err := cli(t, f.remoteCfg, "hold", "garmin")
	if err != nil {
		t.Fatalf("hold: %v", err)
	}
	if !strings.Contains(out, "garmin") {
		t.Errorf("hold wrote %q, want the held source named", out)
	}

	out, _, err = cli(t, f.remoteCfg, "hold")
	if err != nil {
		t.Fatalf("hold: %v", err)
	}
	if !strings.Contains(out, "garmin") {
		t.Errorf("hold listed %q, want the held source named", out)
	}

	out, _, err = cli(t, f.remoteCfg, "hold", "--release", "garmin")
	if err != nil {
		t.Fatalf("hold --release: %v", err)
	}
	out, _, err = cli(t, f.remoteCfg, "hold")
	if err != nil {
		t.Fatalf("hold: %v", err)
	}
	if !strings.Contains(out, "(none)") {
		t.Errorf("hold listed %q, want no holds", out)
	}
}

// A hold needs the daemon: without one there is nothing to hold the admission of.
func TestHoldNeedsTheDaemon(t *testing.T) {
	t.Parallel()
	f := newRemoteFixture(t)
	_, _, err := cli(t, f.cfgPath, "hold", "garmin")
	if err == nil || !strings.Contains(err.Error(), "needs the daemon") {
		t.Fatalf("hold without a server: %v, want an error saying it needs the daemon", err)
	}
}
