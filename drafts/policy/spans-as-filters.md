# Spans as filters: time-bounded access to the record

Draft for a card. v2, 2026-10-25, after anam-phone-log read v1 against anam master 45e4a67 and
found eight things, all taken here. v1 is in this file's history. Written on Matt's behalf by
the Shearline lead; the card is Matt's to cut.

## Job story

When I give a principal a token, I want its grant to name a window of time as well as the
projections it may read, so I can share this year's record, or last quarter's, without
sharing the whole of it.

## Summary

A reader's grant gains a window. A read is narrowed to the window before it runs, the way
`--from` and `--to` narrow it today, and the receipt shows the narrowing. Nothing is refused
for asking too widely; the read is clamped. No new dependency: the first version is a few
lines in `internal/policy` and one clamp in `queryOf`. The Datalog version comes at the flip
card 223 already names, and this draft says what it will look like so the first version does
not paint it out.

## Vocabulary

This feature uses the **derived span** of the Glossary, the lens a person names as
`[from, to]`. It never touches the **bucket span**, the minted checkpoint unit. Two words:

- **Window.** A derived span on a principal's grant: `[from, to)` in the record's zone, the
  zone `clock.ParseBound` already resolves a bare date in. A grant window is resolved exactly
  as a request's bounds are, by the same code, so the two can never disagree at an edge.
- **Clamp.** The read's bounds intersected with the window. A request wider than the window is
  served for the overlap; a request wholly outside it is served empty, with a receipt that
  says why.

Membership is by **fact-time**, as `bounds` decides it today. A facet whose
`effective_time_frame` straddles the window edge is in or out by its fact-time alone, the same
rule a `--from` read applies; the draft does not introduce a second rule.

## What a grant names

Doctrine: roles grant projections, projections are the type allow-lists, nothing lower. So a
grant names **projections**, not facet types, and the window applies to every projection the
grant names. Today every role reads every projection (`daemon/api.go:83`), so the first gap to
close is binding a reader to its projections at all; the window rides on that.

In the principals file:

```yaml
principals:
  - name: clinician-2026
    role: reader
    projections: [heart, sleep]
    window: {from: 2026-01-01, to: 2027-01-01}   # record's zone, half-open
```

A principal with no `window` reads all of time, as today. A principal with a `window` and a
request with no `--from`/`--to` reads the window, not all of time: the grant bounds the read
when the request names no bounds. That is the rule v1 got backwards.

## Where it sits in Anam

- `internal/policy` (static Go, per card 223) gains two checks: the projection is in the
  principal's list; the window, if any, is handed to the read.
- `queryOf` in `internal/read/parse.go` clamps `bounds` to the window before building the
  `receipt.Query`. Every read already narrows through this one place (card 92), so the
  timeline, a published read and a saved selection clamp the same way, and the receipt
  records the clamped `From`/`Before` as the narrowing that was applied.
- The receipt body is unchanged in shape. A clamp shows as the bounds it produced, not as a
  new field, so existing receipts hash as they did. Nothing about the decision is attached to
  the hashed body.

## Acceptance

- A reader with `window: 2026` and no flags reads 2026 only.
- The same reader asking `--from 2025-12-01 --to 2026-03-01` is served December excluded,
  January and February included, and the receipt's `From` is 2026-01-01 in the record's zone.
- The same reader asking for 2024 is served an empty read with a receipt, not a 403.
- A Sydney reading at 09:00 on 1 January 2026 is inside a 2026 window; the last hours of
  31 December 2026 UTC that are 2027 in Sydney are outside it.
- A principal with no window, and the operator, read exactly as before: same receipts, same
  hashes.
- A reader asking a projection not in its list is refused by `internal/policy` with the
  refusal naming the projection.

## The Datalog version, for the card-223 flip

When `internal/policy` moves to the engine (past a dozen rules or a second class of
principal), the same feature is expressed as facts and one rule set after `policy.RBAC()`:

- Resources are `projection/<name>`; `Grant(role, "read", "projection/heart")`.
- A window becomes a set of **units**, ISO prefixes at one resolution, named in the record's
  zone: `2026`, `2026-06`, `2026-06-15`. A coarser unit is a string prefix of every finer one
  under it, so `prefix/2` gives the hierarchy. Grants are `span_grant(Role, Unit)`.
- The request carries the units its bounds resolve to, at the grant's resolution, and the
  rule set says which are covered. The read then clamps to the covered units, so the Datalog
  decision stays a filter, not a gate. Why-not names the first uncovered unit for the receipt.
- Carrying many units on one request needs a many-valued context key in Shearline's `policy`
  package, a small extension; until then it is one `Decide` per unit.

That version is not this card. It is written down so the first version chooses the same
resolution rule, the same zone and the same clamp semantics, and the flip is a change of
mechanism, not of behaviour.

## Not in scope

Attribute-based access over payload values. Per-field grants. Grants on bucket spans. Any
notion of sensitivity: Anam never assesses it; a window is a window.
