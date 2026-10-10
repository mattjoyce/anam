# The day's merges, audited (2026-10-10)

Hickey × Armstrong, After pass, over `66421ea..313f9c9`: every merge to master between 10:37 and
22:09 on 2026-10-10. Cards 54, 64, 70, 71, 72, 78, 80, 83, 84, 85, 86, 87. 104 files, about 4,300
lines added and 1,400 removed. The diff was split into four slices and each slice audited
independently, then collated here.

Only card 71 had a luminary pass of its own in this window (Naur × Procida, Tufte × Majors, in
`luminary-definitions-page-2026-10-10/`). The code of cards 54, 64, 78, 80, 83, 86 and 87 merged on
`scripts/check.sh` alone. AGENTS.md asks for the default pair on any non-trivial change; this file
is that pass, run after the fact.

## Gate

`scripts/check.sh` is green at `313f9c9`: gofmt, goimports, the substrate boundary, go vet,
golangci-lint (0 issues), gosec, and `go test -race -count=1 -shuffle=on -vet=all` over 29
packages. Coverage was not measured: the Go 1.25 install on the ThinkPad has no `covdata` tool, so
`go test -cover` fails before it runs.

Mechanics outside the gate: `internal/inspect/inspect.go` is 539 lines and `facetPage` is 92;
`internal/app/ingest.go` is 472 (pre-existing); `internal/serve/serve.go` and `internal/app/serve.go`
are at 394 and 397. `retractCmd` and `timelineCmd` are over 80 lines each.

## Findings, ranked

1. **Card 78, medium. A facet admitted incrementally during a multi-part resend can be reported
   absent.** `internal/read/snapshot.go:61` judges "untouched since" against `CompletedW` only;
   the `snapshot` table (`internal/store/ops/ops.go:204`) records `started_at` but no start
   watermark. A webhook-driven incremental admission that lands while a resend is in flight, for
   a facet the resend's earlier parts did not carry, has its last event at or below `CompletedW`
   and appears in the absent list. `anam retract --yes` then tombstones a facet the source holds,
   and a tombstone is forever. The existing test covers only a facet sent after completion. Fix:
   record the ledger head at the first part as `started_w`, and skip any facet whose last event
   is after it.

2. **Card 87, medium. `artifact.Put` truncates and writes in place, and publishing is its first
   unserialised writer.** `internal/store/artifact/artifact.go:121-133` is read-then-WriteFile,
   no temp file and rename. Mint runs on the write queue so this never raced. `postReceipt`
   (`internal/daemon/api.go:150`) deliberately takes no queue turn, so two concurrent publishes
   of the same value at the same W both pass the not-exists check and both truncate the same
   path. A verify reading in between sees a short file and reports `ErrCorrupt`. Fix: write to a
   temp name under the `os.Root` and rename.

3. **Card 87, medium. The narrowing predicate exists twice.** `internal/read/timeline.go:117`
   narrows the timeline with `bounds.admits`; `internal/serve/serve.go:218` (`Narrow`)
   reimplements the window, type and snapshot predicate for publish and verify. They agree today
   and `TestPublish_SameOverTheDaemonAndVerifies` pins it, but a change to one leaves the
   timeline showing one set and the receipt certifying another, with verify still passing. Fix:
   build the `receipt.Query` from `bounds` and narrow through one function.

4. **Cards 54 and 80, medium. `imports.example.yaml:10-13` still declares `bp-csv` and
   `withings`.** README says to copy it; a fresh operator gets `unknown source "bp-csv"` on the
   first ingest. `docs/pipeline.html:153,191` still shows the bp-csv worker too.

5. **Card 71, low. `payloadRows` swallows every failure into nil**
   (`internal/inspect/facetfields.go:31-46`). A Definition body that fails to parse makes the
   fields table vanish with no hint why. The definition page handles the same case with
   `ParseError`; the facet page should degrade visibly.

6. **Card 71, low. `heldDefinitions` gives the build's bundled body precedence over the stored
   body for the same (type, version)** (`internal/daemon/definitions.go:112-130`). The facet page
   says "as it was in force" but shows the build's words. Every manifest in this window bumped
   its version, so the hole is unguarded rather than hit. Stored should win, or the lint should
   refuse an unbumped edit.

7. **Card 78, low. `planFromSnapshot` type-asserts to `*ops.DB` and `read.FacetHistory`**
   (`internal/app/retract.go:171,186`) instead of declaring the narrow interface it needs. Fails
   at runtime with a message the operator cannot act on.

