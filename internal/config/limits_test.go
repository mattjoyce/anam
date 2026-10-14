package config_test

import (
	"errors"
	"testing"
	"time"

	"github.com/mattjoyce/anam/internal/config"
)

// Each limit is a key: the environment overrides its default, and a value out of
// range dies at load rather than at the first request that uses it (card 151).
func TestLimitsOverrideAndRange(t *testing.T) {
	tests := []struct {
		env, value string
		ok         bool
		got        func(config.Limits) any
		want       any
	}{
		{"ANAM_HTTP_READ_HEADER_TIMEOUT", "2s", true, func(l config.Limits) any { return l.HTTPReadHeaderTimeout }, 2 * time.Second},
		{"ANAM_HTTP_READ_TIMEOUT", "1m", true, func(l config.Limits) any { return l.HTTPReadTimeout }, time.Minute},
		{"ANAM_HTTP_WRITE_TIMEOUT", "0s", false, nil, nil},
		{"ANAM_HTTP_IDLE_TIMEOUT", "30s", true, func(l config.Limits) any { return l.HTTPIdleTimeout }, 30 * time.Second},
		{"ANAM_SHUTDOWN_GRACE", "-1s", false, nil, nil},
		{"ANAM_ACCEPT_WAIT", "1s", true, func(l config.Limits) any { return l.AcceptWait }, time.Second},
		{"ANAM_IMPORTER_WAIT", "20s", true, func(l config.Limits) any { return l.ImporterWait }, 20 * time.Second},
		{"ANAM_RETRY_AFTER", "500ms", false, nil, nil},
		{"ANAM_RETRY_AFTER", "1m", true, func(l config.Limits) any { return l.RetryAfter }, time.Minute},
		{"ANAM_MINT_QUIET", "10s", true, func(l config.Limits) any { return l.MintQuiet }, 10 * time.Second},
		{"ANAM_MAX_PAYLOAD_BYTES", "1048576", true, func(l config.Limits) any { return l.MaxPayloadBytes }, 1 << 20},
		{"ANAM_MAX_PAYLOAD_BYTES", "10", false, nil, nil},
		{"ANAM_COMMIT_RETRIES", "0", false, nil, nil},
		{"ANAM_MAX_MANIFEST_BYTES", "2147483648", false, nil, nil},
		{"ANAM_MAX_DOCUMENT_BYTES", "268435456", true, func(l config.Limits) any { return l.MaxDocumentBytes }, 256 << 20},
		{"ANAM_SNAPSHOT_TTL", "1s", false, nil, nil},
		{"ANAM_SNAPSHOT_TTL", "72h", true, func(l config.Limits) any { return l.SnapshotTTL }, 72 * time.Hour},
		{"ANAM_SNAPSHOT_KEEP", "0", false, nil, nil},
		{"ANAM_SNAPSHOT_KEEP", "5", true, func(l config.Limits) any { return l.SnapshotKeep }, 5},
		{"ANAM_MAX_SNAPSHOT_PARTS", "0", false, nil, nil},
		{"ANAM_MAX_ABSENT_IDS", "0", true, func(l config.Limits) any { return l.MaxAbsentIDs }, 0},
		{"ANAM_MAX_ABSENT_JUDGED", "-5", false, nil, nil},
		{"ANAM_MAX_RESPONSE_BYTES", "134217728", true, func(l config.Limits) any { return l.MaxResponseBytes }, 128 << 20},
		{"ANAM_NETWORK_MARGIN", "0s", true, func(l config.Limits) any { return l.NetworkMargin }, time.Duration(0)},
		{"ANAM_INSPECT_CACHED_READS", "0", true, func(l config.Limits) any { return l.InspectCachedReads }, 0},
		{"ANAM_INSPECT_CACHED_READS", "5000", false, nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.env+"="+tt.value, func(t *testing.T) {
			t.Setenv("ANAM_PERSON_ID", "matt")
			t.Setenv(tt.env, tt.value)
			got, err := config.Load("")
			if !tt.ok {
				if !errors.Is(err, config.ErrInvalid) {
					t.Fatalf("Load() = %v, want ErrInvalid", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if g := tt.got(got.Limits); g != tt.want {
				t.Errorf("%s = %v, want %v", tt.env, g, tt.want)
			}
		})
	}
}

// The defaults Load resolves are the ones DefaultLimits names, and they pass their own
// range checks.
func TestDefaultLimits(t *testing.T) {
	t.Setenv("ANAM_PERSON_ID", "matt")
	got, err := config.Load("")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.Limits != config.DefaultLimits() {
		t.Errorf("Load().Limits = %+v, want %+v", got.Limits, config.DefaultLimits())
	}
	if err := config.DefaultLimits().Validate(); err != nil {
		t.Errorf("DefaultLimits().Validate() = %v", err)
	}
}
