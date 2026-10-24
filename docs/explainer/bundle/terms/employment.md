---
type: term
title: Employment (the org domain)
description: Where the person worked and in what role, kept as a start, changes of role and an end, one series per period with an employer; the first concept in the org domain.
timestamp: 2026-10-24T00:00:00Z
---

`org` is the tenth domain head. It holds the person's relationships with organisations, and employment is the first concept. Membership comes later, as `org.membership`. A head is added only by a logged decision, and this one was decided on 2026-10-24 (card 239); see [where a distinction is encoded](/decisions/where-a-distinction-is-encoded.md). The cost is accepted: `org` sorts by counterparty where the other heads sort by subject, so a shareholding stays with `money`.

Three types are built, each a [Definition](/terms/facet-definition.md) bundled with the binary, written by hand into a records file or saved by an agent.

- `org.employment.started` opens a period of employment and carries the role the person started in: a `title`, and where known the `employer`'s display name, the `legal_entity`, the `location`, who the role reported to, the `grade` and the `terms`.
- `org.employment.role-changed` is a new role inside the same period. It restates the whole role, because a snapshot keeps only the latest row of a [series](/terms/series.md). An acting role or a secondment is a row, and the return from it is another.
- `org.employment.ended` closes the period.

The series is `org.employment`, keyed by `org` and `stint`, the way a medicine course is keyed by drug and course. `org` is one agreed spelling, lower case with words joined by hyphens, the name the person uses for the employer, so the same employer is the same string across every source. `stint` counts the person's periods with that org from 1. Leaving and rejoining is stint 2, a new series, so the first period stays closed. An employer is not an identity type: the organisation is a string, and the legal entity is a field.

A date is a local date, read in Australia/Sydney wherever the employer is, as the other date types are, with no zone pointer. `date_precision` says how much of it the source supports, `day`, `month` or `year`. When it is coarser than a day, `date` is the first day of that month or year, and the schema refuses any other day, so a month known as "September 2019" is written 2019-09-01. `basis` says why the date is believed. An end is written as the first day the person was no longer employed, the day after the last day worked, because a date is the start of its day and the stint is closed from then. Each type requires `source_time`, as `record.document` does, to say how that date was read.

No type here has a pay field, and each one's handling says so. The classification is Confidential. The id is declared, `<source>:employment/<org>/<stint>/started`, the same with `ended`, and `role-changed/<date>` for a role, so a start and an end are one each per stint.

Two bundled projections serve them. `career` is a log over every `org.employment.` type: each start, change and end at its date. `employment` is a snapshot of the three types, which answers "who was I working for on that day" when read at a fact-time bound: the latest row of each period still open, with an ended period gone. Concurrent employments each stand. Neither is part of `recent-life`.
