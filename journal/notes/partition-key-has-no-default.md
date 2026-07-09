---
id: 202607100015
type: decision
title: "The partition key has no default — an implicit person_id is a silent data hazard"
created: 2026-07-10
status: evergreen
tags: [anam, decision, config, storage, safety]
project: anam
related: ["[[person-substrate-grain]]", "[[config-explicit-fails-discovered-defaults]]", "[[invariants-as-structural-tests]]"]
---

# The partition key has no default — an implicit `person_id` is a silent data hazard

**Decision.** `person_id` has **no default**. `config.Load` fails if it is unset. Every other setting
(`db_path`, `timeout`) keeps a sensible default.

## The tension

A personal, single-user tool wants to run with zero configuration. Anam has exactly one person, whose id
is known; defaulting `person_id: matt` costs nothing and saves a config file before first run. That is
the *easy* choice, and for the other settings it is also the *simple* one.

`person_id` is different in kind, not degree. It is not a preference — it is the **storage substrate**.
Every row is written under `person_id` ([[person-substrate-grain]]). Nothing downstream validates it,
because a person partition is created by writing to it.

So a wrong `person_id` does not fail. It **succeeds, into the wrong partition.** The ledger appears
empty, a re-ingest looks like a first ingest, `changes_since` reports nothing, and the facts sit in
`person#mat` where nothing will ever read them. This is the same failure class as the `db_pth` typo that
silently redirected Anam to the default ledger — but the partition is worse: a wrong path at least
produces a visibly empty database, and a wrong partition inside the right database never fails at all.

## The rule this generalises to

> Default a setting when a wrong value **fails**. Require it when a wrong value **succeeds somewhere
> else.**

A default is a promise that the unconfigured case is safe. For a ledger path or a timeout, the
unconfigured case is safe: you find out immediately. For the key that decides *where your facts are
written*, there is no immediate feedback, and a default converts a loud error into a quiet one. The
cost of requiring it is a single line in a config file, paid once.

The same reasoning rejects the tempting `person_id: $USER`. Deriving the partition key from the
environment means the ledger silently re-partitions when the environment changes — the same hazard,
wearing a cleverer hat.

## Consequence

`viper` needs the key **bound** even though it has no default: `Unmarshal` decodes from viper's known
keys, and a key with neither a default nor a config entry is unknown, so `ANAM_PERSON_ID` would be
ignored. `v.BindEnv("person_id")` registers it without supplying a value. Bind *after* `SetEnvPrefix` —
`BindEnv` resolves the `ANAM_` prefix at call time. (`AutomaticEnv` happens to mask the wrong order
today. That was established by mutating the code and watching what broke, not by reading the docs, and
the comment in the source says not to rely on it.)
