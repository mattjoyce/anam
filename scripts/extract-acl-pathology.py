#!/usr/bin/env python3
"""Extract lab results from an Australian Clinical Labs report PDF into a records file.

Usage:
  extract-acl-pathology.py --out-dir DIR [--history-labs N,N] [--known-times N=HH:MM,...] REPORT.pdf...

For each PDF this runs `pdftotext -layout`, reads the result table, and writes
<out-dir>/<stem>.<sha12>.yaml holding one `lab.result` record per analyte per draw and
one `lab.panel` record for the report's own draw (docs/SELF-LOG.md §5, card 25).

A report prints earlier draws as history columns. Only the report's own draw and the
draws named in --history-labs are emitted, so that a draw with a report of its own is
not also re-admitted from every later report that quotes it. The records worker refuses
one id from two files, so an overlap is caught at ingest rather than ping-ponging.

History columns rarely print a collection time; --known-times supplies the time for a
lab number (one draw is one collection event), and a draw with no known time is
written at 00:00 with `time_unknown: true`.

Deterministic: the same PDF yields the same records, so a re-run is a no-op at the gate.
"""
import argparse
import hashlib
import re
import subprocess
import sys
from pathlib import Path

import yaml

METHOD = "scripts/extract-acl-pathology.py"
VERSION = 1

TERMINATORS = re.compile(
    r"^\s*(This request has other tests|All tests on this request|Glucose Reference Ranges|"
    r"Recommended targets|Specimen Legend|Pathology Report|Lipid ranges and targets|Please note)"
)
SECTION_WORDS = {"BIOCHEMISTRY", "HAEMATOLOGY", "ENDOCRINOLOGY", "CARDIOLOGY", "MICROBIOLOGY"}
VALUE_RE = re.compile(r"(?P<flag>\*\s*)?(?P<cmp>[<>]\s*)?(?P<num>\d+(?:\.\d+)?)(?![\d/])")
RANGE_RE = re.compile(r"\(([^)]*)\)")


def slug(s: str) -> str:
    s = re.sub(r"[^A-Za-z0-9]+", "-", s.strip().lower())
    return s.strip("-")


def norm_space(s: str) -> str:
    return re.sub(r"\s+", " ", s).strip()


def dmy(d: str) -> str:
    """11/09/23 or 30/01/2026 -> 2023-09-11."""
    dd, mm, yy = d.split("/")
    if len(yy) == 2:
        yy = "20" + yy
    return f"{yy}-{mm}-{dd}"


def header_field(text: str, label: str):
    m = re.search(label + r":\s+(\S+(?: \d\d:\d\d(?::\d\d)?)?)", text)
    return m.group(1) if m else None


def parse_report(text: str):
    lines = [ln.rstrip() for ln in text.splitlines()]
    hdr = {
        "lab_ref": header_field(text, "Lab REF"),
        "referred": header_field(text, "REFERRED"),
        "collected": header_field(text, "COLLECTED"),
        "reported": header_field(text, "REPORTED"),
    }
    m = re.match(r"\d\d-(\d+)-", hdr["lab_ref"] or "")
    if not m:
        raise SystemExit(f"no Lab REF found; is this an ACL report?")
    current_lab = m.group(1)

    # Find the table header.
    col_mode = None
    for i, ln in enumerate(lines):
        if re.match(r"^\s*Date:\s+\d\d/\d\d/\d\d", ln):
            col_mode, start = "columns", i
            break
        if re.match(r"^\s*Date\s+Time\s+Lab#", ln):
            col_mode, start = "rows", i
            break
    if col_mode is None:
        raise SystemExit("no result table found")

    # Panel line: last non-empty line above the header that is not a section word.
    panel, specimen = None, None
    for j in range(start - 1, -1, -1):
        s = lines[j].strip()
        if not s or s.startswith("PATIENT:") or s in SECTION_WORDS:
            if s in SECTION_WORDS and panel is None:
                continue
            if panel is not None:
                break
            continue
        if panel is None:
            sm = re.search(r"SPECIMEN:\s*(.+)$", s)
            if sm:
                specimen = sm.group(1).strip()
                s = s[: sm.start()].strip()
            panel = s
            break
    if panel is None:
        panel = "UNKNOWN"
    # Haematology prints the section word as the panel line.
    if panel in SECTION_WORDS and specimen is None:
        sm = re.search(r"SPECIMEN:\s*(.+)$", lines[start - 1])
        if sm:
            specimen = sm.group(1).strip()

    results, comment, fasting = [], None, None
    if col_mode == "columns":
        results, end, fasting = parse_columns(lines, start, hdr)
    else:
        results, end = parse_rows(lines, start)
        fasting = any(r.get("fasting") for r in results) or None

    comment = parse_comment(lines, end, current_lab)
    return hdr, current_lab, panel, specimen, results, comment, fasting


