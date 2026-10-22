# Components

* [Admission gate](gate.md) - The only ingress: resolve binds an envelope to the person, commit validates it against its Definition, composes identity, and appends through the ledger idempotently.
* [anam_bp_csv](importer-bp-csv.md) - Shapes a blood-pressure app export to health.blood-pressure v7, an Open mHealth body beside the export's own text, one reading sent once from the newest export.
* [anam_documents](importer-documents.md) - Sends a folder of documents with .meta.yaml sidecars as one snapshot of record.document envelopes, each batch naming the files whose bytes anam_submit hands Anam's library before the envelopes go.
* [anam_dreamstation](importer-dreamstation.md) - Decodes a CPAP SD card copy with the dreamstation package and sends every session as health.cpap-session.dreamstation, a type the binary does not bundle, so it registers its own Definition.
* [anam_records](importer-records.md) - Sends a records folder as one snapshot of the bundled types it carries, reading YAML with Go's yaml.v3 rules so every record Anam's own records worker admitted before card 119 resends as no-op.
* [anam_submit](anam-submit.md) - The one generic plugin that posts batches: makes sure Anam publishes the batch's Definitions, submits under the importer's token, and raises the absent alert on a completed snapshot.
* [Backup (internal/backup)](backup.md) - POST /v1/backups copies both SQLite files with VACUUM INTO and the artifact and library directories into a dated folder, keeping backup_keep of them.
* [Buckets (internal/span)](span.md) - Bucket arithmetic over fact-time: the configurable width, self-describing ids, and the layout that subdivides a year into months and days.
* [Catch-up (internal/catchup)](catchup.md) - Replays anam-events since the index's last seen seq into the operational store, so the derived tier is behind after a crash, never wrong.
* [CLI (internal/app)](cli.md) - The anam command tree; every command routes to the daemon when ANAM_SERVER is set, or opens the local files itself.
* [Client (internal/client)](client.md) - The Go HTTP client the CLI uses against the daemon: bearer token, a timeout header, a network margin, and a bound on the answer it reads.
* [Config (internal/config)](config.md) - Viper over YAML and ANAM_* env vars; validates every path and limit, refuses an unknown key or bucket width at load, and reads the importers file as principals.
* [Daemon](daemon.md) - The one process that owns the ledger on unraid; HTTP routes behind role-checked tokens, a single write queue, catch-up and mint after each write.
* [Definitions registry (internal/def)](def.md) - Parses Definition manifests, holds the bundled set from embedded JSON, merges registered ones, and guards a new version against re-keying.
* [Facet domain values (internal/facet)](facet.md) - The value types everything else speaks in: Facet, AnamEvent, Envelope, the five event kinds, and SourceRef, the one function that composes identity.
* [Fold](fold.md) - Derives each facet's current status from its anam-events in seq order and builds the canonical value, sorted and hashed.
* [Importer base (anam_importer)](importer-base.md) - The shared Python package every importer starts from: the ductile protocol, Definitions and local validation, envelopes and batches, and the HTTP client.
* [Inspector (internal/inspect)](inspect.md) - The web harness over the full fold, the Anam Workbench: timeline, facets with the edges touching them, events, projections, the selection builder and definitions, the default library's bytes confined to its root, and forms that enter, correct and retract a fact by hand.
* [Ledger store (internal/store/ledger)](store-ledger.md) - The append-only SQLite file: facet artifacts, anam-events and ledger_meta, with triggers that refuse update and delete for every writer.
* [Library (internal/library)](library.md) - Anam's store of document bytes: files named <sha256>.<ext>, written only by Anam, placed by a folder limit and found again by a walk for the name.
* [Mint](mint.md) - Writes bucket checkpoints after each change, starting from the last run and touching only the buckets the tail dirtied; also the composed reads that start from those checkpoints.
* [Operational store (internal/store/ops)](store-ops.md) - The tracking.db SQLite file: Definitions, the facet and event indexes, the checkpoint log, mint runs, snapshot parts, and the pins that refuse a mismatched ledger or width.
* [Policy (internal/policy)](policy.md) - Pure functions over the request and the importers file's principals that answer every who-may question the daemon, the inspector and the CLI ask; it reads no payload.
* [Principals and roles (internal/daemon auth)](principals.md) - Every caller is a principal with a name, a role (reader, importer or operator) and a token hash; admit asks the policy whether the route admits its role, with 401 for an unknown token and 403 for a role the route does not admit.
* [Projection](projection.md) - Applies a template, bundled or registered, to a canonical value, metadata filters only, and explains each facet's verdict by the knob that decided it.
* [Read (internal/read)](read.md) - The read API behind timeline, changes, publish and the snapshot report: parses the query, picks composed or full fold, narrows, and bounds the answer.
* [Receipt (internal/receipt)](receipt.md) - The receipt record and its proofs: validate, deterministic bytes, and compare a re-derived value with what the file holds.
* [Selection (internal/selection)](selection.md) - The shared filter: fact-time from and to, type or prefix, source, standing, and facets ticked or unticked by hand, as one Go value that compiles to a Datalog pick/1 program and parses back only when the program is one it writes.
* [Serve (internal/serve)](serve.md) - Publishes a projection: canonical at head, prune, write the served value then its receipt, and verifies a receipt by re-deriving.
