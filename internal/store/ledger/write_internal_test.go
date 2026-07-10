package ledger

import (
	"errors"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	ddbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/mattjoyce/anam/internal/facet"
)

// The error map's dark arms cannot be reached through a healthy store — ErrCorrupt
// is by definition the state the transaction exists to prevent — so they are
// exercised here with crafted cancellation reasons. The map is the kernel's contract:
// each arm is a distinct situation with a distinct recovery, and collapsing any two
// is the bug (§8).

func canceled(codes ...string) error {
	reasons := make([]ddbtypes.CancellationReason, len(codes))
	for i, c := range codes {
		if c != "" {
			reasons[i] = ddbtypes.CancellationReason{Code: aws.String(c)}
		} else {
			reasons[i] = ddbtypes.CancellationReason{Code: aws.String("None")}
		}
	}
	return fmt.Errorf("wrapped: %w", &ddbtypes.TransactionCanceledException{CancellationReasons: reasons})
}

func testFacetWithHash(hash string) *facet.Facet {
	return &facet.Facet{ID: "f-1", ArtifactVersion: 1, Descriptor: facet.Descriptor{ContentHash: hash}}
}

func TestMapCancellation_Arms(t *testing.T) {
	t.Parallel()
	l := &Ledger{}
	ccf := "ConditionalCheckFailed"

	tests := []struct {
		name        string
		err         error
		artifactPos int
		want        error
	}{
		{"stale: rec and counter both failed, artifact passed",
			canceled("", ccf, ccf), admitArtifactPos, ErrStaleWatermark},
		{"stale via Mark: counter failed",
			canceled("", ccf), markArtifactPos, ErrStaleWatermark},
		{"corrupt: rec failed while counter passed",
			canceled("", ccf, ""), admitArtifactPos, ErrCorrupt},
		{"corrupt via Mark: rec failed while counter passed",
			canceled(ccf, ""), markArtifactPos, ErrCorrupt},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := l.mapCancellation(tc.err, testFacetWithHash("sha256:aaa"), tc.artifactPos)
			if !errors.Is(got, tc.want) {
				t.Errorf("mapCancellation = %v, want %v", got, tc.want)
			}
		})
	}

	t.Run("a non-transaction error passes through wrapped", func(t *testing.T) {
		t.Parallel()
		got := l.mapCancellation(errors.New("network sadness"), nil, markArtifactPos)
		if errors.Is(got, ErrStaleWatermark) || errors.Is(got, ErrCorrupt) || errors.Is(got, ErrAlreadyWritten) {
			t.Errorf("plain error mis-typed: %v", got)
		}
	})

	t.Run("cancellation without any failed condition is surfaced, not swallowed", func(t *testing.T) {
		t.Parallel()
		if got := l.mapCancellation(canceled("", "", ""), nil, admitArtifactPos); got == nil {
			t.Error("want an error, got nil")
		}
	})
}

func TestDiverged_Arms(t *testing.T) {
	t.Parallel()

	item := func(hash string, payload []byte) ddbtypes.CancellationReason {
		m := map[string]ddbtypes.AttributeValue{}
		if hash != "" {
			m["content_hash"] = &ddbtypes.AttributeValueMemberS{Value: hash}
		}
		if payload != nil {
			m["payload"] = &ddbtypes.AttributeValueMemberB{Value: payload}
		}
		return ddbtypes.CancellationReason{Item: m}
	}

	t.Run("different hash is a lost update", func(t *testing.T) {
		t.Parallel()
		err := diverged(testFacetWithHash("sha256:aaa"), item("sha256:bbb", nil))
		if !errors.Is(err, ErrDivergentArtifact) {
			t.Errorf("got %v, want ErrDivergentArtifact", err)
		}
	})
	t.Run("same hash is the idempotent no-op", func(t *testing.T) {
		t.Parallel()
		err := diverged(testFacetWithHash("sha256:aaa"), item("sha256:aaa", nil))
		if !errors.Is(err, ErrAlreadyWritten) {
			t.Errorf("got %v, want ErrAlreadyWritten", err)
		}
	})
	t.Run("same hash but different bytes means the hash lied", func(t *testing.T) {
		t.Parallel()
		f := testFacetWithHash("sha256:aaa") // zero payload: Bytes() is empty
		err := diverged(f, item("sha256:aaa", []byte("other bytes")))
		if !errors.Is(err, ErrDivergentArtifact) {
			t.Errorf("got %v, want ErrDivergentArtifact", err)
		}
	})
	t.Run("an existing item with no hash cannot be trusted as our write", func(t *testing.T) {
		t.Parallel()
		err := diverged(testFacetWithHash("sha256:aaa"), item("", nil))
		if !errors.Is(err, ErrDivergentArtifact) {
			t.Errorf("got %v, want ErrDivergentArtifact", err)
		}
	})
	t.Run("no artifact in the transaction yet its position failed", func(t *testing.T) {
		t.Parallel()
		if err := diverged(nil, item("sha256:aaa", nil)); !errors.Is(err, ErrCorrupt) {
			t.Errorf("got %v, want ErrCorrupt", err)
		}
	})
}
