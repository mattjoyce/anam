package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mattjoyce/anam/internal/api"
	"github.com/mattjoyce/anam/internal/config"
	"github.com/mattjoyce/anam/internal/daemon"
)

// The daemon's ingest is the CLI's ingest on held handles: a real export through the
// real gate into a real ledger, asked for through the daemon and reported per source
// with the W range it moved.
func TestDaemonIngestAdmitsThenFindsNothingNew(t *testing.T) {
	t.Parallel()
	f := newIngestFixture(t)
	importsPath := filepath.Join(f.dir, "imports.yaml")
	decl := fmt.Sprintf("imports:\n  - source: bp-csv\n    path: %s\n", f.bpPath)
	if err := os.WriteFile(importsPath, []byte(decl), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(f.cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Listen = "127.0.0.1:0"

	d, err := daemon.Open(context.Background(), cfg, daemon.Options{Ingest: daemonIngest(cfg, importsPath)})
	if err != nil {
		t.Fatalf("open daemon: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	ran := make(chan error, 1)
	go func() { ran <- d.Run(ctx) }()
	defer func() {
		cancel()
		if err := <-ran; err != nil {
			t.Errorf("run: %v", err)
		}
	}()

	first := ingestOnce(t, d)
	if len(first.Sources) != 1 || first.Sources[0].Admitted != 3 || first.Sources[0].Committed != 3 {
		t.Fatalf("first run: %+v", first.Sources)
	}
	if *first.WAfter <= *first.WBefore {
		t.Fatalf("first run did not move W: %d..%d", *first.WBefore, *first.WAfter)
	}

	second := ingestOnce(t, d)
	if second.Sources[0].Unchanged != 3 || *second.WAfter != *second.WBefore {
		t.Fatalf("second run changed the ledger: %+v W %d..%d", second.Sources, *second.WBefore, *second.WAfter)
	}
}

func ingestOnce(t *testing.T, d *daemon.Daemon) api.Operation {
	t.Helper()
	id, err := d.RequestIngest()
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		op, _ := d.Operation(id)
		switch op.State {
		case api.StateSucceeded:
			return op
		case api.StateFailed:
			t.Fatalf("ingest failed: %s", op.Error)
		}
		if time.Now().After(deadline) {
			t.Fatalf("ingest still %s", op.State)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
