# AGENTS.md — Anam engineering contract

This file governs how code is written in Anam. It is binding for humans and coding agents alike.
Two things sit above the mechanics: the **design lens** we review through, and the **doctrine** the
code serves (`docs/ARCHITECTURE.md`).

---

## Guiding design lens — Hickey × Armstrong

Every design review and code audit runs through **Hickey × Armstrong** (*structure × runtime*) — the
`luminaries` skill's default pair. Hold the tension; do not average it.

- **Hickey (structure).** What is *complected* here — which two concerns are braided that could stand
  apart? Is this *simple* (un-braided, objective) or merely *easy* (familiar)? Where does state
  actually live? Are we producing new **values** or overwriting a **place**?
  - Anam is **Datomic-shaped**: append-only accreting **immutable facts**, epochal time
    (event-time / record-time), **current state derived as a fold**. Never overwrite a fact. Never
    store a fact's current status as mutable place — derive it.
  - The **substrate is accidental**, not essential — the problem is a personal longitudinal record, not a
    store. Only `internal/store` may name DynamoDB, SQLite or `database/sql`; `scripts/check.sh` enforces
    it. The ledger is the record; files hold published artifacts; SQLite is the **operational store**,
    authoritative for what Anam authors. Each of its tables says where it comes back from (§8) — the
    *derived* tier by replaying the ledger, and that tier must stay provably derived.
- **Armstrong (runtime).** For each component, name its failure and its recovery. What is the **error
  kernel** — the small trusted core that must not corrupt? What crashes, and what restarts it clean?
  - Anam's **error kernel** is the *append-only ledger + admission-gate invariants* (three pillars,
    idempotency, immutability). Everything else — capture workers, fold, projection, CLI — is
    **disposable and rebuildable from source** (Anam owns no facts).
  - `commit` is **idempotent**; artifact, record-event and the watermark `W` are one atomic
    `TransactWriteItems`, conditional on the idempotency marker. **There is no transaction across
    substrates** — SQLite catches up by replay, so a crash leaves it *behind*, never *wrong*. A worker may
    crash mid-batch and simply be re-run to known-good. No partial facet, ever.
  - **Append-only is not enforced by DynamoDB.** Our code must never issue an `UpdateItem` against a facet
    or a record-event; a test guards it. This is a known cost of the substrate split.
  - The **`Envelope` is the only ingress** (share-nothing). Every external call carries a `context`
    deadline and **fails fast** — never a hang.

Run it explicitly on any non-trivial change: `luminaries` skill → *Before* (design) or *After* (audit).

---

## Go standards (from Matt's vault — adopted verbatim)

Use these defaults unless a task clearly requires otherwise.

**Defaults**
- Config: `viper` + YAML (or `envconfig`). CLI: `cobra`. Logging: `log/slog`.
- Errors: `errors` + `fmt.Errorf(... %w ...)`. DB: `database/sql`. Modules: `go mod`.
- Testing: stdlib `testing`, table-driven.

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
- Use **real DynamoDB Local** (checked in `TestMain`; skip with a clear message if unreachable) and a
  **real SQLite** file. A **unique table and a temp file per test run** — never shared: parallel packages
  and `-shuffle=on` make shared state flaky. Test behaviour, not internals. Tests beside code. No flaky
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
go vet ./...
golangci-lint run ./...
gosec -severity medium ./...
go test -race -count=1 -shuffle=on -vet=all ./...
```

---

## Naming (Ductile Naming Standard, applied to Anam)

| Surface | Shape | Example |
|---|---|---|
| **Facet type** | lower-case **dotted**, namespaced | `health.blood-pressure`, `calendar.event` |
| Source / operator identity | lower-case **hyphenated** | `health-db`, `bp-csv` |
| CLI command | lower-case hyphenated | `ingest`, `def load` |
| Config / payload / data key | **snake_case** | `db_path`, `ddb_endpoint`, `period_start` |
| DynamoDB attribute / SQL column | snake_case | `event_time`, `content_hash` |
| Env var | UPPER_SNAKE | `ANAM_DDB_ENDPOINT` |
| DynamoDB key value | lower-case, `#`-delimited | `person#matt`, `facet#…` |

Facet types are **dotted and namespaced** on purpose — the namespace earns its keep as sources
multiply (`health.*`, `calendar.*`, `mail.*`, `note.*`). This deviates from Arch's snake_case facet
names; the deviation is deliberate and lives here so it doesn't drift.

---

## Commit discipline

- Commit regularly, in logical chunks; keep the tree buildable.
- Conventional-ish subjects (`feat:`, `docs:`, `chore:`, `test:`, `fix:`).
- Run `scripts/check.sh` green before a commit that touches Go code.
