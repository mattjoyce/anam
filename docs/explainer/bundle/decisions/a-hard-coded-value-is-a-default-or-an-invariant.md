---
type: decision
title: A hard-coded value is a default or an invariant
description: A value someone could reasonably choose differently is a config key with a shipped default and a range check; a value whose change breaks correctness or a contract stays a constant with a comment saying why; no test hook changes either.
timestamp: 2026-10-17T00:00:00Z
resource: internal/config/limits.go#Limits
anchor: "type Limits struct {"
basis: code
tier: supporting
tested_by:
  - internal/config/limits_test.go#TestLimitsOverrideAndRange
  - internal/config/limits_test.go#TestDefaultLimits
alternatives:
  - Leave each number where it was first needed (the waits, bounds and sizes sat as constants in the daemon, gate, client and inspector, so running Anam differently meant editing code)
  - Make every constant a key (would let someone set `MaxBatch` below what the importers send, or a token shorter than the floor, and break the contract from a config file)
  - A package variable a test overwrites (refused; a value a test must change is a default, set the way a person would, and one it must not change is an invariant)
affects:
  - /components/config.md
  - /components/daemon.md
  - /components/gate.md
  - /components/client.md
  - /components/inspect.md
  - /data-structures/limits.md
---

**Context.** Card 151, from Matt's direction that someone running Anam makes their own choices and Anam ships opinionated defaults. The rule is in AGENTS.md ("Defaults and invariants").

**Choice.** A default is a snake_case key whose default is a `Default…` constant in `internal/config` with a comment saying why that number; `Load` range-checks it, it reaches the code as a parameter from the one `Config` loaded at start and is never read from a global or the environment deep in the code, and it is listed in `config.example.yaml`. An invariant stays a constant with a comment saying why it is not configurable. The invariants card 151 named are `gate.MaxBatch` (49), the `Anam-Timeout` header, the snapshot id's shape, `clock.Width`, the token's 32-character floor, the roles, file modes and the library's folder naming. A store's internal tuning that changes no result a person meets, and a page's geometry, are neither.

**Why.** The two failure modes differ. A default set badly is a tuning problem and fails at start with a message naming the key. An invariant set differently breaks something the other end of a wire relies on, which no start-up check can see.

**What it costs.** Every function that used a number now takes it as a parameter (`gate.New` takes `gate.Limits`, `client.New` takes the margin and the response bound, `inspect.Serve` takes its `Deps`), and `config.example.yaml` has 84 more lines to keep true.
