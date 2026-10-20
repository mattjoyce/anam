#!/usr/bin/env python3
"""The Soufflé differential check (card 173): the bundled rules and the card's two
questions, run through Anam's own evaluator and through Soufflé over the same facts,
must answer the same tuples.

Anam's evaluator is a few hundred lines of Go nobody else has run. The equality test
holds current/2 to the fold; this holds the engine's general machinery (joins,
stratified negation, comparisons, the fixpoint) to an independent implementation. It
is an oracle run by hand until the engine has earned trust, not a standing part of
the gate: Soufflé is a binary the gate must not depend on, and the receipts never
name it.

The facts are exported from a served value with `anam query run --dry-run --json`
(entry/7, attr/3, closes/1), written as Soufflé .facts files, and the instants both
sides ask about are every Nth distinct fact-time. The Soufflé program below is the
bundled rule set and the questions translated by hand; a change to anam.dl is a
change here. Numbers are compared as numbers, since the two engines print them
differently; tabs and newlines inside payload strings are spaced on both sides, since
a .facts file cannot carry them.

    scripts/souffle-diff.py --projection everything --every 10
    scripts/souffle-diff.py --ledger copy/ledger.db --person matt --projection bp

Needs `anam` (or --anam) and `souffle` (or --souffle) on the PATH. Exit 1 on any
disagreement. Python, stdlib only, as AGENTS.md allows for a one-off script.
"""

import argparse
import json
import os
import shutil
import subprocess
import sys
import tempfile

# The bundled rule set, translated. Soufflé compares symbols lexicographically, as
# Anam does, so fact-times and ids compare as they do there. attr/3 is split by kind:
# attr holds every leaf as its text, attrf the numeric ones as floats, since Soufflé
# relations are typed and Anam's are not.
BUNDLED = """
.decl entry(e:symbol, t:symbol, ft:symbol, s:symbol, st:symbol, src:symbol, v:number)
.input entry
.decl attr(e:symbol, p:symbol, v:symbol)
.input attr
.decl attrf(e:symbol, p:symbol, v:float)
.input attrf
.decl closes(t:symbol)
.input closes
.decl instant(at:symbol)
.input instant

.decl before(e:symbol, at:symbol)
before(E, At) :- entry(E, _, T, _, _, _, _), instant(At), T <= At.
.decl later(e:symbol, f:symbol)
later(E, F) :- entry(E, _, T, S, _, _, _), entry(F, _, U, S, _, _, _), S != "", T < U.
later(E, F) :- entry(E, _, T, S, _, _, _), entry(F, _, T, S, _, _, _), S != "", E < F.
.decl displaced(e:symbol, at:symbol)
displaced(E, At) :- later(E, F), before(F, At).
.decl latest(e:symbol, at:symbol)
latest(E, At) :- before(E, At), entry(E, _, _, S, _, _, _), S != "", !displaced(E, At).
.decl closed(e:symbol, at:symbol)
closed(E, At) :- latest(E, At), entry(E, Type, _, _, _, _, _), closes(Type).
.decl current(e:symbol, at:symbol)
current(E, At) :- before(E, At), !displaced(E, At), !closed(E, At).
.decl edge_to(e:symbol, f:symbol)
edge_to(E, F) :- entry(E, "record.edge", _, _, _, _, _), attr(E, P, F), match("/to/.*", P).
edge_to(E, F) :- entry(E, "record.edge", _, _, _, _, _), attr(E, P, F), match("/about/.*", P).
.decl edge_from(e:symbol, f:symbol)
edge_from(E, F) :- entry(E, "record.edge", _, _, _, _, _), attr(E, P, F), match("/from/.*", P).
.decl linked(e:symbol, f:symbol)
linked(E, F) :- edge_to(E, F).
linked(E, F) :- edge_from(E, F).
.output current
.output displaced
.output linked
"""

