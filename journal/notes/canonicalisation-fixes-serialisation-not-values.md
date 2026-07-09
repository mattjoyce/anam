---
id: 202607100011
type: decision
title: "Canonicalisation fixes serialisation, never values"
created: 2026-07-10
status: evergreen
tags: [anam, decision, facet, payload, hashing, idempotency]
project: anam
related: ["[[verbatim-payload-metadata-separation]]", "[[fixed-width-timestamps-lexical-order]]", "[[facet-definition-format]]"]
---

# Canonicalisation fixes serialisation, never values

**Decision.** A payload is canonicalised on admission — object keys sorted by byte order, insignificant
whitespace removed, numbers preserved as their **source lexeme** — and the canonical bytes are what get
stored and hashed.

## The tension

Two of Anam's commitments appear to contradict each other.

- [[verbatim-payload-metadata-separation|Payloads are verbatim]]: the body is held exactly as the source
  emitted it, never normalised. This is what makes *"Anam owns no fact"* true.
- **`commit` is idempotent** (§6a): re-ingesting the same source row must be a no-op, detected by
  `content_hash` equality. A *changed* row must hash differently and supersede.

But a source row read through a Go `map[string]any` serialises its keys in **random order** on every
capture. Hash the raw bytes and the same unchanged row hashes differently each run: every re-ingest
looks like a supersession, the ledger accretes phantom versions, and idempotency — the property that
lets a crashed capture worker be re-run to known-good — is gone.

## The resolution

The apparent contradiction dissolves once you separate **a value** from **its representation**.

> "Verbatim" is a claim about *content*, not about *bytes*. Canonicalisation pins the representation so
> the content can be compared. It rewrites how the value is written down; it never rewrites the value.

Sorting keys does not change what a JSON object *is* — `{"a":1,"b":2}` and `{"b":2,"a":1}` are the same
value. Stripping whitespace does not change it. So canonicalisation is not a violation of the verbatim
rule; it is the **precondition for enforcing it**, because only a canonical form can be content-addressed
at all. This is Hickey's point that a value's identity is its content, and the corollary that you must
first decide what counts as "the same content."

## The number trap

The line between representation and value is sharpest — and easiest to cross by accident — at numbers.
Go's `encoding/json` decodes every number into a `float64` unless told otherwise. Round-trip through
that and:

- `1.50` is rewritten `1.5` — a trailing zero the source chose to record is destroyed;
- `9007199254740993` is silently rounded — it exceeds float64's exact-integer range;
- `1e3` becomes `1000`.

Those *are* value changes, and they would reach the ledger as counterfeit source data. So the decoder
runs with `UseNumber()`, holding each number as its original lexeme and emitting it untouched. The
canonicaliser therefore sorts and strips, and does nothing else.

Note the asymmetry that makes this safe: **key order carries no information; number spelling might.**
When in doubt about whether a transformation touches representation or value, ask whether any two
distinct source rows could be mapped onto the same bytes. Sorting keys cannot do that. Float coercion
can.

## Consequence

`facet.Canonicalize` is idempotent (`canon(canon(x)) == canon(x)`), so the stored bytes are already
canonical and `ContentHash` is just `sha256` over them. Strings are encoded without HTML escaping, so a
payload containing `<` reaches the ledger as written.

The hash of the canonical bytes of `{"a":1}` is pinned in a test. If either the canonical form or the
hashing changes, every idempotency marker already in the ledger is orphaned — so that change must be a
deliberate, migrated one, never a silent refactor.
