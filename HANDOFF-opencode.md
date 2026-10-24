# Anam: handoff prompt for a coding agent (opencode, on the Mac)

Paste everything below the line into the agent.

---

You are joining Anam as a coding agent. Matt owns the project and reviews your work. You work on
this Mac in your own clones, through branches and pull requests. You never deploy, and you never
merge to master yourself.

## What Anam is

Anam is Matt's personal longitudinal record, written in Go. Importers post envelopes to a daemon.
An admission gate validates each envelope against a Definition (a JSON Schema manifest per facet
type) and appends it to an append-only SQLite ledger. Current state is a fold over the ledger. An
operational store, a second SQLite file, is derived by replaying the ledger. Projections decide
what a reader sees. The inspector and Workbench are web pages the daemon serves. The error kernel
is the ledger plus the gate's invariants. Everything else can be rebuilt.

## Repositories

Clone the three repos side by side, in one parent folder. The explainer check needs anam-importers
beside anam.

```sh
git clone git@github.com:mattjoyce/anam.git
git clone git@github.com:mattjoyce/anam-importers.git
git clone git@github.com:mattjoyce/anam-mcp.git
```

- **anam**: the daemon, gate, CLI and inspector (Go).
- **anam-importers**: the importers, which are Python ductile plugins (stdlib plus PyYAML).
- **anam-mcp**: the MCP adapter (Go).

Read `anam/AGENTS.md` before anything else. It is binding. Pay most attention to:

- the error kernel;
- that the ledger's triggers are never dropped or disabled;
- Go standards;
- tests over a real ledger and a real operational store under `t.TempDir()`;
- the prose rules for everything you write: no em dashes, plain sentences, no puffery.

Then skim:

- `docs/ARCHITECTURE.md`;
- `docs/explainer/bundle/decisions/where-a-distinction-is-encoded.md`, which says where a
  distinction belongs: type name, Definition meta, payload field, provenance, edge or projection.

## How to work

1. **One card, one branch, one pull request.** Name the branch `card-<n>-<short-slug>`. Cut it
   from an up-to-date `origin/master`. If a card spans repos, use the same branch name in each and
   open one PR per repo, each linking the others.
2. **The gate must pass before you open a PR.**
   - In anam, run `scripts/check.sh`. It runs gofmt, goimports, check-links, the explainer check,
     go vet, golangci-lint, gosec and `go test -race -shuffle=on`. Read its last line for
     `CHECK OK`. The explainer check needs anam-importers beside anam, or `EXPLAINER_IMPORTERS_DIR`.
   - In anam-importers, run `python3 -B -m unittest discover -s tests`. Set `ANAM_BIN` to an anam
     binary you built (`go build -o /tmp/anam ./cmd/anam`), so the tests that need a daemon run.
   - In anam-mcp, run `./scripts/check.sh` or the repo's check. It builds anam from `../anam`.
3. **Known gate traps.**
   - Never run two gates at once. golangci-lint refuses a parallel run, and the gate reports that
     as a lint failure.
   - gosec can stall for many minutes on a loop whose skip is a compound condition. Split the
     skip into plain `if` statements.
4. **Pull requests.**
   - `git push -u origin <branch>`, then `gh pr create --base master`.
   - The PR description says what changed and why, how it was tested (paste the gate's last
     lines), what a deploy needs, and anything you decided that Matt should check.
   - Matt keeps the kanban board on another machine. Your PR description is the card update, and
     he copies it across.
5. **Commits.** Write a conventional subject (`feat:`, `fix:`, `docs:`, `test:`), commit in
   logical chunks, and keep the tree building.

## Rules that are easy to break

- **Code work only.** You work on the Mac, away from the ThinkPad and the server. Anything that needs the
  live system, live data, the gateway or the server's scripts is the lead's, so write and test the code
  and say in the PR what the lead must run.

- **Do not touch the live system.** That means no SSH to the server, no requests to the live
  daemon, and no deploys.
- **Do not read or use any credential you find,** in `~/.config/secrets/`, `~/.config/anam/`, an
  `.env` file or anywhere else: no tokens, keys or passwords. No task in this queue needs one.
  Tests start their own daemons with their own throwaway tokens. If a task seems to need a real
  credential, stop and ask Matt.
- **Definitions are versioned.**
  - A registered (importer) Definition's body is immutable per version, so a change is a new
    version with the old body kept in the importer's `definitions/history/`.
  - A bundled Definition's identity and fact-time rule cannot change between versions (`def.Guard`).
  - A projection version's hash is in every receipt cut through it, so a changed projection is a
    new version, and the old one moves to history.
- **Deploy coupling.** When anam and an importer change a shared type together, say in the PR
  that they deploy together.
- **No card numbers in user-facing text.** That covers CLI help, error messages, HTTP responses,
  inspector pages, Definition and projection descriptions, MCP tool text and READMEs. Code
  comments and docs/explainer may cite cards.
