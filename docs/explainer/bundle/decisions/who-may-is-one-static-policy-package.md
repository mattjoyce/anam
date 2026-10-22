---
type: decision
title: Every who-may rule is one static policy package
description: "internal/policy answers every who-may question as pure Go over the request and the importers file's principals, never a payload; the handlers branch on nothing else about the caller; the flip to a Datalog policy base waits for a stated trigger."
timestamp: 2026-10-23T00:00:00Z
resource: internal/policy/policy.go#Allow
anchor: "func Allow(req Request) Answer {"
basis: code
tier: supporting
tested_by:
  - internal/policy/doctrine_test.go#TestPolicyReadsNoPayload
  - internal/policy/policy_test.go#TestAllow
  - internal/policy/policy_test.go#TestListScope
  - internal/daemon/receipts_reader_test.go#TestRecordingTokenListsOnlyItsOwnReceipts
  - internal/daemon/receipts_reader_test.go#TestRecordingTokenVerifiesOnlyItsOwnReceipts
alternatives:
  - Leave each rule as an if in its handler (how it was; the declared-source rule was walked three times, and the snapshot check and the gate's scopes were one rule in two shapes)
  - Build the policy mesh now, a rule engine over a policy base (Matt, 2026-10-22, "for early personal use it's overkill"; strict leans clinical)
  - Put the policy in the consent gate below the serve boundary (refused; nothing below the handlers knows who is asking, and the consent request carries no principal by design)
affects:
  - /components/principals.md
  - /components/daemon.md
  - /components/inspect.md
  - /components/cli.md
  - /data-structures/importers-file.md
---

**Context.** Card 223. About a dozen places in the daemon, the inspector and the CLI branched on who was calling: the role set on each route, `any_type`, the workbench's allow-list, `receipts: always`, the receipt-open check, the snapshot report's owner, and the declared-source guard on retraction and retirement. Matt, 2026-10-22: "Free, for early personal use, and it's overkill. Maybe we abstract to a policy go, and keep it static and hard coded, ready for a flip later." A survey (`reviews/policy-touch-points-2026-10-22.md`) found 30 touch points, 17 of them policy.

**Choice.** `internal/policy` holds every such rule as a small pure function: `Allow` for a route's roles, `MaySubmitType` and `ScopesOf` (one rule that answers both the handlers and the gate's authenticated pillar), `MayRegisterType` and `MayStoreDocument`, `Records`, `TimelineRead`, `MayOpenReceipt` (which gates verify as it gates open), `ListScope`, `Declared`, `MayRetract` and `MayRetire` for the importers file's sources, and `Held`, `Holdable` and `MayHold` for the admission hold. A handler asks and branches on nothing else about the caller. The route table in the daemon stays the one source for which roles reach a route; `Allow` is handed the route's roles rather than keeping a copy. The list scope is a table of rows, so a per-principal override later is one more row.

**The line it holds.** The policy reads the importers file's principals and the request, never a payload. A test walks every package the policy reaches and refuses any that reads a payload, a fold or a projection; it reaches only `config`, and `facet` for the name of a type. That keeps access a mechanism and never a judgement of content (see [the Envelope is the only ingress](/invariants/envelope-is-the-only-ingress.md) for the other side of that boundary). The admission hold is the first rule whose data is operator-authored state that changes while the daemon runs; it stays inside the line because the daemon reads the hold table inside the write and passes the held set in on the request.

**The flip, later.** The functions can be replaced by the bundled Datalog engine over a policy base (`principal/2`, `flag/2`, `request/3`, and a `policy.dl` rule set versioned and hashed like `anam.dl`, so a receipt can name the policy version that admitted the read). The policy base never includes `attr/3`. The trigger is any of: more than about a dozen rules, a second class of principal (a second person, a clinician's reader with a projection grant), or the first rule that must read beyond the request and the file to answer. Until then the handlers would not change when it flips: they ask the same questions.

**What it costs.** One more package to read for a rule, and an import test that fails if the policy ever reaches for a package outside the short list, by design.
