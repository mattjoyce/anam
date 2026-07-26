---
id: 202607100012
type: decision
title: "Invariants become structural tests, and the tests are mutation-checked"
created: 2026-07-10
status: evergreen
tags: [anam, decision, testing, invariants, go, armstrong]
project: anam
related: ["[[guiding-lens-hickey-armstrong]]", "[[fixed-width-timestamps-lexical-order]]", "[[canonicalisation-fixes-serialisation-not-values]]"]
---

# Invariants become structural tests, and the tests are mutation-checked

**Decision.** Each entry on `ARCHITECTURE.md` §12's do-not-break list is carried into code as a test that
**fails when the invariant is broken** — including invariants about *shape*, not just behaviour. Then each
such guard is verified by deliberately breaking the invariant and watching it fail.

## Why a comment is not enough

§12 says *"current status is derived by the fold from record-events — never stored on the artifact."*
Written as prose in a doc, that survives exactly as long as everyone has read the doc. The failure mode is
mundane: some future session, mid-refactor, adds `Status` to the `Facet` struct because a call site would
be tidier with it. Nothing breaks. The tests pass. The ledger now has two sources of truth — the events,
and a field — and they drift apart the first time one is written without the other.

The invariant is about the **type's shape**, so the guard must be about the type's shape:

```go
rt := reflect.TypeOf(facet.Facet{})
for i := range rt.NumField() {
    name := rt.Field(i).Name
    if strings.Contains(name, "Status") {
        t.Errorf("Facet has field %q: current status must be derived by the fold, never stored", name)
    }
}
```

This is an unusual test — it asserts on a struct's fields rather than on behaviour, which normally smells
of testing internals. It earns its place because **the struct's shape *is* the invariant.** There is no
behaviour to test: the whole point is that no code path exists to overwrite a status, and the way to prove
a path doesn't exist is to prove the field doesn't. The same pattern guards §5's one-directional
dependency (a `RecordEvent` references a `Facet`; a `Facet` never references a record-event).

Armstrong's error kernel is the frame. The ledger plus the gate invariants are the small trusted core that
must not corrupt. A guard on the core's *shape* is cheaper than any amount of care about its *use*.

## The second half: test the test

A guard that passes tells you nothing. It might be asserting a tautology, or asserting on the wrong thing,
and you would never know — a green tick looks identical either way. So each guard was checked by
performing the mutation it exists to catch:

| Mutation | Guard that must fail |
|---|---|
| `Layout` reverted to `time.RFC3339Nano` | lexical-vs-chronological ordering test |
| `dec.UseNumber()` deleted | number-preservation, hash-stability, idempotence |
| a `Status` field added to `Facet` | the reflection guard above |

All three failed as intended, and were restored. This costs about a minute and converts "the tests pass"
into "the tests would have caught it" — which is the only claim worth making.

> Prefer the invariant you can break on purpose. A test you have never seen fail is a hypothesis, not a
> guard.
