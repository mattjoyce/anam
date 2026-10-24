# AGENTS.md — Anam engineering contract

This file governs how code is written in Anam. It is binding for humans and coding agents alike.
Two things sit above the mechanics: the **design lens** we review through, and the **doctrine** the
code serves (`docs/ARCHITECTURE.md`).

---

## Guiding design lens — Hickey × Armstrong

Every design review and code audit runs through **Hickey × Armstrong** (*structure × runtime*).
Hold the tension; do not average it.

- **Hickey (structure).** What is *complected* here — which two concerns are braided that could stand
  apart? Is this *simple* (un-braided, objective) or merely *easy* (familiar)? Where does state
  actually live? Are we producing new **values** or overwriting a **place**?
  - Anam is **Datomic-shaped**: append-only accreting **immutable facts**, epochal time
    (fact-time / anam-time), **current state derived as a fold**. Never overwrite a fact. Never
    store a fact's current status as mutable place — derive it.
  - The **substrate is accidental**, not essential — the problem is a personal longitudinal record, not a
    store. Only `internal/store` may name SQLite or `database/sql`; `scripts/check.sh` enforces
    it. Anam reads no source itself: every source reaches it through an importer, over the envelope
    API (card 119). The ledger is the record, in a SQLite file of its own; files
    hold published artifacts; a second SQLite file is the **operational store**, authoritative for what
    Anam authors. Each of its tables says where it comes back from (§8) — the *derived* tier by replaying
    the ledger, and that tier must stay provably derived.
- **Armstrong (runtime).** For each component, name its failure and its recovery. What is the **error
  kernel** — the small trusted core that must not corrupt? What crashes, and what restarts it clean?
  - Anam's **error kernel** is the *append-only ledger + admission-gate invariants* (three pillars,
    idempotency, immutability). Everything else — importers, fold, projection, CLI — is
    **disposable and rebuildable from source** (Anam owns no facts).
  - `commit` is **idempotent**; the artifact and its anam-event are one SQLite transaction, and the
    watermark `W` is the newest event's `seq`. **There is no transaction across the two files** — the
    operational store catches up by replay, so a crash leaves it *behind*, never *wrong*. A worker may
    crash mid-batch and simply be re-run to known-good. No partial facet, ever.
  - **Append-only is enforced by the ledger's triggers**, for every writer. Commands that only read the
    ledger open it read-only. Never add a code path that drops or disables a ledger trigger.
  - The **`Envelope` is the only ingress** (share-nothing). Every external call carries a `context`
    deadline and **fails fast** — never a hang.

Run it explicitly on any non-trivial change: before a design is built, and after a diff of any size.

---

## Go standards

Use these defaults unless a task clearly requires otherwise.

**Defaults**
- Config: `viper` + YAML (or `envconfig`). CLI: `cobra`. Logging: `log/slog`.
- Errors: `errors` + `fmt.Errorf(... %w ...)`. DB: `database/sql`. Modules: `go mod`.
- Testing: stdlib `testing`, table-driven.

**Dependencies.** Keep them few. The stdlib comes first, and a few lines of our own code beat a
module. Adding a direct dependency to `go.mod` is the maintainer's call: say what the stdlib lacks and what
the module costs (its own transitive deps, its maintenance), and ask before adding it. Never add one
to save a page of code.

**Structure**
- Binaries in `cmd/<name>/main.go`; keep `main` thin — call `app.Run(ctx) error`.
- Group packages by concern, not layers. Non-public code in `internal/`.

**Errors**
- Wrap propagated errors with context. Use `errors.Is` / `errors.As`.
- No `panic` except unrecoverable startup. No `log.Fatal` outside `main`.
- Don't log-and-return the same error. Don't ignore an error without a comment.

**Interfaces**
- Define interfaces **where they are used**; keep them small.
- **Accept interfaces, return concrete types.** Constructors return concrete types.
  → each consumer (ingest, canonical, mint) declares the *narrow* store interface it needs; all are
  satisfied by the concrete stores. No god `Store`.

**Style**
- `gofmt` + `goimports` mandatory. `golangci-lint` must pass.
- Refactor if a function exceeds ~60 lines or a file exceeds ~400.
- Exported names need doc comments. Avoid stutter and vague names.

**Concurrency**
- `context.Context` is the first arg for blocking/cancellable work; cancel on shutdown.
- One synchronisation mechanism per datum. `defer mu.Unlock()` immediately after `mu.Lock()`.
- Race-test failures block merge.

**Tests**
- **Prefer real components over mocks.** Mock only truly uncontrollable externals.
- Use a **real ledger file** and a **real operational store file**, each under the test's own
  `t.TempDir()` — never shared: parallel packages and `-shuffle=on` make shared state flaky. No test
  needs a network, so no test skips. Test behaviour, not internals. Tests beside code. No flaky
  tests. Table-driven for multiple meaningful cases. Target >80% coverage.

**Embedded assets**
- `//go:embed` (`embed.FS`) for small, static, versioned assets only (projection templates, bundled
  facet definitions). Never embed large, mutable, or environment-specific content.

**Security**
- `gosec` must pass. `#nosec` requires rule ID + reason + approval. Never log secrets.
- **Validate all external input** — for Anam that is the ingest gate's *facet-compliant* pillar (source
  rows are external input; they are validated before admission).