- **Dependencies.** Adding a dependency to `go.mod`, or a Python module beyond the stdlib and
  PyYAML, is Matt's call. Ask in the PR before adding one.
- **No test hooks.** No package variable may be set from a test. A value a test needs to change
  is a config default.

## The queue

Take these in order. Open each one's PR before starting the next.

Status 2026-10-24: the earlier queue (the rebuild drill fixture, the gosec findings, the admission
hold, heart rate by the minute) is merged and live, as anam v0.574-85ea2fb and anam-importers
8fdb411. One item is open.

### 1. The garmin snapshot outgrows the gateway's stdout cap (anam-importers, card 229)

When garmin's weekly resend poll runs, it should reach Anam the way every other poll does, so a
gap in the Garmin archive is noticed and filled without anyone running a backfill by hand.

Branch `card-229-garmin-poll-windows`, one PR in anam-importers. Code and tests only; the live
pipeline change and the deploy are Matt's, so write the config change the PR needs into its
description.

#### What happened

Card 129 (heart rate by the minute, now live) made `poll` declare `health.heart-rate.hour` in its snapshot, the whole archive in one
response: 14,617 hour facets and 367 batches, 14.1 MB of JSON. Ductile reads a plugin's stdout
into a buffer capped at 10 MiB (`internal/dispatch/output_limit.go`, a constant, no config key)
and fails the job when it is exceeded. The live backfill on 2026-10-23 went round the gateway by
hand. `handle` (the last `lookback_days` after a GarminDB sync) is small and unaffected. The
response grows by about 7 MB a year, so trimming JSON whitespace does not solve it.

Two facts about the protocol that shape the fix:

- A plugin keeps no state and gives one response per invocation, so one `poll` cannot spread
  its output over several responses by itself. The way round that is already in use: a plugin
  may emit an event that a pipeline routes back to its own `handle` (`anam-concept2-after-fetch`
  does this: `on: anam_concept2.fetched`, `from_plugin: anam_concept2`, `uses: anam_concept2`).
- A snapshot is labelled per batch with `{"id", "part", "parts", "types"}`
  (`anam_importer.envelopes.snapshot_parts`), and `parts` is the total number of batches, so the
  total must be known before the first part is sent, and `snapshot_parts` already takes a
  `run_id` so several invocations can label parts of one snapshot.

#### The design

`poll` becomes a planner. It decodes the archive as it does now (that is the only way to know
the batch count; if you find a cheaper exact count, use it), then instead of returning the
batches it returns one small `anam_garmin.window` event per calendar month (or per N files,
whichever gives the simpler code) carrying the snapshot id, the total `parts`, the index of that
window's first part, and the window's bounds. `handle` learns a second payload shape: when the
triggering event is an `anam_garmin.window`, it shapes only that window's files and days, labels
its batches with the given id and part numbers, and emits `anam_garmin.batch` events as it does
today. A pipeline routes `anam_garmin.window` back to `anam_garmin` (`from_plugin: anam_garmin`),
and the existing `anam-garmin-submit` pipeline carries the batches on. The `health.new_data`
payload keeps today's behaviour. Every response is then a month or less, well under the cap.

Watch for:

- A window's batch count must match what the planner assumed, or the part numbers drift and the
  snapshot never completes. Cut windows and batches so the count is a pure function of the files
  in the window (the same code path in both phases, no sharing of state), and test that the
  parts of a planned snapshot are exactly 1..N with no gap and no repeat.
- The last part completes the snapshot and triggers Anam's absent report; windows may run in any
  order, so the completing part is whichever arrives last. That is fine for Anam, but a window
  that fails and is not retried leaves the snapshot open: say in the PR what the operator sees
  then (the job log) and that a re-run of `poll` starts a fresh snapshot.
- Facets that straddle a window boundary (a night of sleep, an activity) must land in exactly
  one window. Decide by the fact time the Definition uses as identity.
- Keep `health` unchanged; keep the `poll` response under a megabyte with a test that asserts an
  upper bound on its serialised size for a synthetic archive of a few thousand files.

If the design above grows beyond this sketch, stop and say so in the PR. The alternative is a
per-instance capture bound in ductile itself (Matt's project, not in this queue), and that is
Matt's call.

#### Done when

- `poll` over a synthetic archive the size of Matt's (about 8,000 monitoring files) returns a
  response under a megabyte and a set of window events whose parts cover 1..N exactly once.
- `handle` on a window event emits that window's batches with the snapshot label, and on a
  `health.new_data` event behaves as before, with tests for both.
- The suite passes against a daemon built from anam master, and the PR describes the pipeline
  entry the live `pipelines.yaml` needs.

## When you are unsure

Stop and say so in the PR, or ask Matt. Do not guess at:

- anything that changes what the ledger or the gate accept;
- any deploy;
- any new dependency.
