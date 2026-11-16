# Startup: from process start to the first facet

This is the engine's startup order and the path from its listener to an MCP client.
**Ready means the ledger head is readable; it does not mean the index has caught up or
that the startup mint has finished.** A restart need not mint anything.

Verified against engine commit `0cb017b849fbc496f4e0543ea6722f74d44d09be`
(`origin/master`, 2026-11-16), and the supplied live startup log of
`anam:v0.826-f95e80e`, restarted at **2026-11-16T10:24:25Z** after a config-only
change. That log reports the config from `env:ANAM_CONFIG`, catalog admission,
ledger/subject stdout and a serving line timestamped **10:24:26 UTC**, about one
second after the container start timestamp above, with no catch-up or mint line
in the supplied excerpt.
The live image predates master's `/feed/` route (card 276).

Ownership in the timeline is explicit:

- **Engine** means `anam` in this repository, including its image's defaults.
- **Deployment config** means the operator's mounted files and process/container
  configuration (`anam_mattjoyce` and `unraid_admin` in the reference deployment).
- **MCP adapter** means `anam-mcp`, a separate process and repository.

For host paths, binds, compose, restarts, post-flight checks, Cloudflare and rollback,
consult `unraid_admin/vault/Anam Deploy Runbook.md`,
`unraid_admin/vault/Anam Service Container.md`, and
`unraid_admin/vault/Anam MCP Container.md`. Those pages own deployment details;
this guide describes what the engine does with what it is given.
See also [configuration](../config.example.yaml), [API](API.md) and
[architecture](ARCHITECTURE.md).

## Timeline

Steps 3–11 run **in sequence inside `daemon.Open`**, before a listener exists.
An error there returns to the CLI, which writes `anam: <error>` to stderr and
exits with status **1** (`cmd/anam/main.go`); no HTTP health response is possible.
A deployment's restart policy can turn that refusal
into a crash loop. Successful steps with no log are marked silent. Log IDs refer
to the exact source strings in the next section. Unless stated under timings,
there is no verified duration for an individual step.

Three subject refusals, with example runtime values, are:

> invalid config: subject_id is required (ANAM_SUBJECT_ID)

> invalid config: subject_id ("a") and its old name person_id ("b") differ; set subject_id only

> ledger: file belongs to a different subject: ledger is pinned to "stored", config says "configured"

R0–R4 below check their source prefixes and format strings. PR #56 changes the
ledger's stored pin key; the refusal text above is the same at `origin/master`.