**Versioning** (ldflags, not hardcoded constants)
- `scripts/version.sh` → `v0.<commit-count>-<short-hash>`.
- `Makefile` injects `-X main.version=… -X main.gitCommit=… -X main.buildDate=…` and builds with `-a`
  (the build cache otherwise reuses stale ldflags). Fallback `version = "dev"` — honest, not fake semver.

**Pre-merge gate** — `scripts/check.sh`:
```sh
gofmt -l .
goimports -l .
scripts/check-links.sh
python3 docs/explainer/check.py   # needs anam-importers beside this checkout, or EXPLAINER_IMPORTERS_DIR
go vet ./...
golangci-lint run ./...
gosec -severity medium ./...
go test -race -count=1 -shuffle=on -vet=all ./...
```

---

## Defaults and invariants (card 151)

Someone running Anam can make their own choices; Anam ships opinionated defaults. So every
hard-coded value is one of two kinds, and a new value is sorted when it is written.

- **A default** is a choice someone could reasonably make differently: a timeout, a wait, a size
  bound, a retention, a retry count. It is a config key (snake_case, with its `ANAM_` variable)
  whose default is a `Default…` constant in `internal/config`, with a comment saying why that
  number. Load range-checks it, so a bad value fails at start. It reaches the code as a parameter
  from the one `Config` loaded at start, never read from a global, a file or the environment deep
  in the code. It is listed in `config.example.yaml`.
- **An invariant** is a value whose change breaks correctness, data or a contract, not taste: what
  both ends of a wire must agree on (`gate.MaxBatch`, the `Anam-Timeout` header, the snapshot id's
  shape), what the stored files mean (schema versions, `clock.Width`), a security floor (the
  token's length, file modes), or arithmetic (days in a week). It stays a constant, with a comment
  saying why it is not configurable. A value that is part of what the data means belongs in the
  Definition or the type, not as a loose constant.

Two kinds of constant are neither. Presentation (the inspector's geometry and page sizes, a
terminal line's clip) is changed by changing the page. A store's internal tuning that changes no
result and no limit a person meets (`versionsBatch`, `latestScanAt`, a busy timeout bounded by
`timeout`) stays in the store with a comment saying so.

No test hooks. A value a test needs to change is a default, and the test sets it the way a person
would. A value a test must not change is an invariant. No package variable is set from a test.

---

## Python

Anam holds no Python since card 119: the one-off extraction and reshaping scripts, which turn a
source export into a records file, live in `anam-importers` under `records/extract` (card 58). A
script that comes back here never touches the ledger or the operational store, Anam itself stays
Go, and these rules hold:

- `#!/usr/bin/env python3`, stdlib only, `argparse` for flags. PyYAML is the one third-party module
  in use; a second one follows the dependency rule above.
- No virtualenv or install step: a script runs as `python3 scripts/<name>.py` on a stock machine.
- Read external input as untrusted: run with `-I` when the script reads downloaded files, and pass
  paths as arguments.
- The same naming applies: hyphenated file names, snake_case keys in what it writes.

---

## Naming (Ductile Naming Standard, applied to Anam)

| Surface | Shape | Example |
|---|---|---|
| **Facet type** | lower-case **dotted**, `domain.concept[.specialisation…]` | `health.blood-pressure`, `mind.attention` |
| Source / operator identity | lower-case **hyphenated** | `garmin`, `bp-csv` |
| CLI command | lower-case hyphenated | `ingest`, `def load` |
| Config / payload / data key | **snake_case** | `db_path`, `ledger_path`, `period_start` |
| SQL column | snake_case | `fact_time`, `content_hash` |
| Env var | UPPER_SNAKE | `ANAM_LEDGER_PATH` |

Facet types are **dotted `domain.concept[.specialisation…]`** on purpose. The domain is a closed set
of ten (`health`, `activity`, `place`, `record`, `mind`, `org` and `money`, with `social`, `media` and
`work` reserved), so growth is in the concept, in a specialisation after it, and in `family`, not in new
heads (`health.blood-pressure`, `social.message`, `media.play`). A head is added only by a logged
decision, as `mind` was for self-reported inner states, `org` for relationships with
organisations and `money` for what the person paid. Which of those a distinction
belongs in is written down in `docs/explainer/bundle/decisions/where-a-distinction-is-encoded.md`.
Payload keys stay snake_case; the dotted facet type is deliberate and lives here so it doesn't drift.

---

## Commit discipline

- Commit regularly, in logical chunks; keep the tree buildable.
- Conventional-ish subjects (`feat:`, `docs:`, `chore:`, `test:`, `fix:`).
- Run `scripts/check.sh` green before a commit that touches Go code.

---

---

## Cards

Work is tracked on the maintainer's kanban board, outside this repository. A "card N" in these
documents, in `CHANGELOG.md` and in commit subjects is an item on that board, where the reasoning
behind a change is kept. A card has a job story ("When [situation], I want to [motivation], so I
can [outcome]"), its decisions and what was found while building it. The maintainer's own agents
are given a separate operating file for the board, the worktrees and how sessions coordinate; a
contributor who is not one of them needs none of it. Read a card against the code before building
it, where you have one: a later direction can overtake it.
