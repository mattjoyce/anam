---
id: 202607102356
type: decision
title: "Identity is definition-owned — the gate recomputes source_ref and the worker must agree"
created: 2026-07-10
status: evergreen
tags: [anam, decision, gate, identity, source-ref, ingest]
project: anam
related: ["[[source-ref-is-identity-not-content]]", "[[identity-first-keys-and-no-indexes]]", "[[facet-definition-format]]"]
---

# Identity is definition-owned — the gate recomputes source_ref and the worker must agree

**Decision.** At `commit`, the gate composes `source_ref` from the Definition's `identity_key` pointers
over the actual payload, and compares it to the envelope's. A mismatch is a typed rejection
(`ErrIdentityMismatch`), not a shrug and not a silent overwrite.

## The tension

The envelope already carries a `source_ref` — the capture worker knows its rows. Why not trust it?

Because a worker that composes the key itself will drift: a format change on upgrade, a second worker
with its own convention, a timezone normalisation moving a timestamp that participates in the key. Every
drifted key **re-keys the facet** — `facet_id = uuidv5(source_ref)` — so the same source row enters as a
new facet, supersession never fires, and duplicates accumulate with no error anywhere. It is
[[source-ref-is-identity-not-content]]'s failure mode with a different cause: that note guards *what the
key contains*; this one guards *who computes it*.

## Why compare rather than overwrite

If the gate recomputes anyway, the envelope field looks redundant — why not drop it, or silently use the
computed one? Because agreement is the *check*: the worker proving it knows which row it means. Silent
overwrite would hide exactly the drift the rejection is for, and a worker whose keys no longer match its
own rows has a bug worth hearing about on the first envelope, not after a million.

## Consequence

`identity_key`'s open thread in [[facet-definition-format]] ("whether identity_key belongs in meta or
should be derived from source_ref") is settled in the strongest form: **meta owns it, the gate enforces
it, the worker confirms it.** The mutation test removes the comparison and watches the rejection test
fail.