def positions(ln: str, pattern: str):
    return [(m.group(1), m.end(1)) for m in re.finditer(pattern, ln)]


def parse_columns(lines, start, hdr):
    dates = positions(lines[start], r"(\d\d/\d\d/\d\d)")
    times, labs = [], []
    i = start + 1
    while i < len(lines) and not lines[i].strip().startswith("---"):
        if re.match(r"^\s*(Coll\. )?Time:", lines[i]):
            times = positions(lines[i], r"(\d\d:\d\d)")
        elif re.match(r"^\s*(Lab Number|Request):", lines[i]):
            labs = positions(lines[i], r"#?(\d{6,})")
        i += 1
    i += 1  # past the dashes
    if len(labs) != len(dates):
        raise SystemExit(f"header has {len(dates)} dates but {len(labs)} lab numbers")
    cols = []
    for k, (d, _) in enumerate(dates):
        t = times[k][0] if k < len(times) else None
        cols.append({"date": dmy(d), "time": t, "lab": labs[k][0], "end": labs[k][1]})

    results, fasting = [], None
    while i < len(lines):
        ln = lines[i]
        s = ln.strip()
        if not s:
            i += 1
            continue
        if TERMINATORS.match(ln) or re.match(r"^\s*#?\d{6,}\s*:?\s+[A-Za-z]", ln):
            break
        if s.lower().startswith("fasting status"):
            fasting = "Fasting" in s
            i += 1
            continue
        row = parse_row(ln, cols)
        if row is None:
            break
        results.extend(row)
        i += 1
    return results, i, fasting


def parse_row(ln: str, cols):
    rng = RANGE_RE.search(ln)
    body = ln[: rng.start()] if rng else ln
    # The analyte name runs to the first gap of two or more spaces: names carry digits
    # (Free T4, Free T3), and only the column gap tells a name from a value.
    nm = re.match(r"^\s*(\S.*?\S|\S)\s{2,}(.*)$", body)
    if nm is None:
        return None
    name, rest = norm_space(nm.group(1).replace("*", "")), nm.group(2)
    if not name or name[0].isdigit():
        return None
    offset = nm.start(2)
    vals = [(m, offset + m.end("num")) for m in VALUE_RE.finditer(rest)]
    if not vals:
        return None
    tail = ln[rng.end():] if rng else rest[vals[-1][0].end():]
    unit = norm_space(tail).replace("x10 ^", "x10^").replace("^9 /L", "^9/L").replace("^12 /L", "^12/L") or None
    range_s = norm_space(rng.group(1)) if rng else None

    if len(vals) == len(cols):
        assigned = list(zip(cols, vals))
    else:
        assigned = []
        for m, end in vals:
            col = min(cols, key=lambda c: abs(c["end"] - end))
            assigned.append((col, (m, end)))
    out = []
    for col, (m, _) in assigned:
        value = (m.group("cmp") or "").strip() + m.group("num")
        value = value.replace("<", "< ").replace(">", "> ").replace("  ", " ") if m.group("cmp") else value
        rec = {
            "analyte": name, "value": value, "unit": unit, "range": range_s,
            "lab": col["lab"], "date": col["date"], "time": col["time"],
        }
        if m.group("cmp") is None:
            rec["value_num"] = float(m.group("num"))
        if m.group("flag"):
            rec["flagged"] = True
        out.append(rec)
    return out


def parse_rows(lines, start):
    unit = None
    i = start + 1
    while i < len(lines) and not lines[i].strip().startswith("---"):
        um = re.search(r"\((\w+/\w+)\)", lines[i])
        if um:
            unit = um.group(1)
        i += 1
    i += 1
    results = []
    while i < len(lines):
        m = re.match(r"^\s*(\d\d/\d\d/\d\d)\s+(\d\d:\d\d)\s+(\d{6,})\s+(\S+)\s+(\S+)\s+(\*\s*)?([<>]?\s*\d+(?:\.\d+)?)\s*$", lines[i])
        if not m:
            break
        rec = {
            "analyte": "Glucose", "value": m.group(7), "unit": unit, "range": None,
            "lab": m.group(3), "date": dmy(m.group(1)), "time": m.group(2),
            "collection_type": m.group(4), "specimen": m.group(5),
            "fasting": m.group(4).lower() == "fasting",
        }
        if re.fullmatch(r"\d+(?:\.\d+)?", rec["value"]):
            rec["value_num"] = float(rec["value"])
        if m.group(6):
            rec["flagged"] = True
        results.append(rec)
        i += 1
    return results, i


