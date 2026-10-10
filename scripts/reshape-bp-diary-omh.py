#!/usr/bin/env python3
"""Reshape a records file's health.blood-pressure rows to Definition v4 (card 83).

v4's payload is the Open mHealth blood-pressure-4.0 body with the source's own fields
beside it. Per row:

- systolic, diastolic: the source's text becomes {"value": <number>, "unit": "mmHg"}
  under systolic_blood_pressure and diastolic_blood_pressure.
- date_excel stays (it is the fact time and half the identity) and is also written as
  effective_time_frame.date_time, RFC 3339 with the Australia/Sydney offset in force.
- posture "Seated" becomes body_posture "sitting"; any other posture stops the run, since
  mapping one is a decision, not a guess.
- every other field stays exactly as it was.

Ids are untouched, so ingest supersedes each facet in place and keeps its history. The
run is deterministic: the same input writes the same output. Other record types pass
through unchanged. The extracted block gains a reshape note and its version goes up by one.

Usage: reshape-bp-diary-omh.py <records file in> <records file out>
"""

import sys
from datetime import datetime
from zoneinfo import ZoneInfo

import yaml

ZONE = ZoneInfo("Australia/Sydney")
POSTURES = {"Seated": "sitting"}


def number(text, field, rid):
    try:
        return float(text)
    except (TypeError, ValueError):
        sys.exit(f"{rid}: {field} {text!r} is not a number")


def reshape(row):
    rid = row.get("id", "?")
    out = {}
    for k, v in row.items():
        if k == "systolic":
            out["systolic_blood_pressure"] = {"value": number(v, k, rid), "unit": "mmHg"}
        elif k == "diastolic":
            out["diastolic_blood_pressure"] = {"value": number(v, k, rid), "unit": "mmHg"}
        elif k == "posture":
            if v not in POSTURES:
                sys.exit(f"{rid}: posture {v!r} has no OMH mapping; decide one first")
            out["body_posture"] = POSTURES[v]
        else:
            out[k] = v
    when = datetime.strptime(row["date_excel"], "%Y-%m-%d %H:%M:%S").replace(tzinfo=ZONE)
    out["effective_time_frame"] = {"date_time": when.isoformat()}
    return out


def main(src, dst):
    with open(src, encoding="utf-8") as f:
        doc = yaml.safe_load(f)
    n = 0
    for i, row in enumerate(doc["records"]):
        if row.get("type") == "health.blood-pressure":
            if "systolic_blood_pressure" in row:
                sys.exit(f"{row.get('id')}: already reshaped")
            doc["records"][i] = reshape(row)
            n += 1
    ext = doc.setdefault("extracted", {})
    ext["version"] = int(ext.get("version", 1)) + 1
    ext["reshaped"] = "health.blood-pressure v4, Open mHealth blood-pressure-4.0 body (card 83, scripts/reshape-bp-diary-omh.py)"
    with open(dst, "w", encoding="utf-8") as f:
        yaml.safe_dump(doc, f, sort_keys=False, allow_unicode=True, width=200)
    print(f"{n} blood pressure rows reshaped to v4 -> {dst}")


if __name__ == "__main__":
    if len(sys.argv) != 3:
        sys.exit(__doc__)
    main(sys.argv[1], sys.argv[2])
