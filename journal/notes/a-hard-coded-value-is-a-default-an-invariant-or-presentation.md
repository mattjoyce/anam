---
id: 202610142300
type: decision
title: "A hard-coded value is a default, an invariant or presentation; only a default gets a config key"
created: 2026-10-14
status: evergreen
tags: [anam, decision, config, defaults, invariants]
project: anam
related: ["[[config-explicit-fails-discovered-defaults]]", "[[invariants-as-structural-tests]]", "[[partition-key-has-no-default]]"]
---

# A hard-coded value is a default, an invariant or presentation; only a default gets a config key

**Decision (Matt, 2026-10-14, cards 151, 152 and 153).** "Someone can make their choices, and we
provide opinionated defaults. So a config should override a default, and we are opinionated about our
defaults." Every hard-coded value in Anam, anam-importers and anam-mcp was sorted, and the rule is in
each repo's AGENTS.md.

## The tension

Values had been split between config and constants by habit. Config held paths, timeouts and a few
sized limits. Other bounds of the same kind were Go constants with no override: the 64 MiB document
cap, the five-second importer wait, the 24-hour snapshot lifetime, the client's response cap. An
operator with a different machine or record had to edit code.

Making everything a knob pulls the other way. Some values cannot change without breaking the record
or the wire, and a key for one of those is an invitation to corrupt data. Others change no result
anyone relies on, and a key for those bloats the config with settings nobody turns.

## The rule

- **A default** is a choice someone could reasonably make differently. It gets a snake_case key and
  its `ANAM_` variable, a range check at load with a ceiling as well as a floor, and its old value as
  the default, so adding the key changes nothing at deploy. The limit reaches the code as a parameter
  from the one `Config` loaded at start, never read from a global or the environment deep inside.
  Core gained 21 such keys in `internal/config/limits.go`.
- **An invariant** breaks correctness or data if changed: the batch of 49 that fixes which events
  share an anam-time instant, schema versions, the snapshot id pattern, file modes, the token's
  32-character minimum. It stays a constant with a comment saying why.
- **Presentation and tuning** values change no result: inspector geometry, page sizes, clip widths,
  store batch sizes. They stay constants and change with the page or the code.
- A key already set in a live deployment keeps its name even when the name is stale. `ingest_timeout`
  outlived ingest because unraid sets `ANAM_INGEST_TIMEOUT`.

## How it relates to explicit config

[[config-explicit-fails-discovered-defaults]] said Anam runs on defaults when no config file is
found, and fails when a named one is missing. This decision complements it. That note settles where
settings come from, and this one settles which values are settings at all. The same fail-fast line
runs through both. Unknown keys still fail at load, so a misspelt override is never silently
ignored, and an out-of-range value fails at start rather than limping on.