# The questions of 2026-10-17, and the selection by payload §8 admits, each as Anam
# writes it and as Soufflé does. The Anam text is prefixed with the instant facts.
QUESTIONS = [
    {
        "relation": "regimen",
        "anam": """
regimen_type("health.medication.started"). regimen_type("health.medication.dose-changed").
regimen(At, Drug) :- entry(_, "health.blood-pressure", At, _, _, _, _), current(E, At),
                     entry(E, T, _, _, _, _, _), regimen_type(T), attr(E, "/drug", Drug).
""",
        "souffle": """
.decl regimen_type(t:symbol)
regimen_type("health.medication.started"). regimen_type("health.medication.dose-changed").
.decl regimen(at:symbol, drug:symbol)
regimen(At, Drug) :- entry(_, "health.blood-pressure", At, _, _, _, _), current(E, At),
                     entry(E, T, _, _, _, _, _), regimen_type(T), attr(E, "/drug", Drug).
.output regimen
""",
    },
    {
        "relation": "without",
        "anam": """
regimen_type("health.medication.started"). regimen_type("health.medication.dose-changed").
on_d3k2(At) :- current(E, At), entry(E, T, _, _, _, _, _), regimen_type(T), attr(E, "/drug", "vitamin d3 + k2").
without(At) :- entry(_, "health.blood-pressure", At, _, _, _, _),
               At >= "2026-06-01T00:00:00.000000000Z", At < "2026-09-01T00:00:00.000000000Z", not on_d3k2(At).
""",
        "souffle": """
.decl on_d3k2(at:symbol)
on_d3k2(At) :- current(E, At), entry(E, T, _, _, _, _, _), regimen_type(T), attr(E, "/drug", "vitamin d3 + k2").
.decl without(at:symbol)
without(At) :- entry(_, "health.blood-pressure", At, _, _, _, _),
               At >= "2026-06-01T00:00:00.000000000Z", At < "2026-09-01T00:00:00.000000000Z", !on_d3k2(At).
.output without
""",
    },
    {
        "relation": "high",
        "anam": """
high(At, S) :- entry(E, "health.blood-pressure", At, _, _, _, _), attr(E, "/systolic_blood_pressure/value", S), S > 140.
""",
        "souffle": """
.decl high(at:symbol, s:float)
high(At, S) :- entry(E, "health.blood-pressure", At, _, _, _, _), attrf(E, "/systolic_blood_pressure/value", S), S > 140.0.
.output high
""",
    },
]

# The exports: each base relation as a derived one, so `anam query run` answers it.
EXPORTS = {
    "entry": "e(E, T, F, S, St, Src, V) :- entry(E, T, F, S, St, Src, V).",
    "attr": "a(E, P, V) :- attr(E, P, V).",
    "closes": "c(T) :- closes(T).",
}


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--anam", default="anam", help="the anam binary")
    ap.add_argument("--souffle", default="souffle", help="the souffle binary")
    ap.add_argument("--ledger", help="a ledger file to run over (default: the configured one)")
    ap.add_argument("--person", help="the person the ledger is pinned to (default: the configured one)")
    ap.add_argument("--projection", required=True, help="the log-shaped projection both engines run through")
    ap.add_argument("--every", type=int, default=1, help="ask about every Nth distinct fact-time")
    ap.add_argument("--workdir", help="where to write the facts and programs (default: a temp dir, removed unless --keep)")
    ap.add_argument("--keep", action="store_true", help="keep the work directory")
    ap.add_argument("--timeout", default="30m", help="the read timeout each anam run gets (ANAM_TIMEOUT), unless the environment sets one")
    args = ap.parse_args()
    if args.every < 1:
        sys.exit("--every must be at least 1")

    env = dict(os.environ)
    if args.ledger:
        env["ANAM_LEDGER_PATH"] = os.path.abspath(args.ledger)
    if args.person:
        env["ANAM_PERSON_ID"] = args.person
    env.pop("ANAM_SERVER", None)  # the script runs over files; a daemon would own them
    # A whole-record current/2 over many instants outruns the CLI's default read
    # timeout; this is a one-off run, so the bound is raised unless the caller set it.
    env.setdefault("ANAM_TIMEOUT", args.timeout)

    work = args.workdir or tempfile.mkdtemp(prefix="souffle-diff-")
    os.makedirs(work, exist_ok=True)
    try:
        ok = run(args, env, work)
    finally:
        if not args.keep and not args.workdir:
            shutil.rmtree(work, ignore_errors=True)
    sys.exit(0 if ok else 1)


def anam_query(args, env, text, relation):
    """Runs one program through anam and returns the relation's tuples as lists."""
    cmd = [args.anam, "query", "run", "--projection", args.projection, "--relation", relation, "--dry-run", "--json", "-e", text]
    res = subprocess.run(cmd, env=env, capture_output=True, text=True, check=False)
    if res.returncode != 0:
        sys.exit(f"anam query run ({relation}) failed:\n{res.stderr}")
    return json.loads(res.stdout)["answer"]["tuples"]