| Step | Owner | Reads / action, in order | Log/output and refusal or degradation |
|---|---|---|---|
| 1. Start the process | Engine image; deployment config supplies mounts, environment and overrides | The image runs `/usr/local/bin/anam daemon` on distroless as nonroot (65532). A bare process enters the same CLI. Deployment must supply readable config/catalog/token files and usable store paths; it owns restart policy and permissions. | Silent until config loads. A container runtime can refuse before Anam runs (bad executable or mounts); consult the runtime's error. There is no engine log in that case. |
| 2. Resolve and load config | Engine; values are deployment config | `--config`, then `ANAM_CONFIG`, then the platform user config directory's `anam/config.yaml`. An explicitly named missing file fails; an absent discovered file uses defaults. Load the include graph, defaults, YAML, nonempty scalar `ANAM_<KEY>` overrides, then explicit overrides. Validate strict keys/types, subject, limits, paths, zone, bucket width and inspector presentation. | C1 or C2 after successful load. Missing/invalid files, unknown keys, invalid limits, missing `subject_id`, unequal subject aliases or unequal timeout aliases exit before stores open. A configured `server` or `ANAM_SERVER` also refuses daemon startup: local files belong to the configured daemon (`internal/app/app.go`, `loadConfig`). C3–C5 cover compatibility diagnostics. |
| 3. Validate presentation and limits again | Engine | `cfg.CompilePresentation()` then `cfg.Limits.Validate()`, so programmatic callers of `Open` receive the same checks. | Silent; invalid presentation/limits refuse. |
| 4. Authenticate the listener configuration | Engine; token/listen are deployment config | Read `token_file` (`ANAM_TOKEN_FILE`), if configured. Otherwise require a loopback `listen` (`ANAM_LISTEN`). Then compile `bucket_width` into the bucket calendar. | Silent; missing, unreadable or invalid token, non-loopback without a token, or invalid bucket width refuses. An explicitly configured missing token never falls back to tokenless mode. |
| 5. Load one catalog snapshot | Engine; catalog files are deployment config | `catalog.Load(definitions_dir, projections_dir)` reads both configured directories once. The same immutable catalog feeds admission, registries and receipts. Neither directory set means an empty catalog; the binary carries no embedded catalog. | Silent here. A configured missing/unreadable directory, malformed Definition/projection, invalid structural Definition (`record.edge`, `record.document`) or inconsistent catalog refuses before stores open. |
| 6. Load principals | Engine; `importers_file` is deployment config | Read `importers_file` (`ANAM_IMPORTERS_FILE`), if set; validate roles, token hashes, ownership and importer types against the provisioned catalog. Add the operator from step 4; keep the tokenless workbench principal apart. Reader principals live in this file too. | Silent; unreadable/bad YAML, invalid/duplicate principal identity or token, invalid type ownership, or a principal reusing the operator token refuses. Provisioned types may be shared by importers; a type absent from the catalog must have a single owner. Losing the catalog can therefore make a real roster with shared types fail ownership validation. A configured missing catalog directory already refuses at step 5; unsetting the directories can refuse here rather than start empty with a warning. |
| 7. Open the stores | Engine; paths/subject are deployment config | In order: `ledger.Open(ledger_path, subject_id)`, `ledger.OpenReadOnly` on the same ledger, then `ops.Open(db_path, buckets)`. Read ledger metadata (subject and schema) and operational metadata (schema and pinned bucket width). This phase has `timeout`. | Silent; missing/uninitialised ledger, wrong subject/schema, unreadable or unwritable file, or wrong operational schema/bucket width refuses. Daemon startup does not initialise a missing ledger; use the separate init workflow. Opening stores can create operational state or ensure indexes: the whole `Open` is not a read-only transaction. |
| 8. Check operational index identity | Engine | `checkIndex` pins/checks the operational ledger identity, compares `replay_cursor` with the ledger head (`anam_event.seq`). | Silent; a different ledger identity or a cursor ahead of the ledger refuses. A cursor behind is accepted; replay comes later. |
| 9. Guard and build registries | Engine | With a fresh `timeout`: `guardCatalog` reads operational `definition`; `loadDefinitions` reads `registered_definition`; `loadProjections` reads `registered_projection` and validates `default_projection`. This order precedes catalog persistence. | G1 warns when a registered Definition is shadowed by a provisioned one. A contradictory Definition (including a same-version body conflict), incompatible registered body/template, or absent default projection refuses before catalog persistence. |
| 10. Persist and report catalog | Engine | `persistCatalog` upserts admitted current and historical manifests into `definition` (no sweep). Projection files stay the current provisioned templates; a receipt files the body it used. Then `logCatalogGaps` inspects configured components and absent structural types. | G2, then any G3–G5. Neither catalog configured warns but can start if earlier validation passed. One unset component warns; absent `record.edge`/`record.document` logs INFO with its effect. Links cannot be entered/indexed without the edge Definition; documents cannot be referenced/served without the document Definition. Persistence errors refuse. |
| 11. Finish `Open` | Engine | Open a read-only operational handle, construct the daemon, registries, selections, write queue and mint request channel. | Silent; a read-only operational open failure still refuses, even if G2 was already emitted. Catalog admission alone does not prove serving. |
| 12. Print identity; bind listener | Engine | CLI prints ledger path, `subject_id` and configured URL, then `Run` calls `net.Listen` on `listen`. | S1 stdout happens **before** binding. A busy port/invalid address returns an error and closes stores; the printed URL alone does not prove a listener. |
| 13. Start concurrent serving and workers | Engine | `Serve` builds HTTP routes (inspector and `/feed/` included), starts the single write worker, mint worker, startup catch-up goroutine and HTTP server goroutine on the opened listener. | S2 uses the actual bound address (it can differ in spelling from the configured URL). Listener/server failure ends the run; shutdown closes stores. The following work races with requests and with this log. |
| 14. Replay as the startup write | Engine | The startup goroutine submits catch-up to the write queue under `long_write_timeout`. Replay ledger events into `facet_index`, `anam_event_index`, `facet_edge` and `replay_cursor`. Writes serialize; reads use separate read-only handles. | I1 only if replay advanced. I2 leaves the index behind and the process serving. I3 marks a wrong index; index-dependent routes return HTTP 500 until restart on a sound index. No replay progress means no success log. |
| 15. Request and run startup mint if needed | Engine | After the catch-up write returns, the same goroutine runs `mintIfBehind`: compare ledger head with `mint_run`. A nonempty head with no run or an older run requests a mint. The mint worker coalesces requests, waits `mint_quiet`, then mints through the same write queue with `long_write_timeout`. It reads ledger/index/checkpoints and records checkpoint values and a completed run. | M1 on completion, M2 on failure; failure does not undo admitted facets or stop HTTP. Empty ledger/current mint needs no startup mint. Head/run read failures in `mintIfBehind` are silent and do not schedule one. A cut-short mint can leave completed bucket checkpoints without a completed run; the next mint starts from the last recorded run. |
| 16. Probe readiness (concurrent with 14–15) | Engine; probe schedule/post-flight are deployment config | `GET /healthz` reads only the ledger head under `timeout`, without a token. Image health check runs `anam daemon ready`, which reloads config and calls this route at `listen` (wildcard addresses are probed on loopback). | H1 body with HTTP 200, or H2 body with HTTP 503 and H3 warning. The ready command exits 0 on HTTP 200; it does **not** print a readiness success line. It can fail on config loading, connection, deadline or non-200 response. Docker and deploy post-flight can therefore call the daemon healthy/ready during catch-up and mint. |
| 17. Start the adapter (independent of engine startup) | MCP adapter; environment and token are deployment config | Adapter reads `ANAM_SERVER`, `ANAM_TOKEN_FILE`, required `ANAM_ZONE`, call timeout/margin/response limit, and optional HTTP/Access settings. It reads its token once and builds the daemon client and MCP server. Optional note token is a separate writer identity. | Adapter diagnostics belong to `anam-mcp`; no engine startup line. Invalid environment or missing/empty/unreadable configured token exits the adapter. Construction does not probe the daemon, so MCP can listen while Anam is unavailable. A bad daemon token may surface only on the first call. |
| 18. Connect the MCP client | MCP adapter; transport is deployment config | Adapter serves stdio, or Streamable HTTP at `/mcp` with Access configuration. HTTP validates the Access JWT before MCP processing; the client initialises an MCP session and calls a tool. The daemon sees the adapter's own bearer token, not the client's JWT. | No engine log just for connecting MCP. Missing/invalid JWT returns adapter HTTP 401; a busy adapter listen port fails adapter start. The MCP container page owns the tunnel, OAuth and compose setup. |
| 19. Describe the record | MCP adapter calls engine; reader identity is deployment config | `describe_record` sequentially calls `GET /v1/status`, `/v1/definitions`, `/v1/projections`, then `GET /v1/coverage?w=<status watermark>`. The adapter's token hashes to a `principals` entry with role `reader` in `importers.yaml` (the configured `importers_file`). At anam-mcp commit `06ac939`, the description reports `subject`, types, projections, coverage and the advised default projection. The live adapter `v0.67-060b77b` still answers `person`. | No success log per call. Engine returns 401 for missing/unknown token, 403 for wrong role. First three call failures fail the tool; coverage failure is reported in the description instead. An unreachable engine/deadline becomes an MCP tool error, not an empty record. |
| 20. Return the first facet | MCP adapter calls engine | `read_record` without a projection calls `GET /v1/timeline`; with a named projection it calls `POST /v1/receipts`, which publishes the read and returns its first page and receipt. A principal with `receipts: always` must name a projection; an unprojected request is refused with HTTP 400. Reads use the ledger and read-only operational checkpoints/values; publishing uses the operational write handle outside the write queue and stores `receipt`/`served_value` rows, without adding ledger facts. | No success log per facet. HTTP 200 timeline or HTTP 201 published response carries entries; the adapter returns their payloads and artifact versions as MCP evidence (or a projected frame). That response is where the first facet leaves the daemon; the subsequent MCP tool result is where it reaches the client. Empty/filter-excluded records yield no facet. Bad query/projection is HTTP 400; oversized unreceipted read is 413; busy publication is 503 with `Retry-After`; deadline is 504. |

