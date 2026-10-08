package read

import (
	"context"

	"github.com/mattjoyce/anam/internal/api"
	"github.com/mattjoyce/anam/internal/store/ops"
)

// CheckpointLog is the operational store's log of minted checkpoints.
type CheckpointLog interface {
	Checkpoints(ctx context.Context) ([]ops.CheckpointRow, error)
}

// Checkpoints lists every checkpoint in log. db names the store for the reader.
func Checkpoints(ctx context.Context, log CheckpointLog, db string) (api.Checkpoints, error) {
	rows, err := log.Checkpoints(ctx)
	if err != nil {
		return api.Checkpoints{}, err
	}
	out := api.Checkpoints{DB: db, Checkpoints: make([]api.Checkpoint, 0, len(rows))}
	for _, r := range rows {
		out.Checkpoints = append(out.Checkpoints, api.Checkpoint{
			SpanID: r.SpanID, V: r.V, CoversW: r.CoversW, ValueHash: r.ValueHash, MintedAt: r.MintedAt,
		})
	}
	return out, nil
}
