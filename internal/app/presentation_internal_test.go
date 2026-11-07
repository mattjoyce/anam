package app

import (
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/mattjoyce/anam/internal/config"
	"github.com/mattjoyce/anam/internal/presentation"
)

func TestInspectorPresentationThroughBothEntrypoints(t *testing.T) {
	t.Parallel()
	f := newRemoteFixtureWith(t, "UTC", func(t *testing.T, f remoteFixture, cfg *config.Config) {
		appendFile(t, f.cfgPath, "inspector:\n  domains: [{id: test, hue: 4}]\n  families: [{id: measures, label: 'Custom readings'}]\n  types: [{type: test.measure, shape: star}]\n")
		hue := 4
		cfg.Inspector = presentation.Spec{Domains: []presentation.DomainSpec{{ID: "test", Hue: &hue}}, Families: []presentation.Family{{ID: "measures", Label: "Custom readings"}}, Types: []presentation.Override{{Type: "test.measure", Shape: "star"}}}
	})
	// Reserve an ephemeral address, then release it for the standalone command's listener.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	root := newRoot(BuildInfo{})
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"--config", f.cfgPath, "--server", "", "inspect", "--listen", addr})
	done := make(chan error, 1)
	go func() { done <- root.ExecuteContext(ctx) }()
	t.Cleanup(func() {
		cancel()
		if err := <-done; err != nil {
			t.Errorf("standalone: %v", err)
		}
	})
	local := "http://" + addr + "/?view=whole&open=test"
	client := &http.Client{Timeout: time.Second}
	var localBody string
	for {
		req, err := http.NewRequestWithContext(ctx, "GET", local, nil)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := client.Do(req)
		if err == nil {
			body, readErr := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if readErr != nil {
				t.Fatal(readErr)
			}
			if resp.StatusCode != 200 {
				t.Fatalf("standalone = %d %s", resp.StatusCode, body)
			}
			localBody = string(body)
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("standalone never listened")
		case <-time.After(10 * time.Millisecond):
		}
	}
	req, err := http.NewRequestWithContext(ctx, "GET", f.server+"/?view=whole&open=test", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+testToken)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatal(resp.StatusCode)
	}
	for _, page := range []string{localBody, string(body)} {
		for _, want := range []string{"Custom readings", `class="h4"`, `M8.0 2.2L`} {
			if !strings.Contains(page, want) {
				t.Errorf("entrypoint ignored presentation %q", want)
			}
		}
	}
}

func TestInvalidInspectorFailsBeforeStandaloneListener(t *testing.T) {
	t.Parallel()
	f := newLedgerFixture(t)
	appendFile(t, f.cfgPath, "inspector: {families: [{id: bad, hue: 2}]}\n")
	// An invalid address would fail only after config and ledger loading. The schema
	// error must win, before Serve can try to listen.
	_, _, err := cli(t, f.cfgPath, "--server", "", "inspect", "--listen", "invalid-address")
	if err == nil || !strings.Contains(err.Error(), "/inspector/families/0/hue") {
		t.Fatalf("startup = %v", err)
	}
}