## Configuration files that startup actually reads

`include:` is a YAML path or list, resolved from each including file inside the
resolved root config directory. A leading slash means that directory's root,
not the host's root. Root YAML loads first, then includes depth-first in declaration
order; later values win, mappings merge and arrays replace. Missing targets,
cycles/repeated targets, directories, incompatible container kinds and escapes via
`..` or symlinks refuse config loading. Application paths inside every fragment
still expand from the process working directory, not from the fragment's directory.

A deployment can ship `inspector.yaml` beside `config.yaml` and load it with
`include: [inspector.yaml]`. Merely placing it there does not load it: the include
must name it, and a file bind of only `config.yaml` does not expose its sibling.
The `inspector:` section controls presentation, including domains, families, types
and optional track type. Omission gives neutral presentation. Invalid presentation
refuses before stores/listeners. Presentation validation is independent of the
catalog; a presentation entry does not admit its type into the catalog. `include` and `inspector` are YAML-only, with no
`ANAM_INCLUDE` or `ANAM_INSPECTOR` structured override. Config, catalogs, token and
principal roster are loaded at startup; changing their files requires a restart to
change the daemon's loaded snapshot.

## Concurrency and the ready window

Only steps 3–11 are the sequential `Open`. After the listener opens, `Serve` launches
workers and HTTP concurrently. The startup catch-up is submitted as the first
write, then `mintIfBehind` runs after that write returns. The mint worker is already
waiting for requests; it does not mint before it receives one. Scheduling can put
I1 or even a fast startup operation before S2 in a log. Serving does not wait for
either operation. Other writes share the queue and can receive a busy response.

