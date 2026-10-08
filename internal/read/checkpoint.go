package read

import (
	"context"

	"github.com/mattjoyce/anam/internal/api"
	"github.com/mattjoyce/anam/internal/mint"
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

// Mint runs a checkpoint over d and reports it as a value. It writes, unless dryRun:
// call it from whatever may write the operational store and the artifacts.
func Mint(ctx context.Context, d mint.Deps, dryRun bool) (api.Mint, error) {
	res, err := mint.Run(ctx, d, dryRun)
	if err != nil {
		return api.Mint{}, err
	}
	out := api.Mint{W: int64(res.W), Written: res.Written, Outcomes: make([]api.MintOutcome, 0, len(res.Outcomes))}
	for _, o := range res.Outcomes {
		out.Outcomes = append(out.Outcomes, api.MintOutcome{
			SpanID: o.SpanID, V: o.V, CoversW: o.CoversW, ValueHash: o.ValueHash, Len: o.Len, Minted: o.Minted,
		})
	}
	return out, nil
}