def parse_comment(lines, i, current_lab):
    while i < len(lines) and not lines[i].strip():
        i += 1
    if i >= len(lines):
        return None
    m = re.match(r"^\s*#?" + current_lab + r"\s*:?\s+(.*)$", lines[i])
    if not m:
        return None
    parts = [m.group(1).strip()]
    i += 1
    while i < len(lines) and lines[i].strip() and not TERMINATORS.match(lines[i]):
        parts.append(lines[i].strip())
        i += 1
    return norm_space(" ".join(parts))


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--out-dir", required=True)
    ap.add_argument("--history-labs", default="", help="comma-separated lab numbers whose history columns to emit")
    ap.add_argument("--known-times", default="", help="lab=HH:MM,... collection times for draws that print none")
    ap.add_argument("pdfs", nargs="+")
    args = ap.parse_args()
    history = {s for s in args.history_labs.split(",") if s}
    known = dict(kv.split("=") for kv in args.known_times.split(",") if kv)
    out_dir = Path(args.out_dir)
    out_dir.mkdir(parents=True, exist_ok=True)

    for pdf in args.pdfs:
        p = Path(pdf)
        sha = hashlib.sha256(p.read_bytes()).hexdigest()
        text = subprocess.run(["pdftotext", "-layout", str(p), "-"], check=True, capture_output=True, text=True).stdout
        hdr, lab, panel, specimen, results, comment, fasting = parse_report(text)
        if hdr["collected"]:
            cd, ct = hdr["collected"].split(" ")
            known.setdefault(lab, ct[:5])

        records = []
        for r in results:
            if r["lab"] != lab and r["lab"] not in history:
                continue
            t = r["time"] or known.get(r["lab"])
            sp = r.get("specimen") or specimen
            # Urine and serum both report albumin and creatinine; the specimen is part of
            # what the analyte is, so a non-blood specimen prefixes the id.
            prefix = f"{slug(sp)}-" if sp and slug(sp) not in ("serum", "whole-blood", "plasma") else ""
            rec = {
                "type": "lab.result",
                "id": f"acl:{r['lab']}/{prefix}{slug(r['analyte'])}",
                "collected": f"{r['date']} {t or '00:00'}",
                "lab_number": r["lab"],
                "lab": "Australian Clinical Labs",
                "panel": panel,
                "analyte": r["analyte"],
                "value": r["value"],
            }
            if t is None:
                rec["time_unknown"] = True
            for k in ("value_num", "unit", "range", "flagged", "collection_type"):
                if r.get(k) is not None:
                    rec[k] = r[k]
            if sp:
                rec["specimen"] = sp
            if fasting is not None or r.get("fasting") is not None:
                rec["fasting"] = bool(r.get("fasting", fasting))
            records.append(rec)

        cd = dmy(hdr["collected"].split(" ")[0]) if hdr["collected"] else None
        panel_rec = {
            "type": "lab.panel",
            "id": f"acl:{lab}/panel/{slug(panel)}",
            "collected": f"{cd} {known.get(lab, '00:00')}",
            "lab_number": lab,
            "lab": "Australian Clinical Labs",
            "panel": panel,
            "lab_ref": hdr["lab_ref"],
        }
        if specimen:
            panel_rec["specimen"] = specimen
        if hdr["referred"]:
            panel_rec["requested"] = dmy(hdr["referred"])
        if hdr["reported"]:
            rd, rt = hdr["reported"].split(" ")
            panel_rec["reported"] = f"{dmy(rd)} {rt[:5]}"
        if comment:
            panel_rec["comment"] = comment
        if fasting is not None:
            panel_rec["fasting"] = bool(fasting)
        records.append(panel_rec)

        doc = {
            "document": {"name": p.name, "sha256": sha},
            "extracted": {"method": METHOD, "version": VERSION},
            "records": records,
        }
        out = out_dir / f"{p.stem}.{sha[:12]}.yaml"
        out.write_text(yaml.safe_dump(doc, sort_keys=False, allow_unicode=True, width=100))
        print(f"{out.name}: {len(records) - 1} results + 1 panel ({panel}, lab {lab})")


if __name__ == "__main__":
    main()
