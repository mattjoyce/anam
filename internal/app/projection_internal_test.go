package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Card 125: the operator registers a template over the daemon from the CLI; the list
// and a read see it on both sides, since both read the same operational store.
func TestProjection_RegisterListShowOverTheDaemon(t *testing.T) {
	t.Parallel()
	f := newRemoteFixture(t)
	manifest := filepath.Join(f.dir, "bptest.v1.json")
	body := `{"name": "bptest", "version": 1, "description": "a test view of blood pressure",
  "types": ["health.blood-pressure"], "statuses": ["admitted", "endorsed"], "max_classification": "Sensitive",
  "from": "", "to": "", "shape": "log"}`
	if err := os.WriteFile(manifest, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, _, err := cli(t, f.cfgPath, "projection", "register", manifest); err == nil || !strings.Contains(err.Error(), "daemon") {
		t.Errorf("registering against local files: %v, want a refusal naming the daemon", err)
	}
	out, _, err := cli(t, f.remoteCfg, "projection", "register", manifest)
	if err != nil || !strings.Contains(out, "registered bptest v1  sha256:") {
		t.Fatalf("register: %v\n%s", err, out)
	}
	out, _, err = cli(t, f.remoteCfg, "projection", "register", manifest)
	if err != nil || !strings.Contains(out, "already held (unchanged) bptest v1") {
		t.Fatalf("register again: %v\n%s", err, out)
	}
	if err := os.WriteFile(manifest, []byte(strings.Replace(body, `"endorsed"`, `"admitted"`, 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := cli(t, f.remoteCfg, "projection", "register", manifest); err == nil || !strings.Contains(err.Error(), "different body") {
		t.Errorf("a changed body at v1: %v", err)
	}

	for _, cfg := range []string{f.remoteCfg, f.cfgPath} {
		out, _, err = cli(t, cfg, "projection", "list")
		if err != nil || !strings.Contains(out, "bptest") || !strings.Contains(out, "bp ") {
			t.Errorf("list with %s: %v\n%s", filepath.Base(cfg), err, out)
		}
		out, _, err = cli(t, cfg, "projection", "show", "bptest")
		if err != nil || !strings.Contains(out, `"name": "bptest"`) {
			t.Errorf("show with %s: %v\n%s", filepath.Base(cfg), err, out)
		}
	}
	remoteList, _, _ := cli(t, f.remoteCfg, "projection", "list")
	if !strings.Contains(remoteList, "registered") || !strings.Contains(remoteList, "bundled") {
		t.Errorf("the daemon's list names no origins:\n%s", remoteList)
	}
	if _, _, err := cli(t, f.remoteCfg, "projection", "show", "bptest", "2"); err == nil {
		t.Error("show bptest 2 found a version never registered")
	}

	// A read through the registered template is the same on both sides.
	if got := f.sameOutput(t, "timeline", "--projection", "bptest"); !strings.Contains(got, "health.blood-pressure") {
		t.Errorf("timeline --projection bptest served nothing:\n%s", got)
	}
}
