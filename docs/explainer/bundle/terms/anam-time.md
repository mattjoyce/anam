---
type: term
title: Anam-time
description: When Anam's knowledge changed; the clock an as-of reads, and an audit instrument rather than a history one.
timestamp: 2026-10-11T00:00:00Z
---

Anam-time is the clock carried by [anam-events](/terms/anam-event.md): when Anam learned something. It orders events, and because `seq` is the watermark, an anam-time resolves to exactly one watermark. An unqualified `--as-of` always means anam-time.

A bulk backfill compresses its span: thirty months of [fact-time](/terms/fact-time.md) can arrive in one afternoon of anam-time, so an as-of inside a backfill returns whatever prefix had landed by then. That is honest: Anam did not exist during the backfilled period. It makes anam-time an audit instrument, what a receipt stood on, rather than a history instrument.
