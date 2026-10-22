package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mattjoyce/anam/internal/api"
	"github.com/mattjoyce/anam/internal/client"
	"github.com/mattjoyce/anam/internal/config"
	"github.com/mattjoyce/anam/internal/daemon"
)

const testToken = "0123456789abcdef0123456789abcdef"

// remoteFixture is a real ledger holding three readings, a daemon serving it on
// loopback behind a token, and two configs: one that opens the files, and one that
// asks the daemon and names no file that exists.
type remoteFixture struct {
	ledgerFixture
	remoteCfg string
	server    string
}

func newRemoteFixture(t *testing.T) remoteFixture {
	t.Helper()
	return newRemoteFixtureIn(t, "UTC")
}

// newRemoteFixtureIn is newRemoteFixture with both configs reading in zone.
func newRemoteFixtureIn(t *testing.T, zone string) remoteFixture {
	t.Helper()
	return newRemoteFixtureWith(t, zone, nil)
}

// newRemoteFixtureWith is newRemoteFixtureIn with the daemon's config amended by
// tweak before the daemon opens, once the ledger is seeded. The local config is not
// amended: what the daemon alone enforces is tested against a local read that does
// not.
func newRemoteFixtureWith(t *testing.T, zone string, tweak func(t *testing.T, f remoteFixture, cfg *config.Config)) remoteFixture {
	t.Helper()
	f := remoteFixture{ledgerFixture: newLedgerFixture(t)}
	f.admit(t, bpReadings)
	// Both sides read the reader's zone from config; UTC keeps the expected output
	// independent of the machine running the test.
	appendFile(t, f.cfgPath, "zone: "+zone+"\n")

	tokenPath := filepath.Join(f.dir, "token")
	if err := os.WriteFile(tokenPath, []byte(testToken+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(f.cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	cfg.TokenFile = tokenPath
	if tweak != nil {
		tweak(t, f, &cfg)
	}
	d, err := daemon.Open(context.Background(), cfg, daemon.Options{Version: "daemon-build"})
	if err != nil {
		t.Fatalf("open daemon: %v", err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	ran := make(chan error, 1)
	go func() { ran <- d.Serve(ctx, ln) }()
	t.Cleanup(func() {
		cancel()
		if err := <-ran; err != nil {
			t.Errorf("daemon: %v", err)
		}
	})
	f.server = "http://" + ln.Addr().String()

	// No person and no file that exists: if the CLI opened anything local it would fail.
	nowhere := filepath.Join(f.dir, "not-here")
	f.remoteCfg = filepath.Join(f.dir, "remote.yaml")
	remote := fmt.Sprintf("server: %s\ntoken_file: %s\nzone: %s\nledger_path: %s\ndb_path: %s\n",
		f.server, tokenPath, zone, filepath.Join(nowhere, "ledger.db"), filepath.Join(nowhere, "tracking.db"))
	if err := os.WriteFile(f.remoteCfg, []byte(remote), 0o600); err != nil {
		t.Fatal(err)
	}
	f.awaitIndex(t, tokenPath)
	return f
}

// awaitIndex waits for the daemon's first write, which catches its index up to the
// ledger, so a test of changes compares like with like.
func (f remoteFixture) awaitIndex(t *testing.T, tokenPath string) {
	t.Helper()
	c, err := client.New(f.server, tokenPath, 5*time.Second, config.DefaultNetworkMargin, config.DefaultMaxResponseBytes)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		ch, err := c.Changes(context.Background(), api.ChangesQuery{Since: "2000-01-01", Zone: "UTC"})
		if err == nil && ch.IndexSeq == ch.W {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("the daemon's index never caught up: %+v %v", ch, err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func appendFile(t *testing.T, path, s string) {
	t.Helper()
	fh, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = fh.Close() }()
	if _, err := fh.WriteString(s); err != nil {
		t.Fatal(err)
	}
}

// cli runs the whole command tree with cfg as its config file and returns what it
// wrote to stdout and stderr.
func cli(t *testing.T, cfg string, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	root := newRoot(BuildInfo{Version: "cli-build", GitCommit: "abc", BuildDate: "today"})
	var out, errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetArgs(append([]string{"--config", cfg}, args...))
	err = root.ExecuteContext(context.Background())
	return out.String(), errOut.String(), err
}

// sameOutput runs args against the local files and against the daemon and requires
// the same stdout: one value, one renderer, whichever side produced it.
func (f remoteFixture) sameOutput(t *testing.T, args ...string) string {
	t.Helper()
	local, _, err := cli(t, f.cfgPath, args...)
	if err != nil {
		t.Fatalf("local %v: %v", args, err)
	}
	remote, _, err := cli(t, f.remoteCfg, args...)
	if err != nil {
		t.Fatalf("remote %v: %v", args, err)
	}
	if local != remote {
		t.Fatalf("%v differs between local and remote\n--- local\n%s--- remote\n%s", args, local, remote)
	}
	return local
}

func TestStatus_AsksTheDaemon(t *testing.T) {
	t.Parallel()
	f := newRemoteFixture(t)

	local, _, err := cli(t, f.cfgPath, "status")
	if err != nil {
		t.Fatal(err)
	}
	remote, _, err := cli(t, f.remoteCfg, "status")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"(person matt)", "W       3"} {
		if !strings.Contains(local, want) || !strings.Contains(remote, want) {
			t.Errorf("want %q in both\n--- local\n%s--- remote\n%s", want, local, remote)
		}
	}
	if !strings.Contains(local, "cli-build, local files") {
		t.Errorf("local status names the wrong build:\n%s", local)
	}
	if !strings.Contains(remote, "daemon-build, daemon at "+f.server) || !strings.Contains(remote, "since") {
		t.Errorf("remote status does not describe the daemon:\n%s", remote)
	}
}

func TestVersion_NamesTheDaemonsBuild(t *testing.T) {
	t.Parallel()
	f := newRemoteFixture(t)

	local, _, err := cli(t, f.cfgPath, "version")
	if err != nil {
		t.Fatal(err)
	}
	if local != "anam cli-build (commit abc, built today)\n" {
		t.Errorf("local version = %q", local)
	}
	remote, _, err := cli(t, f.remoteCfg, "version")
	if err != nil {
		t.Fatal(err)
	}
	if want := local + "daemon daemon-build at " + f.server + "\n"; remote != want {
		t.Errorf("remote version = %q, want %q", remote, want)
	}
}

func TestInspect_PrintsTheDaemonsInspector(t *testing.T) {
	t.Parallel()
	f := newRemoteFixture(t)
	out, _, err := cli(t, f.remoteCfg, "inspect")
	if err != nil {
		t.Fatal(err)
	}
	if out != f.server+"/\n" {
		t.Errorf("inspect = %q, want the daemon's URL", out)
	}
}

// --server wins over the config file, and names a daemon that is not there.
func TestAnUnreachableServerIsAnErrorThatNamesIt(t *testing.T) {
	t.Parallel()
	f := newRemoteFixture(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	gone := "http://" + ln.Addr().String()
	_ = ln.Close()

	_, _, err = cli(t, f.remoteCfg, "--server", gone, "status")
	if err == nil || !strings.Contains(err.Error(), gone) {
		t.Fatalf("status against a closed port: %v, want an error naming %s", err, gone)
	}
}

func TestAWrongTokenIsRefused(t *testing.T) {
	t.Parallel()
	f := newRemoteFixture(t)
	bad := filepath.Join(f.dir, "bad-token")
	if err := os.WriteFile(bad, []byte(strings.Repeat("x", 32)), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := filepath.Join(f.dir, "bad.yaml")
	if err := os.WriteFile(cfg, []byte(fmt.Sprintf("server: %s\ntoken_file: %s\n", f.server, bad)), 0o600); err != nil {
		t.Fatal(err)
	}
	_, _, err := cli(t, cfg, "status")
	if err == nil || !strings.Contains(err.Error(), "refused the token") {
		t.Fatalf("status with a wrong token: %v", err)
	}
}

// A command not routed to the daemon refuses to open local files while a server is
// configured, rather than reading files the daemon owns.
func TestUnroutedCommandsRefuseWithAServer(t *testing.T) {
	t.Parallel()
	f := newRemoteFixture(t)
	for _, args := range [][]string{{"init"}, {"serve", "meds"}, {"retract", "x"}, {"daemon"}} {
		_, _, err := cli(t, f.remoteCfg, args...)
		if err == nil || !strings.Contains(err.Error(), "opens local files") {
			t.Errorf("%v with a server: %v, want a refusal", args, err)
		}
	}
	var target *os.PathError
	if _, err := os.Stat(filepath.Join(f.dir, "not-here")); !errors.As(err, &target) {
		t.Errorf("a refused command created the local data directory: %v", err)
	}
}

func TestTimeline_SameOverTheDaemon(t *testing.T) {
	t.Parallel()
	f := newRemoteFixtureIn(t, "Australia/Sydney")
	for _, args := range [][]string{
		{"timeline"},
		{"timeline", "--json"},
		{"timeline", "--from", "2026-02-10"},
		{"timeline", "--to", "2026-02-10"},
		{"timeline", "--at", "2026-02-10"},
		{"timeline", "--type", "health.blood-pressure"},
		{"timeline", "--type", "no.such-type", "--json"},
		{"timeline", "--as-of", "2000-01-01"},
		{"timeline", "--explain"},
		{"timeline", "--explain", "--from", "2026-02-10T00:00:00Z"},
	} {
		out := f.sameOutput(t, args...)
		if args[len(args)-1] == "timeline" && !strings.Contains(out, "shown=3") {
			t.Errorf("timeline shows the wrong readings:\n%s", out)
		}
	}
}

// A query the daemon cannot read is the reader's error, worded as it is locally.
func TestTimeline_ABadQueryIsTheSameErrorOverTheDaemon(t *testing.T) {
	t.Parallel()
	f := newRemoteFixture(t)
	for _, args := range [][]string{
		{"timeline", "--from", "last tuesday"},
		{"timeline", "--at", "2026-01-01", "--to", "2026-01-02"},
		{"timeline", "--projection", "no-such-projection"},
		{"changes", "--since", "yesterday"},
		{"coverage", "--projection", "no-such-projection"},
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

func TestChanges_SameOverTheDaemon(t *testing.T) {
	t.Parallel()
	f := newRemoteFixture(t)
	out := f.sameOutput(t, "changes", "--since", "2000-01-01")
	if !strings.Contains(out, "3 change(s)") {
		t.Errorf("changes missed the readings:\n%s", out)
	}
	f.sameOutput(t, "changes", "--since", "2000-01-01", "--until", "2000-01-02")
}

// Card 90: coverage is one value, rendered alike from the local files and the daemon.
func TestCoverage_SameOverTheDaemon(t *testing.T) {
	t.Parallel()
	f := newRemoteFixture(t)
	out := f.sameOutput(t, "coverage")
	if !strings.Contains(out, "health.blood-pressure  3") {
		t.Errorf("coverage missed the readings:\n%s", out)
	}
	for _, args := range [][]string{
		{"coverage", "--json"},
		{"coverage", "--w", "1"},
		{"coverage", "--as-of", "2000-01-01"},
		{"coverage", "--projection", "bp"},
	} {
		f.sameOutput(t, args...)
	}
}

// The daemon has no zone of its own, so a query that names Local is refused rather
// than read in the daemon's.
func TestTheDaemonRefusesALocalZone(t *testing.T) {
	t.Parallel()
	f := newRemoteFixture(t)
	c, err := client.New(f.server, filepath.Join(f.dir, "token"), 5*time.Second, config.DefaultNetworkMargin, config.DefaultMaxResponseBytes)
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Timeline(context.Background(), api.TimelineQuery{Zone: "Local"})
	if err == nil || !strings.Contains(err.Error(), "IANA") {
		t.Fatalf("timeline in zone Local: %v", err)
	}
}

func TestReceipts_SameOverTheDaemon(t *testing.T) {
	t.Parallel()
	f := newRemoteFixture(t)
	f.sameOutput(t, "receipt", "list")

	// A serve publishes a receipt into the artifact directory the daemon reads.
	if _, _, err := cli(t, f.cfgPath, "serve", "bp"); err != nil {
		t.Fatalf("serve bp: %v", err)
	}
	list := f.sameOutput(t, "receipt", "list")
	hash := ""
	for _, field := range strings.Fields(list) {
		if strings.HasPrefix(field, "sha256:") {
			hash = field
		}
	}
	if hash == "" {
		t.Fatalf("no receipt listed:\n%s", list)
	}
	f.sameOutput(t, "receipt", "show", hash)
	f.sameOutput(t, "receipt", "show", hash, "--json")
	if out := f.sameOutput(t, "receipt", "verify", hash); !strings.Contains(out, "OK   reproduced") {
		t.Errorf("verify did not reproduce:\n%s", out)
	}

	for _, args := range [][]string{
		{"receipt", "show", "sha256:" + strings.Repeat("0", 64)},
		{"receipt", "verify", "sha256:" + strings.Repeat("0", 64)},
		{"receipt", "show", "not-a-digest"},
	} {
		_, _, localErr := cli(t, f.cfgPath, args...)
		_, _, remoteErr := cli(t, f.remoteCfg, args...)
		if localErr == nil || remoteErr == nil || !strings.Contains(remoteErr.Error(), localErr.Error()) {
			t.Errorf("%v: local %v, remote %v", args, localErr, remoteErr)
		}
	}
}

func TestCheckpoints_SameOverTheDaemon(t *testing.T) {
	t.Parallel()
	f := newRemoteFixture(t)
	f.sameOutput(t, "checkpoint", "list")
	f.sameOutput(t, "checkpoint", "--dry-run")

	// Minted over the daemon, then nothing left to mint by either path.
	minted, _, err := cli(t, f.remoteCfg, "checkpoint")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(minted, "minted") || strings.Contains(minted, "minted 0 of") {
		t.Errorf("the daemon minted nothing:\n%s", minted)
	}
	f.sameOutput(t, "checkpoint", "list")
	if out := f.sameOutput(t, "checkpoint", "--dry-run"); !strings.Contains(out, "would mint (dry run — nothing written) 0 of") {
		t.Errorf("a second mint would mint again:\n%s", out)
	}
	if out := f.sameOutput(t, "checkpoint", "--dry-run"); !strings.Contains(out, "from the run at W=") {
		t.Errorf("a mint after a run did not start from it:\n%s", out)
	}
	if out := f.sameOutput(t, "checkpoint", "--dry-run", "--full"); !strings.Contains(out, "0 of") ||
		!strings.Contains(out, "folding the whole ledger") {
		t.Errorf("a full mint after a run:\n%s", out)
	}
}