During this window, `/healthz` can return HTTP 200 while the operational index is
behind or even marked wrong. `GET /v1/changes` answers the index as it stands with
`index_seq` and ledger head `w`: `index_seq < w` says recent changes are missing.
Facet links likewise carry the index watermark. These reads do not trigger replay.
A wrong index makes guarded index routes (changes, checkpoints, receipt-facet links)
return HTTP 500; readiness is still a ledger check.

A timeline read is different: it composes from a usable mint run's checkpoints plus
ledger replay, or folds the ledger from the beginning when no run is usable. A
pending mint therefore need not hide facets; it can make the first read more
expensive. A projected/policy-published read files its receipt on the operational write handle
**outside the write queue**. If another writer holds the store beyond its busy
timeout, filing returns HTTP 503 with `Retry-After`; no receipt or answer was served.
Catch-up or mint can cause this store contention, but a busy queue alone does not
refuse publication. The browser guard
runs before authentication: an unsafe cross-site browser request is 403; ordinary
server-to-server calls without browser origin metadata pass that guard. Authentication
and per-route role checks still apply, including the operator-only inspector and
`/feed/`. The feed is a separately served UI, not the MCP read endpoint; at the
verified master it still opens on `fixture.js` data pending its live-read slice.

## Log and output reference

The quoted cells below are **exact Go string literals or format strings**, including
escaped newlines where the CLI prints several lines in one call. Expand `%s` and
other format verbs with runtime values. For structured logs the level and attribute
keys are separate columns; timestamps and values come from the logger/runtime.
An attribute value containing spaces may be quoted by the logger. Silent steps
have no success marker to grep. Each row must contain exactly one backticked Go
string literal; `TestStartupQuotedOutput` binds its ID to a source file, declaration
and message argument of the expected emitter. Both G4 call sites are required.
Slog levels and attribute keys in call order are checked against these columns.
R0/R3 are error prefixes; R1/R2/R4 are the formats composed with them.

