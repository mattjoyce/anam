---
type: term
title: Supersession
description: "What happens when a source row changes: a new artifact version and a superseded event against the prior, nothing mutated."
timestamp: 2026-10-11T00:00:00Z
---

When a row with a known [source ref](/terms/source-ref.md) arrives with a different content hash, the gate writes a new artifact version and appends a `superseded` [anam-event](/terms/anam-event.md) against the prior version. The old artifact stays. Same ref and same hash is a no-op, which is what makes re-sending a batch always safe and a crashed import re-runnable.
