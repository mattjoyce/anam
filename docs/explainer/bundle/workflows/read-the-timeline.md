---
type: workflow
title: Read the timeline
description: anam timeline --from 2026-01-01 --to 2026-02-01 against the daemon, answered by composing the last mint's checkpoints with the tail of events, or by a full fold when the checkpoint log cannot be trusted.
timestamp: 2026-10-11T00:00:00Z
resource: internal/read/timeline.go#Timeline
anchor: "case narrow(src, q, b):"
basis: code
tier: supporting
trigger: The operator runs `anam timeline --from 2026-01-01 --to 2026-02-01` with `ANAM_SERVER` set to the daemon.
steps:
  - actor: /components/cli.md
    caller: outside
    label: parse the flags into a timeline query
    action: "`timelineCmd` builds an `api.TimelineQuery{From, To, At, AsOf, W, Type, Projection, Explain}` and, because a server is configured, hands it to the client with the reader's zone."
    resource: internal/app/timeline.go#timelineCmd
    anchor: 'cmd.Flags().StringVar(&from, "from", ""'
  - actor: /components/client.md
    caller: /components/cli.md
    label: GET /v1/timeline with the query
    action: The client encodes the query as URL values, sends its bearer token (any role may read) and an `Anam-Timeout` header, and refuses an answer over `max_response_bytes` (64 MiB by default) with "narrow the read".
    resource: internal/client/client.go#Timeline
    anchor: 'http.MethodGet, "/v1/timeline", q.Values()'
  - actor: /components/daemon.md
    caller: /components/client.md
    label: a read source from read-only handles
    action: "`getTimeline` parses the query back, refuses zone `Local`, takes the shorter of its own timeout and the header, and builds `read.Source{Ledger: d.read, Checkpoints: d.readOps, Artifacts, Definitions}` from the read-only ledger and ops handles. No turn on the write queue."
    resource: internal/daemon/api.go#getTimeline
    anchor: "q, err := api.TimelineQueryOf(r.URL.Query())"
  - actor: /components/read.md
    caller: /components/daemon.md
    label: parse bounds and pick the narrow path
    action: "`--from` is inclusive; a bare `--to` date rolls to the next midnight and becomes an exclusive bound. With a window, no projection, no `--at` and no `--explain`, and checkpoints to hand, the read takes `readWindow`."
    resource: internal/read/timeline.go#Timeline
    anchor: "b, at, err := parseBoundsAt(q.From, q.To, q.At, q.Explain, loc)"
  - actor: /components/read.md
    label: the watermark is the head
    action: With no `--as-of` or `--w`, W is the ledger head. `--as-of` would translate an anam-time to a watermark by replaying the ledger; `--w` above the head is refused; giving both is refused.
    resource: internal/read/knowledge.go#watermarkOf
    anchor: "if know.asOf.IsZero() {"
  - actor: /components/mint.md
    caller: /components/read.md
    label: find the last run at or below W
    action: "`startingRun` checks the log is pinned to this ledger, is sealed (the seal's checkpoint count equals the rows), and has a `mint_run` at or below W, then reads the leaf checkpoints of that run and derives the layout from their ids."
    reads: [/data-structures/checkpoint-row.md]
    resource: internal/mint/read.go#startingRun
    anchor: "if sealed, err := log.LogSealed(ctx); err != nil || !sealed {"
  - actor: /components/fold.md
    caller: /components/read.md
    branch: unpinned, unsealed, no run, or weekly ids
    label: fold the ledger from seq 0
    action: "`ReadAt` replays every event to W, folds every facet's status, fetches the live versions and builds the whole value; the read then narrows it in Go and answers as below. The header's Live and Canonical come from this whole value."
    ends: true
    reads: [/data-structures/anam-event.md, /data-structures/facet-artifact.md]
    resource: internal/fold/read.go#ReadAt
    anchor: "events, err := l.Replay(ctx, 0, w)"
  - actor: /components/mint.md
    caller: /components/read.md
    branch: a sealed log with a run at or below W
    label: replay the tail and find dirty buckets
    action: "`ReadWindow` replays the events in (run, W] and, per touched facet, marks dirty the bucket its version sat in at the run and the bucket its version sits in at W, keeping the live entries to re-add."
    reads: [/data-structures/anam-event.md, /data-structures/checkpoint-row.md]
    resource: internal/mint/window.go#ReadWindow
    anchor: "tail, err := l.Replay(ctx, run, w)"
  - actor: /components/mint.md
    label: decode only shown or dirtied buckets
    action: A bucket the window does not overlap and the tail did not touch is read as hash-checked bytes and never decoded; its count comes from `checkpoint_size`. The January bucket is decoded, touched facets dropped and the tail's entries appended.
    reads: [/data-structures/checkpoint-value.md]
    resource: internal/mint/window.go#readBucket
    anchor: "if !need && !d.buckets[id] && sized {"
  - actor: /components/mint.md
    label: stream the whole hash, value the window
    action: The whole-person hash is streamed over every bucket's inner bytes joined in time order, so it equals the hash of the full value without decoding it. Only the overlapping buckets' entries become the window's value.
    reads: [/data-structures/fold-entry.md]
    resource: internal/mint/window.go#ReadWindow
    anchor: 'win.Hash = "sha256:" + hex.EncodeToString(h.Sum(nil))'
  - actor: /components/read.md
    label: filter the window, write the header
    action: Entries are kept when inside `[from, to)` and of the requested type; the header carries W, the live count and the whole-person hash as `Canonical`, and the version of each shown facet.
    resource: internal/read/timeline.go#describe
    anchor: "t.Live, t.Canonical = v.whole.Live, v.whole.Hash"
  - actor: /components/read.md
    caller: /components/daemon.md
    label: measure the answer against the bound
    action: "`FitTimeline` encodes entries as they will go on the wire and stops at the first one crossing the bound (512 KiB by default)."
    resource: internal/read/bound.go#FitTimeline
    anchor: "return fitQuery(t.Entries, limit, q)"
  - actor: /components/daemon.md
    branch: over the bound
    label: 413 naming the parameters to narrow by
    action: The refusal names how many matched, the bytes, the limit, and which of `type`, `from`, `to`, `at`, `projection` the query did not use. A local read is never bounded.
    ends: true
    resource: internal/daemon/api.go#writeReadError
    anchor: "Code: api.CodeTooLarge"
  - actor: /components/daemon.md
    branch: fits
    label: answer 200 with the timeline JSON
    action: "`writeJSON` encodes `api.Timeline{W, Live, Shown, Canonical, Entries, Versions, ...}` with HTML escaping off."
    writes: [/data-structures/timeline-response.md]
    resource: internal/daemon/api.go#writeJSON
    anchor: "enc.SetEscapeHTML(false)"
  - actor: /components/cli.md
    label: render the table in the reader's zone
    action: "`writeTimeline` prints `W=<w>  live=<n>` and one row per entry with fact-time shown in the configured zone; `--json` writes the entries to stdout and the header to stderr."
    ends: true
    reads: [/data-structures/timeline-response.md]
    resource: internal/app/timeline.go#writeTimeline
    anchor: 'fmt.Fprintln(w, "FACT TIME\tTYPE\tV\tSTATUS\tSOURCE REF\tID")'
---

The read is where the mint pays for itself: a window read decodes only the buckets it overlaps and the ones a late correction dirtied, and still reports the whole-person hash. The fallback to a full fold is not an error path but the oracle: every composed read is tested equal to it at every watermark.

Without a window (`anam timeline` bare) the read takes `mint.ReadAt`, which loads every leaf checkpoint and applies the tail. With `--projection` or `--at` the whole value is read and then pruned. With `--explain` the checkpoints are dropped on purpose so displaced versions can be listed.

Related: [any state is nearest checkpoint plus replay](/invariants/any-state-is-checkpoint-plus-replay.md), [reads over the daemon are bounded](/decisions/reads-over-the-daemon-are-bounded.md), [weekly width disables composed reads](/gaps/weekly-width-disables-composed-reads.md).