| ID | Level / stream | Source literal or format | Attributes, in call order |
|---|---|---|---|
| C1 | stderr, no level | `"config: %s (%s)\n"` | resolved path, origin (for example `env:ANAM_CONFIG`) |
| C2 | stderr, no level | `"config: defaults (no file found)"` | none; printed with a newline |
| C3 | WARN | `"config: person_id is the old name of subject_id; set subject_id (ANAM_SUBJECT_ID) instead"` | `subject` |
| C4 | INFO | `"config: subject_id and its old name person_id are both set and equal; drop person_id"` | `subject` |
| C5 | WARN | `"config: ingest_timeout is the old name of long_write_timeout; set long_write_timeout (ANAM_LONG_WRITE_TIMEOUT) instead"` | `value` |
| G1 | WARN | `"daemon: a registered definition is shadowed by the bundled one"` | `type`, `version` |
| G2 | INFO | `"daemon: catalog admitted"` | `definitions_source`, `projections_source`, `definition_set_version` |
| G3 | WARN | `"daemon: no catalog configured; set definitions_dir and projections_dir, or register definitions over the API"` | none |
| G4 | WARN | `"daemon: catalog component not configured; it is empty"` | `unset`, `effect` |
| G5 | INFO | `"daemon: structural type not in the catalog"` | `type`, `effect` |
| S1 | stdout, no level | `"daemon: ledger %s (subject %s)\nhttp://%s/\n"` | ledger path, subject, configured listen address |
| S2 | INFO | `"daemon: serving"` | `addr`, `ledger`, `subject` |
| I1 | INFO | `"daemon: index caught up"` | `from`, `to` |
| I2 | WARN | `"daemon: index catch-up, left behind"` | `err` |
| I3 | ERROR | `"daemon: operational index is wrong"` | `err` |
| M1 | INFO | `"daemon: minted"` | `w`, `full`, `from`, `minted`, `buckets` |
| M2 | ERROR | `"daemon: mint at head failed"` | `err` |
| H1 | HTTP body, no level | `"ready"` | newline appended |
| H2 | HTTP body, no level | `"not ready"` | newline appended |
| H3 | WARN | `"healthz: ledger head unreadable"` | `err` |
| R0 | error prefix, no level | `"invalid config"` | none; config validation prefix |
| R1 | error format, no level | `"subject_id is required (ANAM_SUBJECT_ID)"` | R0 prefix |
| R2 | error format, no level | `"subject_id (%q) and its old name person_id (%q) differ; set subject_id only"` | R0 prefix, new subject, old alias |
| R3 | error prefix, no level | `"ledger: file belongs to a different subject"` | none; ledger subject mismatch prefix |
| R4 | error format, no level | `"%w: ledger is pinned to %q, config says %q"` | R3 error, stored subject, configured subject |

## Timings and verification limits

The supplied 2026-11-16 live log shows serving about **1 second after restart**:
the serving line's time is **10:24:26 UTC**, against container start
**10:24:25Z**. On the same image, `v0.826-f95e80e`, `/v1/status` reports
`server_since` **2026-11-16T10:24:26.03Z**, consistent with that interval.
It does not establish Docker health timing, catch-up duration, startup-mint duration,
or time to the first MCP facet. There is no general startup-time guarantee.

The runbook's 2026-10-22 schema-9 window records **12 seconds for index catch-up**
and a full mint of 47 buckets **at 25 seconds** on `v0.528-6489310`; these are
historical rebuild observations, not current per-step budgets. The MCP container
page's 2026-11-03 first-deploy history records a restart **healthy in 11 seconds**
on `v0.726-215cfcd`. The R4b rehearsal mint duration could not be verified in those
sources and is omitted. `mint_quiet` defaults to **2 seconds**
(`internal/config/limits.go`); that is a configurable debounce, not mint duration.
The image's health probe interval is **30 seconds**, timeout **10 seconds**, start
period **20 seconds**, and retries **3** (`Dockerfile`); none makes readiness wait
for the workers.

The MCP hop was checked read-only against `anam-mcp`'s local `origin/master`,
`06ac939886b85b9856f1c98c5c0ba12d868186cc`, especially `internal/app/app.go`,
`internal/app/http.go`, `internal/anam/client.go`, and `internal/adapter/{describe,read}.go`.
The runbook/container pages were consulted on 2026-11-16 at `unraid_admin` commit
`4a5641f7e7410ae462ba9ef9b3bdf94012d961cd`. The runbook's older compose paragraph
still describes a defaults-only configuration; the supplied live log establishes
that this restart instead loaded `config.yaml`. Deployment details should be checked
against that deployment's current files.

The [source-literal test](startup_test.go),
`go test ./docs -run '^TestStartupQuotedOutput$'` (also run by `scripts/check.sh`),
requires every reference row to contain exactly one checked literal, bound to its
production emitter under `internal/daemon`, `internal/config`, `internal/app` or
`internal/store/ledger`. It checks slog levels and attribute keys, including their
order. Test files, comments, detached literals and other call sites cannot satisfy
it. Inline code spans and fenced lines containing a daemon/config/healthz diagnostic
must be checked literals too; rendered log examples with levels or runtime values
are rejected. **Residual risk:** ordering, branching, attribute values and timing
changes can preserve all checked messages, levels and keys; this check cannot
prove those. Re-read the call sites and refresh the verified commit
when startup changes. The supplied log checks a real restart, not a new MCP
end-to-end rehearsal or a rebuild.
