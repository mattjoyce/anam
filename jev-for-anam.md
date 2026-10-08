# Jev for Anam

Written 2026-10-08 for agents working on Anam. This is an exploration and recommendation,
not an approved implementation spec. No Jev integration or evaluation has been performed.

**My assessment: Jev could help Anam distinguish what a passage asserts, check candidate
extractions, and find relevant evidence. Its strongest initial application is medication
assertion classification in document processing.** The ledger mechanics already have exact
answers; the opportunity is in interpreting source material around those mechanics.

## What Jev is

Jev is TypeSafe AI's first public **System One model**: a model built to return bounded,
typed judgments for software. A caller supplies a state containing the relevant evidence
and named questions with defined answer spaces. The response contains values the caller
can branch on rather than generated prose. TypeSafe describes its training approach as
Reinforcement Learning for Calibrated Decisions (RLCD). [TypeSafe overview](https://typesafe.ai/)

Its three primitives are:

| Primitive | Question shape | Result |
|---|---|---|
| Choice | Which defined option fits? | Selected option, option probabilities, confidence |
| Score | Where does the evidence fall on an ordered rubric? | Score, level probabilities, confidence |
| Noul | Is this specific statement true? | Probability of yes, from 0 to 1 |

Questions in a request are evaluated independently against shared state. If one decision
depends on another answer, code must compose the answers or make a subsequent request.
TypeSafe recommends narrowly scoped judgments rather than bundling several dimensions into
one question. [Introduction](https://docs.typesafe.ai/introduction)

For an agent, the useful distinction is between **supplying a judgment** and **doing the
investigation**. Jev can judge a passage you have selected. Retrieval, candidate construction,
policy, tool execution, and explanation remain work for code or an agent workflow.

Typed output constrains the answer's form; it does not establish the answer's truth.
Choice and Score confidence summarize their probability distributions. Noul has no separate
confidence field. Neither a concentration statistic nor the vendor's calibration claim is
an observed error rate on Anam's documents. [Confidence](https://docs.typesafe.ai/confidence)

## Anam's relevant boundaries

Read [ARCHITECTURE.md](docs/ARCHITECTURE.md), [INBOX.md](docs/INBOX.md), and
[SELF-LOG.md](docs/SELF-LOG.md) before turning these ideas into implementation work.

Anam assembles one person's longitudinal record from source systems. Captured payloads are
held verbatim; immutable facets enter through the admission gate. Current status is a fold
over events. Fact-time describes the world; anam-time describes changes in Anam's knowledge.
Published projections apply versioned metadata filters and carry receipts.

The inbox is a particularly useful seam. Some document formats already have deterministic
extractors. Other documents are read by an agent, which writes a records file for review.
The runbook documents repeated historical lab results, uncertain dates, OCR errors, and
other people's results inside a scan. These are semantic problems that schema validation
alone cannot resolve.

The architecture also defines derived findings admitted as `proposed`, with provenance and
endorsement kept separate. The inbox runbook explicitly says that proposed lifecycle is
not built. Treat it as a design direction, not an available containment mechanism.

## Candidate problems, in priority order

### 1. Classify what a medication passage actually asserts

Given one passage and a named medication, distinguish assertions of a start, dose change,
stop, current use, historical use, recommendation, negation, or insufficient evidence.
This helps a document-processing agent decide whether there is a candidate event to extract.

Illustrative cases, with expected interpretations rather than measured Jev answers:

| Passage | Interpretation |
|---|---|
| “Started ramipril 5 mg daily on Monday.” | An asserted start; the date still needs resolution |
| “Previously on ramipril; consider restarting.” | Historical use and a recommendation; no asserted restart |
| “Ramipril increased to 10 mg daily.” | An asserted dose change |
| “Current medications: ramipril 5 mg.” | Current-use evidence; the start date is unstated |
| “Did not stop ramipril.” | Negated stop; no stop event |

A passage can contain several assertions. Split it into candidate clauses or ask independent
questions about each assertion; a single exclusive Choice should not erase multiple events.
Keep an explicit insufficient-evidence outcome. The medication, course identity, dose, and
date must still come from evidence and the extraction workflow.

This is my preferred first experiment: the task is narrow, connects directly to Anam's
existing regimen types, and has consequential distinctions that simple keyword matching misses.

### 2. Check candidate records against source evidence

Give Jev a candidate record and its cited passage, then ask separate questions: is the event
asserted, is the subject supported, and does the passage support the selected date's role?
Return supported, contradicted, or insufficient evidence for each semantic question.

Code should check exact numeric values and units wherever parsing permits. A model can help
select which printed value belongs to an analyte, but the selected value should be copied
from an identified source span and validated deterministically. Agreement between two models
is useful triage evidence, not proof; both can inherit the same OCR mistake.

This would complement the scratch-ledger validation already in the inbox runbook. The gate
checks admission contracts; an evidence checker asks whether the proposed content was actually
said. Failed or uncertain checks should retain the candidate and evidence for review.

### 3. Route documents to an existing processing path

Given extracted header text and relevant format markers, choose an ACL report, other pathology
report, prescription, diary, or unknown processing path. Use deterministic signatures first;
Jev earns its place for ambiguous inputs.

This decision selects an extractor. It does not certify that the extractor succeeded, that
every page is the same kind, or that every result belongs to Matt. Mixed documents need
page-level handling. An unknown route leaves the document pending for manual processing.

### 4. Flag possible duplicate observations across sources

Code first finds candidate pairs by compatible type, time, and source identifiers. Jev then
judges whether their descriptions suggest the same observation, separate observations, or
insufficient evidence. The inbox's historical lab columns are a concrete example.

Known lab numbers, hashes, and identical source keys are exact comparisons and belong in code.
The semantic judgment helps with ambiguous descriptions. It should flag a pair for review;
it should not rewrite stable identities, merge facets, or discard observations. Two nearby
blood-pressure readings may be two real measurements.

### 5. Rank evidence for a specific agent question

After authorized reads and deterministic narrowing by type and date, score candidate passages
against the question an agent is investigating. For “what was happening around this cluster
of heart-rate alerts?”, relevance questions might concern an explicitly recorded illness,
travel, or medication change.

The relevance result belongs to the consumer's retrieval workflow. It must respect the
projection's allowed evidence and must not become a new hidden projection filter. A ranking
can help an agent decide what to read; it cannot establish causation or record completeness.

## Tasks that should stay exact

Anam already derives “what medication was I on at date D?” through series and snapshot
semantics. Watermarks, supersession, liveness, idempotency, bucket membership, source hashes,
and projection eligibility also have exact procedures. Keep those procedures authoritative.
The expected-dose schedule in `internal/regimen` is arithmetic over supplied facts.

Likewise, compute numerical trends and temporal overlap in code. Jev might judge whether a
note explicitly describes illness; that judgment does not show illness caused a change in
heart rate. An investigation needs evidence selection and reasoning beyond one bounded call.

This division also matches TypeSafe's documented limitations for `jev-1.13`: numeric precision,
date ordering, irrelevant context, indirection, and option-order sensitivity. The guidance is
version-specific and should be rechecked when selecting a model.
[Documented limitations](https://docs.typesafe.ai/model-jaggedness/jev-1.13)

## Design assessment: Hickey × Armstrong

**Hickey — separate evidence, interpretation, and policy.** A Jev answer is an authored
judgment about evidence. Keep the original payload intact and retain the judgment's model
version, question version, input references, and probabilities separately. Code owns the
policy that turns a judgment into routing or review. Preserve existing identity and time
semantics. If a finding eventually enters the ledger, use the derived-facet contract and
identify the evidence versions and watermark it depended on.

**Armstrong — contain a fallible external worker.** Put Jev calls in a disposable document
or consumer worker with context deadlines and bounded retries. Timeout, rate limiting,
invalid responses, and worker crashes leave work pending; a later run can resume. The gate,
fold, projection, and ordinary reads should continue independently. Persist a completed
judgment before using it in a write-producing workflow, so retrying admission does not silently
substitute a new inference for the earlier one.

Both lenses support a small seam outside the error kernel. Their tension is worth preserving:
keeping judgments disposable simplifies runtime recovery, while judgments relied on by a
durable finding need enough recorded evidence to explain the finding later. Persist according
to use; avoid making every transient relevance score a ledger event.

## Suggested first experiment

Run medication assertion classification in shadow mode against a reviewed set of passages.
The experiment produces an evaluation artifact outside the live ledger and changes no records.

1. Build a labelled set covering starts, changes, stops, current lists, history, recommendations,
   negation, mixed assertions, missing dates, and uncertain OCR. Retain document and passage refs.
2. Compare Jev with a simple keyword baseline and the existing agent reading workflow. Group
   development and held-out cases by document so repeated passages do not leak across splits.
3. Pin the model and question definitions. Record answers, probabilities, latency, cost, and
   failures. Test representative option permutations and irrelevant-context perturbations.
4. Report errors by category, especially invented starts or stops. Measure accuracy among
   accepted answers together with coverage: abstaining on everything is not useful automation.
5. Choose per-question thresholds from development results, then evaluate on held-out documents.
   Before making hosted calls, explicitly settle which document excerpts may leave the source
   environment. This document does not authorize transmitting private records.

The decision to proceed should depend on measured reduction in review work at an acceptable
error rate, including operation when the service fails. Vendor speed and price comparisons
are reasons to investigate, not evidence that this workflow will improve.

My expectation is that Jev's value here would be repeated semantic discrimination: helping an
agent tell an event from a mention and a supported extraction from an unsupported one. Anam's
trust still comes from preserved evidence, explicit provenance, deterministic mechanics, and
review of authored interpretations.
