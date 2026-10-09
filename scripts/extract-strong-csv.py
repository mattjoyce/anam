#!/usr/bin/env python3
"""Strong app CSV export -> a records file of exercise.session, one record per workout.

Usage: extract-strong-csv.py --out-dir DIR strong<digits>.csv

The export is one row per set, semicolon-separated. A workout is identified by its start
time (the "Workout #" column is renumbered per export, so it is not identity). Each record
carries the workout's rows verbatim under `sets`, in file order, minus the workout-level
columns; "Rest Timer" and "Note" rows stay as rows, because that is what the source says.
Deterministic; ids are strong:session:<ISO start>. (Version 1 wrote gym.visit with ids
strong:<ISO start>; a new type takes new ids, so the move is by retraction, not supersession.)

Each payload conforms to IEEE 1752.1 physical-activity-1.0 as `strength_training`, which
requires an interval. Strong keeps a workout open until the next one starts, so its duration
is unreliable (some run for days, one is negative). Matt's rule, 2026-10-08: a Strong session
is 3 hours at most. The interval's duration is Strong's own figure when it is above 0 and at
most 3 hours, and 3 hours otherwise; then `duration_capped: true` marks it, and Strong's
number is always kept as reported_duration_sec. The start is Strong's local clock read in
Australia/Sydney; a workout logged abroad is placed as if at home.
"""
import argparse, csv, hashlib
from collections import OrderedDict
from datetime import datetime
from zoneinfo import ZoneInfo

MAX_SESSION_SEC = 3 * 3600  # Matt, 2026-10-08
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
        at = datetime.fromisoformat(start).replace(tzinfo=ZoneInfo("Australia/Sydney"))
        reported = num(first["Duration (sec)"])
        capped = not (0 < reported <= MAX_SESSION_SEC)
        rec = {"type": "exercise.session", "id": f"strong:session:{iso}",
               "activity_name": "strength_training",
               "effective_time_frame": {"time_interval": {
                   "start_date_time": at.isoformat(timespec="seconds"),
                   "duration": {"value": MAX_SESSION_SEC if capped else reported, "unit": "sec"}}},
               "source": "Strong", "workout_name": first["Workout Name"],
               "reported_duration_sec": reported}
        if capped:
            rec["duration_capped"] = True
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
    doc = {"document": {"name": p.name, "sha256": sha}, "extracted": {"method": "scripts/extract-strong-csv.py", "version": 2}, "records": records}
    out = Path(a.out_dir) / f"strong-workouts.{sha[:12]}.yaml"
    out.write_text(yaml.safe_dump(doc, sort_keys=False, allow_unicode=True, width=100))
    print(f"{out.name}: {len(records)} workouts from {len(rows)} rows, {records[0]['id'][15:25]} to {records[-1]['id'][15:25]}")

if __name__ == "__main__":
    main()
