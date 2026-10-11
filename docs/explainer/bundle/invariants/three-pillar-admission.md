---
type: invariant
title: Admission is three pillars through the one gate
description: Facet-compliant (the Definition's closed schema and time semantics), authenticated (the source's token scope), associated (bound to the person with assurance); nothing enters except through resolve then commit.
timestamp: 2026-10-11T00:00:00Z
resource: internal/gate/commit.go#admitChecks
anchor: "if !g.scopes.Allowed(env.Source, env.Type) {"
basis: code
tier: supporting
enforced_at: internal/gate/commit.go#admitChecks
enforced_anchor: "if !g.scopes.Allowed(env.Source, env.Type) {"
applies_to:
  - /components/gate.md
  - /components/daemon.md
  - /terms/envelope.md
  - /components/def.md
tested_by:
  - internal/gate/gate_test.go#TestCommit_TypedRejections
  - internal/gate/batch_test.go#TestCommitBatch_RefusesWithEveryFailure
---

ARCHITECTURE §12, eleventh bullet, first half, and §6. This is where "validate all external input" lives: source rows are external input, checked before admission. The order inside `admitChecks` decides which error an importer sees first: envelope valid, binding for this person, scope, Definition exists, version current, assurance meets the floor, payload under 300 KiB, decodes, passes the schema. A batch reports every failure at once and writes nothing.
