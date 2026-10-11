---
type: term
title: Anam-event
description: An assertion that Anam's knowledge changed, in one of exactly five kinds, each with a gapless sequence number.
timestamp: 2026-10-11T00:00:00Z
---

An anam-event records that Anam's knowledge changed. There are exactly five kinds: `admitted`, `superseded`, `tombstoned`, `endorsed` and `rejected`. Each carries [anam-time](/terms/anam-time.md) and a monotonic, gapless `seq`. Every kind can change the live-set, so the [watermark](/terms/watermark.md) is the sequence itself.

Publication is deliberately not an anam-event: serving a facet changes nothing Anam knows, and a [receipt](/terms/receipt.md) records what was shown. An anam-event references a facet; a facet never references an anam-event. The row is the [anam-event row](/data-structures/anam-event.md).
