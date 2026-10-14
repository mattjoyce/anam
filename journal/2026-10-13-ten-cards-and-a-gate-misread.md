# 13 October: ten cards, three holes closed, and a gate read wrong

Matt was away for most of the day and handed the dispatcher a list by number: "#50", then "#125,
#21", then easier cards while out. By a few minutes past midnight ten cards had merged. Two
added things you can see, the checkpoint strip and registered projections. Three closed holes a
review had found. The rest retired the health ETL from the code, fixed a note's time, cleared the
day audit's small findings and kept the Definition history through a rebuild. One of them, card 108, went to master on a gate that had failed. Master was red
for a few minutes, and the reason is worth more than the fix.

## The health ETL leaves the code (cards 50 and 123)

Card 55's cut-over on 11 October had already retired health-db's 5,818 summary facets, so
`internal/capture/healthdb` had been dead code for two days. Card 50 deleted it, with its scan case,
its scope entry and the example import. Tests that used health-db as "a declared source whose path
is missing" now use documents. `ARCHITECTURE.md` §2 draws importers posting envelopes under their own
tokens where capture workers used to be.

Card 50 had a fourth criterion it could not meet, that the record can be rebuilt from its sources
alone. Rather than tick it, the card closed and the criterion became card 141, the rebuild drill.
Card 123 then redrew the pipeline page: every source row ends in its importer on the unraid gateway
and the daemon, and the ETL appears only as history.

## The checkpoints become visible (card 21)

A strip under the timeline band draws one segment per leaf bucket of the mint's layout, in the
band's own x scale. Each segment is sealed, behind or never minted, and links to a zoom on its
bucket. The header counts them and says when the mint log is unsealed.

The state comes from the one comparison [[hard-and-soft-checkpoints]] already names: the live value,
partitioned by the layout the log implies, hashed per leaf and compared with the stored heads. It is
computed for the page and dropped. A soft checkpoint is never cached, because a cached one would be
a second place for state to live. The inspector asks for a narrow `Checkpoints` interface, and with
none it draws no strip, so `anam inspect` over a bare ledger is unchanged.

## Projections are registered like Definitions (card 125)

Until now a projection template was only what the binary bundled. Card 125 made the bundled set a
floor and let the operator register more over `POST /v1/projections`, with the same rules as
`def.Register`: the same body at a held version is unchanged, a different body is refused, versions
only go forward, and every type a template names needs a Definition in force.

The question was who decides what a read looks like. The consumer selects a projection by name. The
operator defines it, and every version stays held, because a receipt names one
([[a-consumer-selects-a-projection-the-operator-registers-it]]). Receipts now carry
`template_hash`, and verify looks up the exact version and compares it. Another body at that version
is `ErrTemplateMoved`, which is the point of [[published-is-a-receipt-not-a-record-event]]. A receipt
that could be satisfied by a template edited later would prove nothing.

The `registered_projection` table is authored, not provisioned. The operator's file has no git home
that Anam knows, so it comes back from a backup or by registering again, and triggers make a version
immutable. Heart v1 and sleep v2 wait in `projections/` to be registered on the live daemon after
the deploy, so the card stayed in doing.

## A note's time is when it was written (card 136)

`note.annotation` v1 left `at` loosely described, and the inspector drafted it as the earliest mark
selected. That copies a time the noted facets already hold. Each facet carries its own fact time,
and anam-time is the ingest, so the one time nothing else holds is the moment the note was written.
v2 says so. Identity and validation did not change, so v1 values stay valid and the existing note
resent as a no-op.

## Three holes from the review (cards 105, 106 and 108)

**Card 105.** A facet's id is its source and identity values. The type is not part of it, by
[[identity-is-definition-owned]]. So two types from one source whose identity values coincide are one
id, and the second would have been admitted as a new version of the first, an overwrite in
everything but name. The gate now compares the current version's type with the envelope's and
refuses a change as `ErrTypeChanged`, joining the same 422 the other pillars use. A dimension left
out of the identity ([[the-type-is-not-in-the-identity-so-the-gate-refuses-a-type-change]]) needs a check at admission, or it becomes a place where a value can be
replaced. A scan of the live ledger for any facet that had already changed type was asked of
ThinkPad.unraid.

**Card 106.** The inspector shares its origin with the API, so an HTML or SVG document from the
library, served inline, could have run as the operator and read the record. Every `/document`
response now carries a CSP sandbox with no scripts and no origin. HTML, XHTML, SVG and XML download
instead of rendering. Images, PDFs and text stay inline.

**Card 108.** A browser that has cached basic auth for the inspector will send it from any page.
`browserGuard` now wraps the whole handler ahead of `authorize`. For an unsafe method, a
`Sec-Fetch-Site` other than `same-origin` or `none`, or an `Origin` whose host is not the request's,
is 403 before the token is read or any work queued. On a tokenless daemon a non-loopback `Host` is
refused too, which is the DNS rebinding case. The guard has to run first because the credential it
protects is exactly the one an attacker's page would spend.

## Hickey × Armstrong

**Hickey.** Three of the day's cards are the same lesson in different clothes. The type was braided
out of the identity on purpose and needed its own check. The template was braided into "whatever the
binary holds" and got its own versioned registry. The note's `at` copied a value it should have left
to the facets. In each case the fix was to name the separate thing and give it one home.

**Armstrong.** Card 105 hardened the error kernel, and its refusal writes nothing in a batch, the
whole-or-nothing every pillar keeps.

## The gate that was read wrong

Card 108 merged at 23:45 on a background task notification that said exit 0. The log ended
`CHECK FAILED` and `exit=1`. The notification reports the shell line that wrote the log, which always
succeeds. The one failure was a test, not the guard: `httptest` names the host `example.com`, and a
tokenless daemon now takes a POST only from a loopback name, so the ingest-wait test was refused 403
before its wait was read. The fix, c0cf17d, landed two minutes later.

The rule since is to read the gate's own last lines, `CHECK OK` and `exit=0`, before a merge, and to
leave Go files alone while a gate runs in that tree. A test phase that compiles whatever is on disk
at that moment gives a green result belonging to neither version.

## The audit's small findings, and the history (cards 100 and 111)

Card 100 cleared the day audit's low findings. Among them, `read.Absent` takes a bound the daemon
sets and answers 413 past it, and `inspect.go` was split into files per page.

Card 111 shipped at 00:04. `manifests/history/` now carries every earlier version a facet may have
been admitted under, recovered from git: blood pressure v1 to v4, measurement and weight v1 and v2,
exercise.session v1 to v4. The start-up upsert provisions them, so a rebuilt operational store holds
every version without a one-time restore. A history body is words, not a contract. It is checked to
be JSON naming its type and version and is never parsed under today's lint, because an old version
was valid under the rules of its own day. `Bundled` and `SetVersion` skip the history, so what
envelopes are admitted against did not change.

## Left open

- Card 125: register heart v1 and sleep v2 on the live daemon after the deploy.
- Card 105: the live ledger scan for a facet that already changed type.
- Card 21: two of the four ways the card described were not built.
- Card 141: the rebuild drill, carried from card 50.
- Card 139, drafted today: types name concepts and brands are sources, with `garmin.day` maybe the
  exception. Card 140, drafted today: a migration importer from one ledger to another.
