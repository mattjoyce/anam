#!/usr/bin/env python3
"""Fitbit "Weight Logs.xlsx" (Google Takeout Fitbit export) -> a records file of health.weight.

Usage: extract-fitbit-weight.py --out-dir DIR "Weight Logs.xlsx"

The workbook has no header row: column A is the log time ("February 12, 2015 at 06:33AM"),
B the weight, C the unit, D the BMI Fitbit computed. Read with the standard library, since
the sheet is simple and openpyxl is not installed. Deterministic; ids are fitbit:<ISO time>.
"""
import argparse, hashlib, re, zipfile
from datetime import datetime
from pathlib import Path
import yaml

def cells(z):
    ss = z.read("xl/sharedStrings.xml").decode()
    strings = [re.sub(r"<[^>]+>", "", m) for m in re.findall(r"<si>(.*?)</si>", ss, re.S)]
    sheet = z.read("xl/worksheets/sheet1.xml").decode()
    for row in re.findall(r"<row[^>]*>(.*?)</row>", sheet, re.S):
        out = {}
        for col, attrs, v in re.findall(r'<c r="([A-Z]+)\d+"([^>]*)>(?:<v>([^<]*)</v>)?', row):
            out[col] = strings[int(v)] if 't="s"' in attrs else v
        yield out

def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--out-dir", required=True)
    ap.add_argument("xlsx")
    a = ap.parse_args()
    p = Path(a.xlsx); sha = hashlib.sha256(p.read_bytes()).hexdigest()
    records, seen = [], {}
    with zipfile.ZipFile(p) as z:
        for c in cells(z):
            when = datetime.strptime(c["A"], "%B %d, %Y at %I:%M%p")
            unit = c.get("C", "")
            if unit != "kilograms":
                raise SystemExit(f"unexpected unit {unit!r} at {c['A']}")
            # Two logs in one minute happen (a re-weigh); the second gets an ordinal so
            # neither is lost and the id stays deterministic in file order.
            key = f"{when:%Y-%m-%dT%H:%M}"
            seen[key] = seen.get(key, 0) + 1
            rid = key if seen[key] == 1 else f"{key}:{seen[key]}"
            rec = {"type": "health.weight", "id": f"fitbit:{rid}", "date": f"{when:%Y-%m-%d %H:%M}",
                   "kg": float(c["B"]), "logged_as": c["A"], "source_note": "Fitbit weight log (Google Takeout export)"}
            if c.get("D"):
                rec["bmi"] = float(c["D"])
            records.append(rec)
    doc = {"document": {"name": p.name, "sha256": sha}, "extracted": {"method": "scripts/extract-fitbit-weight.py", "version": 1}, "records": records}
    out = Path(a.out_dir) / f"fitbit-weight-logs.{sha[:12]}.yaml"
    out.write_text(yaml.safe_dump(doc, sort_keys=False, allow_unicode=True, width=100))
    print(f"{out.name}: {len(records)} weights, {records[0]['date']} to {records[-1]['date']}")

if __name__ == "__main__":
    main()
