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

	"github.com/mattjoyce/anam/internal/config"
	"github.com/mattjoyce/anam/internal/daemon"
)

const testToken = "0123456789abcdef0123456789abcdef"

// remoteFixture is a real ledger holding three readings, a daemon serving it on
// loopback behind a token, and two configs: one that opens the files, and one that
// asks the daemon and names no file that exists.
type remoteFixture struct {
	ingestFixture
	imports   string
	remoteCfg string
	server    string
}

func newRemoteFixture(t *testing.T) remoteFixture {
	t.Helper()
	f := remoteFixture{ingestFixture: newIngestFixture(t)}
	f.imports = fmt.Sprintf("imports:\n  - source: bp-csv\n    path: %s\n", f.bpPath)
	if _, _, err := f.ingest(t, f.imports); err != nil {
		t.Fatalf("seed the ledger: %v", err)
	}
	// Both sides read the reader's zone from config; UTC keeps the expected output
	// independent of the machine running the test.
	appendFile(t, f.cfgPath, "zone: UTC\n")

	tokenPath := filepath.Join(f.dir, "token")
	if err := os.WriteFile(tokenPath, []byte(testToken+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(f.cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	cfg.TokenFile = tokenPath
	d, err := daemon.Open(context.Background(), cfg, daemon.Options{
		Ingest:  daemonIngest(cfg, filepath.Join(f.dir, "imports.yaml")),
		Version: "daemon-build",
	})
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
	remote := fmt.Sprintf("server: %s\ntoken_file: %s\nzone: UTC\nledger_path: %s\ndb_path: %s\nartifact_dir: %s\n",
		f.server, tokenPath, filepath.Join(nowhere, "ledger.db"), filepath.Join(nowhere, "tracking.db"),
		filepath.Join(nowhere, "artifacts"))
	if err := os.WriteFile(f.remoteCfg, []byte(remote), 0o600); err != nil {
		t.Fatal(err)
	}
	return f
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
