---
id: 202607100016
type: decision
title: "An explicitly-named config file must exist; a discovered one need not"
created: 2026-07-10
status: evergreen
tags: [anam, decision, config, cli, xdg]
project: anam
related: ["[[partition-key-has-no-default]]", "[[guiding-lens-hickey-armstrong]]"]
---

# An explicitly-named config file must exist; a discovered one need not

**Decision.** Anam searches for its config file in one order, stopping at the first hit:

1. `--config <path>` — **explicit**
2. `$ANAM_CONFIG` — **explicit**
3. `<user config dir>/anam/config.yaml` — **discovered**

A path named **explicitly** that does not exist is an **error**. A **discovered** path that does not
exist is **not** an error: Anam runs on defaults.

## Why the asymmetry

Both cases end with "the file isn't there," and it is tempting to treat them alike — fall back to
defaults, carry on. That is the *easy* answer and it is wrong, because the two cases differ in what the
operator **asked for**.

Typing `--config ./prod.yaml` is a statement of intent. If the file is absent, silently loading defaults
means Anam starts against `localhost` while the operator believes it is running against production
config. Nothing errors; the wrong thing simply happens. Whereas the discovered path was never promised
by anyone — its absence is the ordinary first-run state, and defaults are exactly what should happen.

> Fail on the absence of something requested. Shrug at the absence of something merely looked for.

This is the same fail-fast reasoning that makes an unrecognised config key an error rather than a shrug,
and it is Armstrong's: crash where the assumption is violated, do not limp on in a state the operator
did not intend.

## Two smaller choices

**No project-local `./anam.yaml`.** It is convenient during development and a footgun in use: the same
command run from two directories would write to two different ledgers. Discovery must not depend on the
working directory. Development overrides go through `$ANAM_CONFIG` or the flag, which are explicit and
therefore honest.

**`os.UserConfigDir()`, never a hardcoded `~/.config`.** The stdlib function already honours
`$XDG_CONFIG_HOME` and falls back to `~/.config`. Hardcoding the tilde path silently ignores an operator
who has moved their config root — a small instance of the same sin: doing something other than what was
asked, without saying so.

## Consequence for structure

Discovery and decoding are **separate functions** — `Resolve(explicit) (path, Origin, error)` and
`Load(path)`. Braiding them would make discovery untestable without a real `$HOME`, and it would hide the
`Origin`. Anam is a provenance system; *where a setting came from* deserves the same answer as *where a
facet came from*, and a command can now report it.
