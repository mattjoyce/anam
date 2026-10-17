# Anam — History

Where Anam came from. You do not need any of it to read, run or extend Anam; the other documents
stand alone.

## Arch

Anam began as a re-instantiation of **Arch**, a design for a person-centric, evidence-backed clinical
context layer, published at [mattjoyce/Arch](https://github.com/mattjoyce/Arch) as a PRFAQ and
ADRs. Arch was a design discussion and never an implementation.

Arch scoped every read to an Episode, a bounded span such as a hospital stay, and made the Person a
thin identity index. Its reasoning was that a person is unbounded, a life has no edges, and
"longitudinal is system-of-record gravity". Anam makes the opposite choice, because a personal
record wants to be longitudinal: the Person is the substrate and the whole-person timeline is the
default read. The cost Arch could not pay across a jurisdiction is affordable at the scale of one life.

The rest carried over: the Facet, the append-only ledger, the two clocks, the admission gate, mint,
fold and projections. Early Anam documents cited Arch's ADRs by number (0030 on tombstones, 0031 on
the datom shape, 0032 on checkpoint plus replay, 0033 on per-span minting, 0035 on the Episode as
the unit). The journal still does. Where Anam's own documents are silent, they are silent, and
Arch no longer fills the gap.

Two early mistakes are worth knowing because the journal mentions them. Anam first reasoned that with
no required span the mint unit must be the whole timeline, which conflates the read default with the
mint unit; it was retracted. And it inherited Arch's sentence that a tombstone is a status-bearing
artifact version, which cannot be true when status is folded out of the artifact.

## Senechal, ductile and the health ETL

Senechal was one system that fetched, stored, summarised and answered. It was cleaved into
[ductile](https://github.com/mattjoyce/ductile), which decides when to do things, and a health ETL,
which decided what was measured. Anam succeeds the ETL as the record of what was learned and when,
and sources reach it through the importers in
[anam-importers](https://github.com/mattjoyce/anam-importers). Every drift check in the importer
design guards against one of those two roles growing back into Anam.
