package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	decl := fmt.Sprintf("imports:\n  - source: records\n    path: %s\n", f.recordsDir)
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

	// Card 61: an ingest that moved W is followed by a mint at the new head, so a read
	// after it starts from checkpoints with no tail.
	if m := first.Mint; m == nil || m.Error != "" || m.W != *first.WAfter || m.Minted == 0 || m.Minted != m.Buckets {
		t.Fatalf("first run's mint: %+v", first.Mint)
	}

	second := ingestOnce(t, d)
	if second.Sources[0].Unchanged != 3 || *second.WAfter != *second.WBefore {
		t.Fatalf("second run changed the ledger: %+v W %d..%d", second.Sources, *second.WBefore, *second.WAfter)
	}
	if second.Mint != nil {
		t.Fatalf("a run that changed nothing minted: %+v", second.Mint)
	}

	// A correction moves W again; the mint after it starts from a run, not from
	// genesis, and reaches the new head. The run is the first ingest's, unless the
	// mint worker got there first: the daemon's startup check can ask it for a mint
	// that lands between this ingest's write and its own mint, which then starts from
	// the worker's run at the same head and has nothing left to cut. Both are correct.
	more := strings.Replace(bpFixture, "114.0", "115.0", 1)
	if err := os.WriteFile(f.bpPath, []byte(more), 0o600); err != nil {
		t.Fatal(err)
	}
	third := ingestOnce(t, d)
	if m := third.Mint; m == nil || m.Error != "" || m.Full || m.W != *third.WAfter ||
		(m.From != *first.WAfter && m.From != *third.WAfter) {
		t.Fatalf("third run's mint: %+v (first W %d)", third.Mint, *first.WAfter)
	}
}

// A mint that fails after an ingest is reported on the operation and fails nothing:
// what the ingest admitted is in the ledger whatever the mint did.
func TestDaemonIngestSucceedsWhenItsMintFails(t *testing.T) {
	t.Parallel()
	f := newIngestFixture(t)
	importsPath := filepath.Join(f.dir, "imports.yaml")
	decl := fmt.Sprintf("imports:\n  - source: records\n    path: %s\n", f.recordsDir)
	if err := os.WriteFile(importsPath, []byte(decl), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(f.cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Listen = "127.0.0.1:0"
	// A file where the artifact directory should be: every Put fails.
	cfg.ArtifactDir = filepath.Join(f.dir, "not-a-dir")
	if err := os.WriteFile(cfg.ArtifactDir, nil, 0o600); err != nil {
		t.Fatal(err)
	}

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

	op := ingestOnce(t, d)
	if op.State != api.StateSucceeded || op.Changed != 3 {
		t.Fatalf("ingest: %s, changed %d, error %q", op.State, op.Changed, op.Error)
	}
	if op.Mint == nil || op.Mint.Error == "" {
		t.Fatalf("a failed mint was not reported: %+v", op.Mint)
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
