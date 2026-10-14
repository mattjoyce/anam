---
id: 202610142310
type: decision
title: "Queue wait spends the caller's deadline up to acceptance; a started write runs to the end"
created: 2026-10-14
status: evergreen
tags: [anam, decision, daemon, concurrency, deadlines, error-kernel]
project: anam
related: ["[[guiding-lens-hickey-armstrong]]", "[[invariants-enforced-by-the-substrate]]"]
---

# Queue wait spends the caller's deadline up to acceptance; a started write runs to the end

**Decision (card 110, from the API blue team's pressure test of 2026-10-11; built 2026-10-14).** A
request's budget, the shorter of the configured `timeout` and the caller's `Anam-Timeout`, counts from
arrival. Waiting for the write queue spends it. Acceptance, the moment the worker starts the job, is
where the caller's say ends.

## The tension

The daemon runs every write through one queue. A fixed five-second importer wait ran before the job's
deadline began, so a batch sent with a 10 ms budget waited five seconds for a 503, and a batch could
wait out its caller entirely and then start writing with a fresh deadline. The importer had long since
timed out and retried. The blue team found it with the queue held.

The card named the tension in Hickey × Armstrong terms. Waiting work should be cancelled promptly.
But an accepted commit should not be abandoned and its outcome misreported. Cancelling a write half
way buys nothing when the write is one transaction, and the idempotent commit makes a resend harmless
either way. So the line falls at acceptance, and the two sides follow different rules.

## The rule

- **Before acceptance.** The end time is absolute, from arrival. The queue wait uses a timer of the
  shorter of the configured wait and the remaining budget, and watches the caller's context. A budget
  that runs out is busy (503 with `Retry-After`), and a caller that goes is dropped. Neither write ever
  starts.
- **At acceptance** the worker checks both again. A select that finds the queue free and the caller
  gone at the same moment may take either branch, so the second check is what makes "never starts"
  true.
- **After acceptance** the write runs to its own deadline, and its real outcome is reported, even to a
  caller who has left.
- **Operator writes** (backups, the mint) keep a deadline that starts at execution. A backup should not
  spend its time in the queue. Each kind of job now says where its clock starts.

The tests hold the configured wait at a minute, so a pass proves the caller's budget or cancellation
ended the wait, without leaning on tight timings.
