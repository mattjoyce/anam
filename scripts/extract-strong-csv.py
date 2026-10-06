#!/usr/bin/env python3
"""Strong app CSV export -> a records file of gym.visit, one record per workout.

Usage: extract-strong-csv.py --out-dir DIR strong<digits>.csv

The export is one row per set, semicolon-separated. A workout is identified by its start
time (the "Workout #" column is renumbered per export, so it is not identity). Each record
carries the workout's rows verbatim under `sets`, in file order, minus the workout-level
columns; "Rest Timer" and "Note" rows stay as rows, because that is what the source says.
Deterministic; ids are strong:<ISO start>.
"""
import argparse, csv, hashlib
from collections import OrderedDict
from pathlib import Path
import yaml

def num(s):
    if s in ("", None):
        return None
    f = float(s)
    return int(f) if f.is_integer() else f

def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--out-dir", required=True)
    ap.add_argument("csv")
    a = ap.parse_args()
    p = Path(a.csv); sha = hashlib.sha256(p.read_bytes()).hexdigest()
    with open(p, encoding="utf-8-sig", newline="") as f:
        rows = list(csv.DictReader(f, delimiter=";"))
    workouts = OrderedDict()
    for r in rows:
        workouts.setdefault(r["Date"], []).append(r)
    records = []
    for start, wrows in workouts.items():
        first = wrows[0]
        iso = start.replace(" ", "T")
        rec = {"type": "gym.visit", "id": f"strong:{iso}", "date": start[:16], "started": start,
               "activity": "strength training", "app": "Strong", "workout_name": first["Workout Name"],
               "duration_sec": num(first["Duration (sec)"])}
        notes = sorted({r["Workout Notes"] for r in wrows if r["Workout Notes"]})
        if notes:
            rec["workout_notes"] = notes
        exercises = []
        for r in wrows:
            if r["Exercise Name"] not in exercises:
                exercises.append(r["Exercise Name"])
        rec["exercises"] = exercises
        sets = []
        for r in wrows:
            s = {"exercise": r["Exercise Name"], "set": r["Set Order"]}
            for col, key in (("Weight (kg)", "kg"), ("Reps", "reps"), ("RPE", "rpe"), ("Distance (meters)", "distance_m"), ("Seconds", "seconds")):
                v = num(r[col])
                if v is not None:
                    s[key] = v
            if r["Notes"]:
                s["note"] = r["Notes"]
            sets.append(s)
        rec["sets"] = sets
        records.append(rec)
    doc = {"document": {"name": p.name, "sha256": sha}, "extracted": {"method": "scripts/extract-strong-csv.py", "version": 1}, "records": records}
    out = Path(a.out_dir) / f"strong-workouts.{sha[:12]}.yaml"
    out.write_text(yaml.safe_dump(doc, sort_keys=False, allow_unicode=True, width=100))
    print(f"{out.name}: {len(records)} workouts from {len(rows)} rows, {records[0]['date']} to {records[-1]['date']}")

if __name__ == "__main__":
    main()
