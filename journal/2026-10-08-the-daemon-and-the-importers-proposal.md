# 8 October: the daemon, and the proposal that moved the sources out

The day started with an agreement from the night before written down. Anam would run as a service in
Docker on unraid, and the CLI, the inspector, an MCP adapter and the scheduled ingest would be its
clients, not openers of its files. The morning fixed a Withings attribution question and added
flights. By evening `anam daemon` existed, ran in a 29 MB image behind a token, and answered the CLI
over HTTP. Late on, a proposal moved every vendor source out of Anam into importers, and one card
built that afternoon was reverted because of it.

## Who stood on the scale

Card 38 began with a bug that had been doing a job. The ductile withings plugin sent `meastype` with
a comma list of twelve types. Withings ignores a list in that parameter and returns every type the
account holds, which is the only reason height and muscle mass, missing from the list, ever arrived.
Switching to `meastypes` with the same list would have quietly lost both. The fix sends no filter.

The real question was attribution. Withings tags each measure group with `attrib`. 0 is a device
reading known to be Matt's, 1 is one the scale could not place, 2 was entered by hand. The tempting
rule was that anything not from a device binds lower. It would have refused every blood pressure
reading Matt has, all 260 groups of them manual, because `health.measurement` binds high. Manual is
how a reading was taken, not whose it is. So only `attrib` 1 lowers the binding to medium, and the
payload carries `attrib` so a reader can still filter hand entries. `health.measurement` v2 names
the new fields and lowers the floor; identity and fact time are unchanged, so the Guard passed.

Retract then found nothing to drop while Withings no longer held three groups: two 56.5 kg weigh-ins
by someone else and a typo with diastolic 17. A mirror built by upserting since a timestamp never
sees a deletion. The fix was a full reconcile upstream in `withings.db`, guarded the same way
`anam retract` is: refuse an empty fetch, refuse a removal over a tenth of what is stored. Rehearsed
on copies of the live ledger, it superseded 5,506, admitted 14, bound 3 weights medium (87.6, 88.2
and 92.9 kg), and tombstoned the 23 facets of the three vanished groups.

Shipping the plugin taught a deployment lesson. It is baked into the ductile image, and a rebuild
would also have rebuilt ductile core from a share three months past the running image. It went in
by copying two files into the running container, re-attesting the lock, and fast-forwarding the
share's clone so a recreate would not revert it.

## Flights

Card 35 added `travel.flight`, one fact per leg, dated in the departure airport's offset. One zone
per type would put a London departure up to a day from where it happened. Flights carry `/trip` but
stand outside the `travel.trip` series. The rehearsal showed why. SQ211, dated in Singapore, sorted
after a return dated the same day in Sydney and held the 2018 trip open for good. A trip's state is
departed, arrived and returned. A flight is evidence of a movement beside it.

## The daemon

Cards 42 to 45 landed between 21:00 and midnight. Card 42 is one process that opens the ledger and
the operational store once and keeps them. Writes go through one queue with one worker, on the
daemon's own context, so a caller that gives up never cuts a write short. Reads use a read-only
ledger handle. The listener stays on loopback until requests carry a token, because the inspector
serves the whole record and the library.

Card 43 added the token, at least 32 characters, compared in constant time, accepted as a bearer
token or a basic-auth password so a browser can open the inspector. `POST /v1/ingest` allows one run
in progress and one pending. A request during a run joins the pending one, since the running one has
already read its sources and would miss what the request was sent about. `?wait=` was added for the
unraid gateway, because a scheduler judges a job by how it ends. Card 44 is the image: distroless,
non-root, static because modernc SQLite needs no cgo, and a `.dockerignore` that admits only
`go.mod`, `go.sum`, `cmd/` and `internal/`, so `private/` never enters a build context.

Card 45 made the CLI a client. With `ANAM_SERVER` set, status, version, timeline, changes, receipts,
checkpoints and ingest ask the daemon and open no local file. Commands not yet routed refuse rather
than open files the daemon owns. The wire values moved to `internal/api` so a client decodes them
without importing the daemon. A reader's `Local` zone is resolved to its IANA name before it leaves
the machine.

## Hickey × Armstrong

**Armstrong.** Two reviews found the failures before production did. Codex's API review found three
runtime faults. Shutdown held a waiting ingest caller for its whole 10s grace and then reported an
error, because the write queue closed only after `http.Server.Shutdown`, which waits for that very
handler. The queue now closes first. The server had only a read-header timeout and now has read,
write and idle ones. The bpcsv, records and documents scanners took no context, so nothing could
stop one stuck on a dead share. They now run under `bounded()`.

The After review of card 45 found more. A daemon now refuses to start on an operational index pinned
to another ledger or ahead of this one. Minting uses `TryWrite`, 503 when another write holds the
queue past 250ms, and once accepted the answer waits for the mint, so a client never sees a timeout
for a write that happened. The client sends its deadline as `Anam-Timeout` and the daemon works to
the shorter. That last rule was the first step toward card 110, which a week later made queue wait
count against that deadline too.

**Hickey.** Package `read` had been minting. `mint.Report` replaced `read.Mint`, so a package named
for reading only reads.

## Sources outside Anam

At 22:42 the importers proposal went into `docs/IMPORTERS.md`. Anam's vendor capture workers would
move out into ductile plugins, each shaping one source to a Facet Definition and submitting
envelopes. It recorded the lineage, Senechal cleaved into ductile and `healthdata.db`, with Anam
succeeding the latter, and what Anam needs first: gate-composed identity, JSON Schema, real
validation, definitions and envelopes endpoints, registration
([[sources-reach-anam-only-through-importers]]). Matt then decided that where an Open
mHealth schema exists, a health payload is the OMH body at a pinned version
([[open-mhealth-is-the-payload-vocabulary-for-health-measures]]). Type names stay Anam's.

Card 56 had been built that afternoon. `exercise.session` is one type for a session of exercise,
conforming to IEEE 1752.1, with 498 Technogym sessions and the Strong workouts as records; all 907
payloads validated. Matt said it should wait. Taken in through records now, the sessions would come
in again from the Technogym and Strong importers under new source refs, two rounds of retract and
admit for the same facts. It was reverted at 23:27, before any ingest ran it, so the Guard never
pinned the type and its rules stay free to change.

Two side threads. `jev-for-anam.md` (since moved to kanban card 82) assessed TypeSafe's Jev decision models, suggesting medication
assertion classification in documents as the first use; nothing was integrated. And two Claude
sessions in one repo turned out to share one board name, so each one's claims read as the other's
own. The second session now gets its own worktree and name.

## Left open

- Card 46: moving the record itself into the container on unraid.
- Card 56: `exercise.session`, waiting for the importers, on branch `card-56-exercise-session`.
- The importer prerequisites in Anam, from gate-composed identity to the envelopes endpoint.
- Jev stays an assessment until a card asks for it.