8. **Card 78, low. The `disposable` tier is not one ARCHITECTURE.md §8 names.** The snapshot
   tables are honest about coming back by resend, but a reader of §8 will not find the tier.

9. **Card 78, low. `read.Absent` runs one `FacetEvents` query per uncarried live facet.** A
   source that shrank by thousands makes thousands of queries under one request timeout on
   `GET /v1/snapshots/{id}`; the CLI is bounded by `--max-fraction`, the API is not.

10. **Card 87, low. The published header's projection hash is the template value's hash, not
    the narrowed served value's** (`internal/read/timeline.go:160`), and `Receipt.Span` is the
    template's span. Two claims name different values with no field saying so.

11. **Card 86, low. `PutDefinitions` never removes a dropped bundled type**
    (`internal/app/ingest.go:354`), so `health.measurement` v3 stays as a provisioned row. The
    takeover test depends on this, so it is intended, but nothing near the upsert says so.

12. **Card 71, low.** `ledger.Version` returning `ErrNotFound` for a prior version maps to 500
    (`internal/inspect/inspect.go:362`) where the current version maps to 404. `?before=abc` is
    silently the head (`inspect.go:493`). `typesOf` is duplicated between `inspect/definitions.go`
    and `def/schema.go`.

13. **Nits.** `internal/daemon/envelopes.go:77` calls `time.Now()` where the daemon elsewhere
    uses `d.now()`. The daemon's startup mint and the ingest's post-run mint race to the same
    head, and `daemon_internal_test.go:81` accepts either; both are correct and idempotent, but
    two minters of one chain have no named owner.

## Clean

- Card 64: the prefix claim holds because `fold.Snapshot` stops at `at`, and the oracle test
  compares composed to full across snapshot, window and as-of queries.
- Card 83: v3 facts still fold and read. The fact-time pointer and identity key are unchanged,
  the guard refuses a version that moves them, and no read path validates payloads against the
  current Definition. `bench/gen.go` emits a valid v4 body. The reshape script touches only YAML.
- Card 85: the bounded lists are pure functions over the fold rows; no cursor, no cache.
- Card 71: output goes through `html/template`, tests prove tags arrive escaped; the annotation
  lint skips keywords only in keyword position, so a field named `default` is still checked;
  `lab.result`'s `time_unknown` is typed and the test refuses a string.
- Everywhere: no SQLite named outside `internal/store`; no ledger trigger touched; the Envelope
  stays the only ingress; every external call carries a deadline; errors wrap with `%w`; tests use
  real files under `t.TempDir()`.

## Hickey

The day's direction is right by this lens: two in-process source readers left the binary, a
Definition found its one owner, knowledge position (`as_of`, `w`) was pulled apart from fact-time
selection, and absence is derived rather than stored. What got braided back is small and
specific. A snapshot is a claim about a source over an interval, keyed to a single instant
(finding 1). A query that narrows a read lives twice, as `bounds` and as `receipt.Query` (finding
3). A (type, version) identity that two sources may fill is resolved by precedence rather than by
naming which is meant (finding 6). v4 stores one time twice, `date_excel` and
`effective_time_frame`, each derived by a different program.

## Armstrong

Nothing in the window can corrupt the error kernel; every new writer of the ledger goes through
the gate, and the operational tables say which tier they are. The two real runtime findings are
the same shape: a component moved out from under its supervisor. Publishing left the write queue
and inherited a store write that was only safe because it was serialised (finding 2). Absence
judgment is correct under crash but not under concurrency between two of the same source's runs,
and the consequence lands in the one irreversible path (finding 1). Elsewhere the weakness is
visibility, not containment: `payloadRows` limps on silently (finding 5), and a reader's gap
renders as a server fault (finding 12).

## Where the lenses converge and pull apart

Both lenses land on findings 1 and 2. A point where an interval was meant, and a derivation
performed twice, are structural faults to Hickey and become races or wrong tombstones to Armstrong
once two of the same thing run unsupervised.

They pull apart on finding 3 and on the snapshot's home. Armstrong is content that verify replays
`Narrow` and a test pins the equality; Hickey says a pinned coincidence is still a coincidence.
Hickey would put the snapshot claim in the ledger as a fact the world asserted; Armstrong keeps it
in the disposable tier because losing it costs nothing and keeps the kernel small. Matt chose the
latter for card 78; §8 should record it.

## Outside the window

`anam-project.html` sits untracked at the repo root and still describes five worker packages
including Withings and bp-csv.
