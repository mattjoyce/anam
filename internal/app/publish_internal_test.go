package app

import (
	"strings"
	"testing"
)

// receiptOf is the receipt hash a publish names on stderr.
func receiptOf(t *testing.T, stderr string) string {
	t.Helper()
	for _, field := range strings.Fields(stderr) {
		if strings.HasPrefix(field, "sha256:") {
			return field
		}
	}
	t.Fatalf("no receipt named:\n%s", stderr)
	return ""
}

// A published read shows what the same read shows, files a receipt for exactly that,
// and the receipt verifies, locally and over the daemon alike.
func TestPublish_SameOverTheDaemonAndVerifies(t *testing.T) {
	t.Parallel()
	f := newRemoteFixtureIn(t, "Australia/Sydney")
	for _, narrowing := range [][]string{
		{"--from", "2026-02-10"},
		{"--at", "2026-02-10"},
		{"--type", "health.blood-pressure", "--to", "2026-02-10"},
		{"--type", "health.blood-pressure."},
		{},
	} {
		read := append([]string{"timeline", "--projection", "bp"}, narrowing...)
		shown := f.sameOutput(t, read...)
		published := f.sameOutput(t, append(read, "--publish")...)
		if published != shown {
			t.Errorf("%v: a publish shows other entries than the read\n--- read\n%s--- published\n%s", narrowing, shown, published)
		}

		_, stderr, err := cli(t, f.remoteCfg, append(read, "--publish")...)
		if err != nil {
			t.Fatalf("%v: publish: %v", narrowing, err)
		}
		hash := receiptOf(t, stderr)
		if out := f.sameOutput(t, "receipt", "verify", hash); !strings.Contains(out, "OK   reproduced") {
			t.Errorf("%v: the published receipt did not reproduce:\n%s", narrowing, out)
		}
	}
}

// A read at a W is the read Anam's knowledge gave then: at the head's W it is the
// head read, and the daemon reads it the same.
func TestTimeline_AtAW(t *testing.T) {
	t.Parallel()
	f := newRemoteFixture(t)
	head := f.sameOutput(t, "timeline")
	w := strings.TrimPrefix(strings.Fields(head)[0], "W=")
	if atW := f.sameOutput(t, "timeline", "--w", w); atW != head {
		t.Errorf("a read at the head's W differs from the head read\n--- head\n%s--- at W\n%s", head, atW)
	}
	if first := f.sameOutput(t, "timeline", "--w", "1"); !strings.Contains(first, "shown=1") {
		t.Errorf("a read at W=1 should show the first reading only:\n%s", first)
	}
}

// What cannot be published, or read, is refused as the reader's error on both sides.
func TestPublish_RefusalsAreTheSameOverTheDaemon(t *testing.T) {
	t.Parallel()
	f := newRemoteFixture(t)
	for _, args := range [][]string{
		{"timeline", "--publish"},
		{"timeline", "--publish", "--projection", "bp", "--explain"},
		{"timeline", "--w", "1", "--as-of", "2026-01-01"},
		{"timeline", "--w", "999"},
		{"timeline", "--type", "health."},
		{"timeline", "--projection", "bp", "--publish", "--type", "health.blood-pressure.."},
	} {
		_, _, localErr := cli(t, f.cfgPath, args...)
		_, _, remoteErr := cli(t, f.remoteCfg, args...)
		if localErr == nil || remoteErr == nil || !strings.Contains(remoteErr.Error(), localErr.Error()) {
			t.Errorf("%v: local %v, remote %v", args, localErr, remoteErr)
		}
		if remoteErr != nil && !strings.Contains(remoteErr.Error(), "400") {
			t.Errorf("%v: remote error is not a 400: %v", args, remoteErr)
		}
	}
}
