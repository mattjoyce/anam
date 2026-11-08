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
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/presentation"
	"github.com/mattjoyce/anam/internal/store/ledger"
)

func TestInspectorPresentationThroughBothEntrypoints(t *testing.T) {
	t.Parallel()
	f := newRemoteFixtureWith(t, "UTC", func(t *testing.T, f remoteFixture, cfg *config.Config) {
		appendFile(t, f.cfgPath, "inspector:\n  domains: [{id: test, hue: 4}]\n  families: [{id: measures, label: 'Custom readings'}]\n  types: [{type: test.measure, shape: star}]\n")
		hue := 4
		cfg.Inspector = presentation.Spec{Domains: []presentation.DomainSpec{{ID: "test", Hue: &hue}}, Families: []presentation.Family{{ID: "measures", Label: "Custom readings"}}, Types: []presentation.Override{{Type: "test.measure", Shape: "star"}}}
	})
	local := standaloneInspector(t, f.cfgPath)
	ctx := context.Background()
	client := &http.Client{Timeout: time.Second}
	_, localBody := inspectorRequest(t, client, "GET", local+"/?view=whole&open=test", false)
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

// standaloneInspector waits for the listener before returning its loopback URL.
func standaloneInspector(t *testing.T, configPath string) string {
	t.Helper()
	// Reserve an ephemeral address, then release it for the standalone command's listener.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	root := newRoot(BuildInfo{})
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"--config", configPath, "--server", "", "inspect", "--listen", addr})
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
			_ = body
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("standalone never listened")
		case <-time.After(10 * time.Millisecond):
		}
	}
	return "http://" + addr
}

func inspectorRequest(t *testing.T, client *http.Client, method, target string, authorized bool) (int, string) {
	t.Helper()
	req, err := http.NewRequest(method, target, nil)
	if err != nil {
		t.Fatal(err)
	}
	if authorized {
		req.Header.Set("Authorization", "Bearer "+testToken)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, string(body)
}

func TestSpatialThroughBothEntrypoints(t *testing.T) {
	t.Parallel()
	for _, selected := range []string{"", "test.measure"} {
		t.Run("selector="+selected, func(t *testing.T) {
			t.Parallel()
			var id string
			f := newRemoteFixtureWith(t, "UTC", func(t *testing.T, f remoteFixture, cfg *config.Config) {
				appendFile(t, f.cfgPath, "inspector: {track_type: '"+selected+"'}\n")
				cfg.Inspector.TrackType = facet.Type(selected)
				id = seedSpatialEntrypoint(t, f.ledgerPath)
			})
			local := standaloneInspector(t, f.cfgPath)
			client := &http.Client{Timeout: 5 * time.Second}
			for _, base := range []string{f.server, local} {
				auth := base == f.server
				for _, method := range []string{"GET", "HEAD"} {
					for _, path := range []string{"/location", "/location/"} {
						code, _ := inspectorRequest(t, client, method, base+path, auth)
						want := 404
						if selected != "" && path == "/location" {
							want = 200
						}
						if code != want {
							t.Errorf("%s %s = %d, want %d", method, path, code, want)
						}
					}
				}
				for _, path := range []string{"/", "/facets", "/events", "/definitions", "/definition?type=test.measure", "/projection", "/select", "/facet?id=" + id} {
					code, body := inspectorRequest(t, client, "GET", base+path, auth)
					if code != 200 {
						t.Fatalf("%s = %d", path, code)
					}
					if strings.Contains(body, `href="/location"`) != (selected != "") {
						t.Errorf("nav on %s", path)
					}
					if strings.Contains(body, `class="track"`) != (selected != "" && strings.Contains(path, "/facet?")) {
						t.Errorf("facet track on %s", path)
					}
					if strings.Contains(path, "/facet?") && !strings.Contains(body, "151.2093") {
						t.Error("JSON unavailable")
					}
				}
			}
			// The daemon catch-all must still require operator authorization when disabled.
			if code, _ := inspectorRequest(t, client, "GET", f.server+"/location", false); code != 401 {
				t.Fatalf("unauthorized = %d", code)
			}
		})
	}
}

func seedSpatialEntrypoint(t *testing.T, path string) string {
	t.Helper()
	ctx := context.Background()
	l, err := ledger.Open(ctx, path, "matt")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()
	head, err := l.Head(ctx)
	if err != nil {
		t.Fatal(err)
	}
	events, err := l.Replay(ctx, 0, head)
	if err != nil {
		t.Fatal(err)
	}
	f, err := l.CurrentVersion(ctx, events[0].FacetID)
	if err != nil {
		t.Fatal(err)
	}
	f.ID = facet.NewID("entrypoint:track")
	f.Provenance.SourceRef = "entrypoint:track"
	f.FactTime = time.Date(2026, 10, 12, 2, 0, 0, 0, time.UTC)
	f.Payload, err = facet.Canonicalize([]byte(`{"device":"phone","points":[{"t":"2026-10-12T02:05:00Z","lat":-33.8,"lon":151.2093}]}`))
	if err != nil {
		t.Fatal(err)
	}
	f.Descriptor.ContentHash = facet.ContentHash(f.Payload)
	ev := facet.AnamEvent{Kind: facet.EventAdmitted, FacetID: f.ID, ArtifactVersion: 1, Seq: int64(head) + 1, AnamTime: time.Now()}
	if err := l.Admit(ctx, head, f, ev); err != nil {
		t.Fatal(err)
	}
	return f.ID
}