def clean(s):
    """A symbol as a .facts file can carry it: no tab or newline."""
    return str(s).replace("\t", " ").replace("\n", " ").replace("\r", " ")


def cell_text(v):
    """A tuple cell as Anam prints it: a bool as true/false, a number as its lexeme."""
    if isinstance(v, bool):
        return "true" if v else "false"
    if isinstance(v, float) and v.is_integer():
        return str(int(v))
    return clean(v)


def write_facts(work, name, rows):
    with open(os.path.join(work, name + ".facts"), "w", encoding="utf-8") as f:
        for row in rows:
            f.write("\t".join(row) + "\n")


def export(args, env, work):
    """Exports the base relations and the instants; returns the instant list."""
    entries = anam_query(args, env, EXPORTS["entry"], "e")
    attrs = anam_query(args, env, EXPORTS["attr"], "a")
    closes = anam_query(args, env, EXPORTS["closes"], "c")
    write_facts(work, "entry", ([cell_text(c) for c in row] for row in entries))
    write_facts(work, "attr", ([clean(row[0]), clean(row[1]), cell_text(row[2])] for row in attrs))
    write_facts(work, "attrf", ([clean(row[0]), clean(row[1]), repr(float(row[2]))]
                               for row in attrs if isinstance(row[2], (int, float)) and not isinstance(row[2], bool)))
    write_facts(work, "closes", ([clean(row[0])] for row in closes))
    times = sorted({row[2] for row in entries})
    if not times:
        sys.exit(f"the projection {args.projection} serves no entry, so there is no instant to ask about; "
                 "pick one that serves facets (anam projection list)")
    instants = times[:: args.every]
    write_facts(work, "instant", ([t] for t in instants))
    print(f"exported {len(entries)} entries, {len(attrs)} attrs, {len(closes)} closing types; "
          f"{len(instants)} of {len(times)} fact-times as instants (every {args.every})")
    return instants


def run_souffle(args, work):
    program = BUNDLED + "".join(q["souffle"] for q in QUESTIONS)
    path = os.path.join(work, "program.dl")
    with open(path, "w", encoding="utf-8") as f:
        f.write(program)
    out = os.path.join(work, "out")
    os.makedirs(out, exist_ok=True)
    res = subprocess.run([args.souffle, "-F", work, "-D", out, path], capture_output=True, text=True, check=False)
    if res.returncode != 0:
        sys.exit(f"souffle failed:\n{res.stderr}")
    return out


def read_souffle(out, relation):
    rows = []
    with open(os.path.join(out, relation + ".csv"), encoding="utf-8") as f:
        for line in f:
            line = line.rstrip("\n")
            if line:
                rows.append(line.split("\t"))
    return rows


def normalise(row):
    """A tuple as a comparable key: numbers as floats, everything else as clean text."""
    key = []
    for c in row:
        if isinstance(c, bool):
            key.append("true" if c else "false")
            continue
        try:
            key.append(f"{float(c):.9g}")
        except (TypeError, ValueError):
            key.append(clean(c))
    return tuple(key)


def compare(name, ours, theirs):
    a, b = {normalise(r) for r in ours}, {normalise(r) for r in theirs}
    missing, extra = sorted(b - a), sorted(a - b)
    verdict = "agree" if not missing and not extra else "DISAGREE"
    print(f"{verdict:9s} {name:10s} anam {len(a):7d}  souffle {len(b):7d}")
    for row in missing[:5]:
        print(f"          souffle has, anam lacks: {row}")
    for row in extra[:5]:
        print(f"          anam has, souffle lacks:  {row}")
    return not missing and not extra


def run(args, env, work):
    instants = export(args, env, work)
    out = run_souffle(args, work)
    facts = "".join(f'instant("{t}").\n' for t in instants)
    ok = True
    for relation in ("current", "displaced", "linked"):
        ok &= compare(relation, anam_query(args, env, facts, relation), read_souffle(out, relation))
    for q in QUESTIONS:
        ok &= compare(q["relation"], anam_query(args, env, facts + q["anam"], q["relation"]), read_souffle(out, q["relation"]))
    print("OK   the two engines agree" if ok else "FAIL the two engines disagree")
    return ok


if __name__ == "__main__":
    main()
