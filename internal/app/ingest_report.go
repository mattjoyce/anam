package app

import (
	"fmt"

	"github.com/mattjoyce/anam/internal/api"
	"github.com/mattjoyce/anam/internal/config"
	"github.com/mattjoyce/anam/internal/gate"
	"github.com/spf13/cobra"
)

// How an ingest is reported: a line per source, then the verdict. A local run and a
// run asked of the daemon print the same lines from the same value.

// remoteIngest asks the daemon for an ingest, waits for it, and reports it as a local
// run does: a line per source, then the verdict or the error.
func remoteIngest(cmd *cobra.Command, cfg config.Config) error {
	c, err := newClient(cfg)
	if err != nil {
		return err
	}
	op, err := c.Ingest(cmd.Context(), cfg.IngestTimeout)
	if err != nil {
		return err
	}
	for _, r := range op.Sources {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), sourceLine(r))
	}
	switch op.State {
	case api.StateSucceeded:
		if _, err = fmt.Fprintln(cmd.OutOrStdout(), op.Summary); err != nil || op.Mint == nil {
			return err
		}
		_, err = fmt.Fprintln(cmd.OutOrStdout(), mintLine(*op.Mint))
		return err
	case api.StateFailed:
		return fmt.Errorf("ingest %s on %s: %s", op.ID, c.Server(), op.Error)
	default:
		return fmt.Errorf("ingest %s on %s is still %s after %s; anam status shows it",
			op.ID, c.Server(), op.State, cfg.IngestTimeout)
	}
}

// mintLine renders the mint the daemon ran after an ingest. Its failure is a line,
// not an error: the ingest succeeded, and the next mint catches up.
func mintLine(m api.MintRun) string {
	if m.Error != "" {
		return "checkpoints: mint failed, reads replay the tail until the next one: " + m.Error
	}
	from := "folding the whole ledger"
	if !m.Full {
		from = fmt.Sprintf("from the run at W=%d", m.From)
	}
	return fmt.Sprintf("checkpoints: minted %d of %d buckets at W=%d, %s", m.Minted, m.Buckets, m.W, from)
}

// resultOf is one source's part of a run as the API reports it, whichever process ran
// it.
func resultOf(source string, offered int, p progress) api.SourceResult {
	r := api.SourceResult{
		Source:     source,
		Offered:    offered,
		Admitted:   p.tally[gate.OutcomeAdmitted],
		Superseded: p.tally[gate.OutcomeSuperseded],
		Unchanged:  p.tally[gate.OutcomeNoOp],
		Committed:  p.done,
	}
	if p.done > 0 {
		w := int64(p.w)
		r.W = &w
	}
	return r
}

// sourceLine renders one source's part of a run, naming itself partial when it is.
func sourceLine(r api.SourceResult) string {
	if r.Committed == 0 || r.W == nil {
		return fmt.Sprintf("%s: %d envelopes → nothing committed", r.Source, r.Offered)
	}
	s := fmt.Sprintf("%s: %d envelopes → admitted=%d superseded=%d no-op=%d  W=%d",
		r.Source, r.Offered, r.Admitted, r.Superseded, r.Unchanged, *r.W)
	if r.Committed < r.Offered {
		s += fmt.Sprintf("  (stopped after %d of %d — commit is idempotent, so re-run)", r.Committed, r.Offered)
	}
	return s
}
