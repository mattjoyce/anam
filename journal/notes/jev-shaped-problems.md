# Where Jev earns a place in Anam

2026-10-10 · Card 81 · assessment, not an implementation decision.

**Jev fits the small semantic decisions around the record: what a source asserts, whether it
supports a candidate claim, and which evidence deserves a reader's attention. Anam's record
mechanics remain exact. My first experiment would check claim support against source passages,
across several kinds of record, before committing to a medication classifier.**

“Legitimate” below means the problem has the right shape and a real Anam use. It does not mean
Jev has demonstrated adequate accuracy or saved anyone work. No inference calls or benchmark
were run for this card. Live documentation was checked on 2026-10-10; local inspection covered
Anam at `9e92afc`, `anam-importers` at `f49b0f7`, the open board, and source inventories.

## The short definition

A Jev-shaped problem is a **bounded semantic question over supplied evidence, with a declared
answer space and a useful, reversible consequence**. A reader can label its answer from that
state. Code cannot settle it merely by applying the source's explicit rules. Its mistakes can
remain mistakes of interpretation without becoming changes to the record.

Jev returns typed judgments and probabilities, not an explanation, an investigation, or newly
written text. Its current input is text, including structured JSON; it cannot inspect a scan.
These are the vendor's documented capabilities, not measured Anam performance.
[System One](https://docs.typesafe.ai/concepts/system-one)

| Primitive | What comes back | Appropriate question |
|---|---|---|
| Choice | One option, distribution over options, confidence | Which supplied meaning or candidate fits? Include `unknown`/`none` where needed. |
| Noul | Probability of yes; no separate confidence | Does this specific condition hold? Separate questions allow several labels to apply. |
| Score | Probability-weighted position on described ordered levels, level probabilities, confidence | How directly does this passage address this question? A score is not a measured quantity. |

Sources: [Choice](https://docs.typesafe.ai/primitives/choice),
[Noul](https://docs.typesafe.ai/primitives/noul), [Score](https://docs.typesafe.ai/primitives/score).
Confidence describes the distribution's concentration. It is neither observed accuracy on this
corpus nor permission to act. A Noul near one-half expresses uncertainty between yes and no,
not “half present.” [Confidence](https://docs.typesafe.ai/confidence)

Supply the evidence, target, relevant relationships, and complete question meaning. Question
IDs are bookkeeping, not instructions. Questions sharing a state cannot consume one another's
answers: fetch new evidence or construct dependent candidates in a later step. Operationally
independent evaluation does not imply statistically independent errors; multiplying several
answers does not produce a justified probability that an extraction is correct.
[State](https://docs.typesafe.ai/concepts/state)

Use this sorting test:

1. **Does an explicit contract settle it? Code.** Parse, validate, compare, calculate, join,
   select by metadata, resolve time with a known zone, or replay events.
2. **Could someone answer from this bounded evidence using the supplied labels? Jev is a
   candidate**, if the remaining work is semantic and the result actually helps a workflow.
3. **Must someone find evidence, decide what to ask, reconcile an open history, or explain a
   conclusion? A reasoning model or person**, using tools and deterministic calculations.
4. **Is evidence absent or the domain decision unresolved? Obtain evidence or decide the
   contract.** Neither model is a substitute for the missing 2023 screenshot or an agreed schema.

A yes/no wrapper does not make “Did my medication cause this trend?” a bounded judgment.
Conversely, selecting a source span or a read operation can be legitimate; Jev is not limited
to tagging documents. The live cookbooks demonstrate
[candidate selection](https://docs.typesafe.ai/cookbooks/pre_parsed_value_extraction_cookbook),
[function selection](https://docs.typesafe.ai/cookbooks/function_calling), and
[evidence reranking](https://docs.typesafe.ai/cookbooks/rerank_typesafe).

## What the project actually needs

The [PR/FAQ](../../docs/PRFAQ.md) promises a whole-life account with evidence and honest knowledge
time. [API](../../docs/API.md) §§1, 4–6 expands that into discovery, conversation, and reliable
“log this.” [Self-log](../../docs/SELF-LOG.md) separates recorded regimen, reported adherence,
and derived expectations. Those goals put semantic work on both sides of the ledger: helping
someone prepare a source assertion, and helping a consumer understand evidence already served.

The [importer contract](../../docs/IMPORTERS.md) has a different job: preserve observations,
shape known formats, and submit through the gate. Existing Withings shaping, BP CSV capture,
ACL parsing, and the Strong/Fitbit/Technogym scripts already have deterministic rules. A new
vendor format calls for engineering once, not a model interpreting every row forever.

Several documents describe target behaviour. `IMPORTERS.md` still opens with “No importer
exists yet,” but the sibling repo contains Withings and `anam_submit`; cards 54, 75 and 76
track the remaining cutover. `API.md` explicitly distinguishes its proposed operations from
the interim daemon. This assessment uses code and current cards to establish availability.

Two present limits matter more than model selection:

- [`gate.build`](../../internal/gate/commit.go) sets captured confidence to 1 and supplies no
  derivation. [`records`](../../internal/capture/records/records.go) reads `document` and
  `extracted` wrappers, but they do not become derivation metadata in its envelopes.
  Merely writing “Jev” in a records file does **not** create a contained proposed finding.
- [API §5–6](../../docs/API.md) still proposes version-bound evidence resolution and the
  source-authoring/submission lifecycle. The [inbox runbook](../../docs/INBOX.md) explicitly
  relies on manual review today. These gaps cannot be filled by a confident answer.

A read-only inventory found 20 records files, including 273 `lab.result` rows across ten files,
33 panels, travel records, and four regimen-change rows in one file. Counts describe source
files, not independently checked truth or live ledger totals. The original BP export contains
234 rows, 17 nonempty comments, only six distinct comment texts, and one explicit missed-medication
flag. This is a small personal corpus: setup, labelling and review time can dominate inference cost.
No private passages are reproduced here or were sent to TypeSafe.

## Candidate contracts and verdicts

### 1. Does the cited evidence support this statement? — legitimate; first experiment

**State:** one candidate statement, the exact cited passage with surrounding context, its source
hash/page or facet version, and relevant provenance. For a lab candidate include the header,
row and column labels; for a travel statement include whether the text is a booking or a report
of completed travel. A source-backed statement remains different from proof of the event.

**Question / answer / primitive:** “Does this passage support this specific statement, preserving
who, event meaning and certainty?” Choice: `supported`, `contradicted`, `not_established`.
Use separate questions for independently useful dimensions such as event assertion, subject,
and date role; do not ask one vague “is this record correct?”

**Flow / code:** an offline source-preparation reviewer, or a conversational consumer after a
receipted read and before showing its answer. Code verifies citation existence, source version,
quoted spans and exact arithmetic first. The semantic answer marks a claim for inspection or
revision. It never grants endorsement, publishes a fact, or suppresses source evidence.

**Wrong answer:** a false pass leaves an unsupported claim looking checked; a false failure costs
review time. Show the evidence with the judgment, retain a manual path, and keep numerical and
provenance checks independent. Two models can agree on the same bad OCR. Jev cannot verify a
scan it has not seen. This is particularly relevant to the inbox's documented OCR/date error.

This also serves card 72's eventual conversations: a receipt proves which evidence was shown;
it does not prove that prose accurately describes it. TypeSafe's
[citation cookbook](https://docs.typesafe.ai/cookbooks/citation_check) makes the same useful
split between locating a quote in code and checking its contextual support semantically.

### 2. What is being asserted in this passage? — legitimate, with a narrow target

**State:** a clause and its surrounding passage, named event/medication, source kind and speaker.
**Question / answer / primitive:** Choice for one targeted proposition: `occurred`, `planned_or_recommended`,
`historical_or_current_mention`, `explicitly_negated`, `unclear`. Specialize these labels for the task;
a medication change and a booked journey need different criteria. Multiple assertions require
multiple targets or independent Nouls, not one exclusive label for a whole document.

**Flow / code:** prepare candidates in the document workflow or annotate evidence in a consumer's
reading list. Code keeps the original text and references, offers an interpretation for review,
and leaves dates, stable identity and schema validation to their existing contracts.

**Wrong answer:** a mention becomes a fictitious event. A prescription/dispensing record does not
by itself establish first use or adherence; a booking does not establish travel. An ambiguous
short BP comment must remain ambiguous. Existing structured missed-dose flags need no Jev call.

This extends the earlier medication proposal to the longitudinal goal. It is a good semantic
problem but a weaker first investment: four existing regimen events do not constitute a useful
medication benchmark, and a dictionary may exhaust the six observed BP comment forms.

### 3. Which evidence should I read for this question? — legitimate at the consumer

**State:** a user's question and each candidate passage, with source kind and temporal context,
obtained within an authorized projection. **Question / answer / primitive:** per-item Score with
standalone levels: unrelated; mentions the topic without addressing the question; directly
addresses the question. Keep relevance separate from source reliability or support for a claim.

**Flow / code:** metadata narrowing and retrieval first, semantic ranking second, reasoning and
citation afterward. Code orders a reading list, retains exact references, and keeps the full
served set accessible. Rank both supporting and conflicting evidence. This is a consumer view,
not a change to projection membership, canonical bytes or a receipt.

**Wrong answer:** relevant contrary evidence is buried. Report retrieval scope and limits;
low rank means “not selected for this reading list,” never “absent from the record.” Measure
retrieval recall as well as ranking. A reranker cannot recover candidates the first stage omitted.

This becomes more valuable as mail, notes and calendar sources arrive. It is not required to
answer “show June's BP”: type/time filtering already does that. Semantic payload search is
explicitly a future capability in API §5, not something today's timeline endpoint already offers.

### 4. Which available read or authoring type matches these words? — plausible but unproven

**State:** the current user utterance plus minimal dialogue context, advertised operations,
authorized projection descriptions, and installed Definitions. **Question / answer / primitive:**
Choice among the offered operation/type IDs plus `ambiguous`, `unsupported`, `multiple`.

**Flow / code:** in the conversational client before constructing a typed API request. Code
resolves explicit dates, validates arguments and capability availability, and handles retries.
Jev may suggest “snapshot” or `donation.blood`; it cannot create the missing authoring operation,
authorize a write, invent fields, or choose a medication course from inadequate evidence.

**Wrong answer:** the client retrieves the wrong period or misinterprets a write request. Clarify
unresolved meaning and preserve the utterance; an actual write still requires user intent and
the source-first submission contract. A classification of intent is not authorization.

For card 72, an existing reasoning client already does this interpretation. A second model
must demonstrate a latency/cost advantage without making dialogue worse. No separate router
is justified merely because a function-calling cookbook exists.

### 5. Which printed value has this role? — plausible but unproven

**State:** layout-preserving text, named analyte/field, candidate spans and headers.
**Question / answer / primitive:** Choice over candidate IDs plus `none`/`ambiguous`, e.g. the
collection-date span rather than the reported-date span.

**Flow / code:** after parsing/OCR, before preparing a record. Code enumerates candidates,
copies the selected bytes, parses units/dates and validates. ACL's known columns remain the
existing parser's job; the candidate is for unfamiliar layouts.

**Wrong answer:** a perfectly copied value belongs to the wrong column/person/draw. Candidate
coverage and text fidelity must be checked separately; absent or garbled values go to original
source review. Current Jev is text-only. Card 36's glasses prescription needs a supported
Definition and reliable source reading before a selector is useful.

### 6. Which processing path fits this document? — plausible but unproven

**State:** a page's text/header, known format signatures and available extractor descriptions.
**Question / answer / primitive:** Choice among known extractors, manual reading, unknown/mixed.
**Flow / code:** inbox preparation after deterministic signature matching; code invokes the
candidate parser and validates its complete output. It must not classify a whole mixed scan
from its first page or treat routing as successful extraction.

**Wrong answer:** wasted processing or misparsed content; fail visibly and return to manual
review. With this small corpus and several established scripts, a model router may cost more
maintenance than it saves. Generic document classification alone is not a product need.

### 7. Do these descriptions refer to the same occurrence? — plausible but unproven

**State:** a pair proposed by deterministic time/type/source narrowing, with both descriptions
and provenance. **Question / answer / primitive:** Choice: same occurrence, distinct occurrences,
insufficient evidence. **Flow / code:** consumer review suggestion only; retain both facets and
let an explicit source correction, if warranted, follow its own contract.

**Wrong answer:** collapsing two real observations destroys history. Do not merge, retract,
rewrite keys, or silently remove a reading on similarity. Lab accession numbers, hashes and
same-source identities are exact; use code. Card 80's cross-source BP overlap is mainly a
provenance/identity-policy question, not inherently a language problem. If both inputs contain
only numbers and times, Jev has no special evidence to add. The BP identity journal records
13 timestamps with two genuine readings; temporal proximity is not identity.

### 8. Is this a useful context label for a period? — plausible but unproven

**State:** a note and a proposed explicit label such as a mention of travel, illness, or exercise.
**Question / answer / primitive:** one Noul per label: “Does this note explicitly describe X?”
**Flow / code:** consumer-side exploration; code joins to periods and computes statistics.
Keep multiple labels and uncertainty rather than forcing one story onto a week.

**Wrong answer:** a mislabeled period suggests a spurious association. Do not infer causation,
invent event dates, or call an inferred annotation a source fact. Structured travel/illness
facets already support exact grouping; only previously unstructured text creates a Jev seam.
At today's volume this is an exploratory convenience, not a reason to build a semantic index.

### 9. The attractive non-candidates — no

| Proposed use | Why no; who answers it |
|---|---|
| Regimen at D, historical knowledge at T, expected doses | Fold/series semantics and `regimen.Expected`; adherence itself needs evidence. |
| “Did my treatment cause this change?”; explain my whole history | Investigation, statistical analysis and reasoned interpretation; not a bounded Jev answer. |
| Admission, field validity, timestamps, unit conversion, source identity, hashes | Definition, parser and gate contracts. Unknown format/schema is engineering work. |
| Freshness, missing source rows, receipt validity, why a facet was excluded | Measured operational state and core verdicts; unknown completeness stays unknown. |
| Choose whose Withings measurement this is from its value | Source attribution or explicit resolution. Missing identity cannot be supplied by plausibility. |
| Sensitivity, harm, access, endorsement, which agent may see something | Outside this card's permitted model role. Roles select projections; policy and reviewer authority are not predictions. |
| CPAP/FIT decoding, alert counts, weekly trends, anomaly thresholds | Binary parsing and arithmetic. A reasoning agent may investigate an anomaly afterward. |
| Repair a failed import or fill missing facts by guessing | Failures and absence must remain visible. Confidence cannot repair unavailable evidence. |

The live [Jev 1.13 limitations](https://docs.typesafe.ai/model-jaggedness/jev-1.13) also advise
keeping numeric operations and date comparisons in code, limiting irrelevant context, and
testing option-order sensitivity. These are additional practical cautions, not the reason
Anam's exact contracts belong in code.

## What this means for the open board

Reviewed all 31 open cards, including this one, at the claim-time board state. They fall into
concrete groups:

| Cards | Consequence for this assessment |
|---|---|
| 72, 71, 49, 73 | Honest API discovery, Definition descriptions and evidence access enable useful consumers. Build these contracts before adding semantic routing. README/docs work uses a reasoning/writing agent, not Jev judgments at runtime. |
| 37, 36 | Missing medicines history and unsupported prescription/CPAP types need source evidence and domain/schema decisions. No model can manufacture those prerequisites. |
| 50, 54, 55, 58, 70, 74, 75, 76, 77, 78, 79, 80 | Importer extraction, deployment, numeric representation, retry, retraction and Definition provenance remain deterministic. Only free-text interpretation beside these flows is a candidate. |
| 21, 41, 60, 63, 64, 65 | Checkpoint presentation, density, hash design and read performance need code/design, not probabilistic selection. |
| 30, 32, 33, 40, 47, 67 | Service completion, upgrades, scheduling, merge, backup and retention need operational work. |

The prior [Jev exploration](../../jev-for-anam.md) correctly isolates judgments from the kernel.
Its medication-first recommendation was not evaluated. This review changes the order, broadens
the scope to conversational claim support and discovery, and downgrades document routing and
duplicate detection where explicit formats/keys already settle the question. The main new
lesson is that **source support is a reusable question; “medical truth” is not**.

## Doctrine and Hickey × Armstrong — Before review

**Hickey: preserve the distinctions.** Evidence, interpretation and action policy are separate
values. A judgment about a source is never a world fact. Its retained record can say “this model
answered this question over these bytes”; it cannot silently say the world was as it answered.
A consumer ranking cannot become a payload filter inside `projection.Apply`. A helper selecting
an assertion type cannot become a second source writer hidden in MCP.

**Armstrong: a confident error is still a successful HTTP response.** Timeouts are only one
failure class. Put any future evaluator outside the ledger/gate, with a deadline and bounded
retries. An offline pilot resumes by case ID under its operator; a future scheduled worker is
restarted by Ductile; a conversational request falls back to ordinary evidence and explicit
“unchecked” status. Missing source, malformed answer, rate limiting and semantic disagreement
have distinct outcomes. Source ingestion and ordinary reads must not depend on Jev availability.

**Convergence:** retain exact input references, source/extraction hashes, ledger position where
applicable, model ID, question/criteria version, full answer and consumer-policy version. Treat
source text as evidence, never instructions. Check freshness before using a result against a
changed candidate. Confidence never grants access, admission or endorsement. No use here asks
Jev to assess harm or sensitivity; role-to-projection access remains outside inference. Today's
open consent hook is not an implemented fine-grained authorization system.

**Tension: recomputable is not reproducible.** Disposable annotations may be dropped and recomputed,
but rerunning a hosted model need not reproduce its old answer, even with a pinned model name.
If an answer is relied upon in a published explanation or proposed finding, preserve the exact
judgment as authored evidence and back it up. It must not be hidden in a supposedly replayable
index. This follows [Architecture §8, §11–12](../../docs/ARCHITECTURE.md): derived indexes replay;
authored findings and decisions are retained. The card's “derived and rebuildable” requirement
means no irreplaceable truth lives in an inference cache, not that a fresh call reconstructs
historical testimony.

A future durable finding requires a real proposed/review lifecycle through the Envelope and gate.
Until then the useful containment is an evaluation artifact or visible consumer annotation,
outside the live records source. Neither adding a YAML method label nor passing JSON Schema
validation supplies that lifecycle.

## Ranked next steps and the cheapest honest experiment

1. **Claim support against a cited source:** first experiment, useful during current manual
   extraction and later conversation. It tests whether Jev helps where structural validation stops.
2. **Targeted assertion meaning:** reuse the evidence set if the first result reveals recurring
   event-versus-mention errors. Avoid a medication-only product on four existing event rows.
3. **Evidence ranking:** revisit with card 72 and a demonstrated retrieval problem. Measure it
   against ordinary type/date narrowing and text search, including missed contrary evidence.

The other plausible candidates stay ideas, not integration tickets. None has earned production
use. The first follow-up is an experiment whose valid result can be “keep doing this manually.”

**Pilot protocol, on Anam's real data:**

- Start with 12 source/claim pairs from the existing records and their primary documents, covering
  lab column/date roles, medication event-versus-mention, and planned-versus-completed travel.
  A person labels each before seeing model output. Preserve `not_established` and adjudicate
  disagreement; a generated records file is a candidate, never the ground truth. If original
  evidence cannot be located or the labels cannot be agreed, stop and fix that prerequisite.
- If useful disagreements exist, expand to at most 60 pairs from at least ten independently
  sourced document groups. Target four development groups and six held-out groups; keep the
  same draw, repeated report history, trip and paraphrases in one group. Do not force quotas by
  pretending correlated rows are independent. If the corpus cannot supply this, report a small
  feasibility study, not a validated classifier.
- Include naturally occurring candidates and separately labelled challenge variants: an
  unsupported event, wrong date role, missing qualifier or swapped subject. Keep the two result
  sets separate. Artificial errors test discrimination; they do not estimate production error
  prevalence. Minimize the evidence to what each question needs without removing qualifiers.
- Compare code-only exact checks, Jev after the same checks, and the present reasoning-model
  review after those checks. Pin prompts and resolved model IDs. Use the current documented
  `jev-1.13.0`, not a moving alias. Check a subset under option permutations and irrelevant
  context; retain the original outcome rather than selecting the run that looks best.
- Report a confusion matrix against hand labels, per-class precision/recall, and especially
  **false `supported`** outcomes. For any review threshold, report accuracy among passed claims
  together with coverage and the number deferred. Tune on development groups only. Audit both
  passes and failures. A small or class-empty holdout cannot certify a low error rate.
- Measure complete wall-clock latency per case and per batch (p50/p95), timeouts, retries, billed
  input tokens, total cost, and human seconds reviewing each condition in randomized order.
  Include source preparation and OCR separately so cheap judgment calls do not conceal expensive
  evidence assembly. Failures count in end-to-end results rather than disappearing from the sample.

The live [model page](https://docs.typesafe.ai/models) lists $0.042 per million input tokens,
with output tokens free. **Illustrative budget, not a measurement:** 60 requests averaging 2,000
billed input tokens would cost $0.00504 for one Jev pass; three such passes would cost $0.01512.
Use returned usage and actual billing, not that assumption, for the report. Human labels,
source preparation and comparison-model calls are additional costs. Latency and accuracy on
Anam are **unmeasured**; cookbook results are not substitutes.

Proceed to an integration proposal only if the held-out pilot reveals useful errors beyond exact
checks, a lower review burden than the current workflow, and no observed false-supported result
in the reviewed pilot. That last condition is a pilot rejection rule, not proof of safety or a
production threshold. Any win still needs a larger evaluation and the named lifecycle/evidence
seams. If rules cover the observed cases, keep rules. If only a handful of one-off cases remain,
keep human reading. The question to answer is whether this saves Matt attention while leaving
his record just as inspectable.
