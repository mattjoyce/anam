# Card 216 and the three small fixes, through Hickey x Armstrong and Nissenbaum x Solove

After pass on master at 6489310 (card 216, `git diff e02da49 6489310`) and the fixes in `git diff 441e4a0 f3f0ce5`. Read only.

## Verdict

Card 216 is built the way the doctrine asks: the setting is data in the importers file, the receipt stays a value, old receipts keep their bytes, and filing is on the read's path so a failed receipt fails the read. What is not finished is the guarantee the docs state, "no read of facets goes unrecorded": one payload read still escapes, and a recording token's read now rises and falls with the write handle. The privacy question is settled by a role rule, not a content judgement.

## Findings, ranked

1. **dry_run is the unrecorded payload read.** `internal/read/query.go:93` files nothing when `q.DryRun`, and `internal/daemon/query.go:76-82` hands the full answer back regardless. A recording token gets every row of a query unrecorded by adding one flag, the flag anam-mcp sends. Smallest fix: in `postQuery`, before line 82, refuse `dry_run` for a token whose `Records()` is true, with the 400 the timeline gives `explain` (api.go:339). Design call for Matt, since anam-mcp's try-a-program path depends on it; my pick is refuse.

2. **The exact-decimal parser panics on a large exponent.** `internal/query/number.go:97-101`: `len(n.digits)+n.exp` overflows for `1e9223372036854775807`, the guard reads false, and `strings.Repeat` dies with `makeslice: len out of range` (reproduced under Go 1.25). Any reader can put that constant in a program; net/http recovers per request, but AGENTS bans panics. Defect. Smallest fix: in `parseNumber`, refuse `|e|` above a stated invariant with the existing "exponent out of range" error, before any arithmetic on it.

3. **A recording token's read is hostage to the writer.** `internal/daemon/api.go:272` and `query.go:76` file through `d.writes.Ops`, outside the write queue. A mint or backup holding its transaction leaves the receipt `Put` to `busy_timeout` (`ops.go:374`, 5 s, equal to `DefaultTimeout`), and the read fails as a 500 through the default arm of `writeReadError` (api.go:449), while the same read by an unrecorded token succeeds. Nothing corrupts, but busy is reported as broken. Defect, now on every read of a recording token. Smallest fix: map SQLite's busy error to 503 in `writeReadError`, as `postCheckpoints` already answers busy.

4. **Every role lists every reader's receipts.** `api.go:60` admits any role to `GET /v1/receipts`, and `getReceipts` (151) applies only the filter the caller typed. `mayOpenReceipt` (302) keeps a recording token out of another's value, but the list and `verify` hand it the facet ids, query text and name of every other reader's reads. Smallest fix: in `getReceipts`, when `principalOf(r).Records()`, set `q.Principal = who.Name`, so the rule that gates open gates list; the operator still lists all. Design call for Matt; I would clamp.

5. **Rollback breaks the whole list.** `receipt.go:207` refuses unknown fields and `serve.go:483` fails `List` on the first receipt it cannot parse, so once one named receipt exists an older binary's `anam receipt list` and `GET /v1/receipts` fail. A list that silently skips would be worse. Design call: write on the deploy card that rollback past 6489310 is closed once a recording token has read, and leave the code.

6. **GET /v1/timeline answers in two shapes by identity.** `api.go:334-345` branches on `who.Records()`, answers `Published` to one token and `Timeline` to another, and the GET writes. Smallest fix: answer `Published` to everyone with the receipt fields empty, or 400 a recording token's GET to `POST /v1/receipts`. Taste, leaning design call.

## Hickey x Armstrong on card 216

Hickey. "Who read what" sits beside the read path, not in it: `deps.Reader` is one string handed to `serve.Publish`, and the receipt is still a value whose bytes are its hash. Nothing that was a value became a place. The one braid is finding 6, where a handler switches on who; the explainer records it as the one exception.

Armstrong. Filing is inside the read's flow of control: served value first, receipt second, the answer read back from what was filed (`read/receipt.go:221-230`). A receipt can stand for a read the reader never received, when `ReceiptPage` or `versionsOf` fails after the `Put` (lines 230, 245) or the client goes away; that over-records, the safe direction for a log that asks "what was served". There is no read queue to preserve: reads are concurrent, and the list orders by `served_at` then hash. The busy coupling is finding 3.

Tension. Hickey wants the read path untouched by identity; Armstrong wants the record of a read as reliable as the read. The code chose Armstrong and paid in finding 3. Holding both means the read emits the receipt as a value and a supervised writer drains it.

## Nissenbaum x Solove on card 216

Nissenbaum. The data flow is unchanged: subject Matt, sender Anam, recipient a token, through a projection. The new flow is the log: subject a reader, information "what it asked and when", recipient whoever lists. Matt seeing his assistants' reads keeps the norm of his own context. A reader seeing other readers' reads is a flow no context asked for; finding 4 draws the line. The neutral-layer doctrine settles it: who may list is a role, mechanism, never a judgement of what was read.

Solove. Aggregation: the list is a profile of a reader's attention over time, and query text can carry a constant from Matt's life; finding 4 bounds it to the reader's own and the operator. Secondary use: receipts were made for reproduction and now serve accountability, but the subject of the new use is the token and Matt named the purpose. Exclusion: Matt gains visibility. Retention: no deletion path and one value per read, deduplicated by hash; the time filter is enough for now, and a retention, if one comes, is a card 151 default.

Tension. Nissenbaum would record more flows; Solove would name who may see the record. Both land on finding 4 and on judging no content.

## The three small fixes, briefly

Container-pointer refusal (`plan.go:203-240`, `fields.go`): correct; one declaring Definition that says leaf lets the pointer through. Exact decimals (`number.go`, `term.go`): the right call, our own 215 lines beat a module, and `1` and `1.0` still share a key; finding 2 is its one hole. Path hardening (`safepath.go:54-83`): `os.OpenRoot` confines the stat, but the caller later opens the returned path by name, so the comment at 54 overclaims; fix the comment, or return a file opened through the root. `inspect/url.go` rewrites `url.Values.Encode`, which already sorts and escapes; keep the comment and call `Encode`.

## Plainly

Defects: 2, 3. Design calls for Matt: 1, 4, 5. Taste: 6, the safepath comment, the `address` rewrite.
