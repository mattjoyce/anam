#!/usr/bin/env python3
"""Technogym mywellness export -> a records file of exercise.session, one per indoor session.

Usage: extract-technogym-json.py --out-dir DIR <export>.zip

Reads indooractivities-*.json from the zip (outdoor activities and biometrics are card 58's).
Each session's payload conforms to IEEE 1752.1 physical-activity-1.0, with Technogym's other
measures beside it in snake_case (journal note: open-mhealth-is-the-payload-vocabulary-for-
health-measures).

Identity is technogym:<on>. `on` is unique per session and carries the Sydney offset; `id` and
`createdOn` are minted per export. `on` is when the session ENDED: on 2025-02-21 Garmin stopped
recording at 19:23:14 and Technogym's `on` reads 19:23:03. So the start is `on` minus the
session's Duration, to the second, and `on` itself stays in the id.

A measure name this script does not know stops it: a new name needs a decision, not a guess.
Deterministic: sessions in `on` order, measures in a fixed order.
"""
import argparse, hashlib, json, zipfile
from datetime import datetime, timedelta
from pathlib import Path
import yaml

# IEEE 1752.1 physical-activity fields: name -> (field, unit)
IEEE = {
    "Duration": ("duration", "sec"),
    "RowingDistance": ("distance", "m"),
    "Calories": ("kcal_burned", "kcal"),
    "IsoReps": ("base_movement_quantity", "strokes"),
    "AvgSpm": ("average_cadence", "strokes/min"),
}
# Technogym measures IEEE has no field for, kept as the source reports them.
EXTRA = {
    "AvgPower": "avg_power_w", "MaxPower": "max_power_w", "MaxSpm": "max_spm",
    "RowingSplit": "split_500m_sec", "Level": "level", "AvgHr": "avg_hr_bpm",
    "MaxHr": "max_hr_bpm", "Move": "move", "CaloriesConsumption": "calories_consumption",
    "DragFactor": "drag_factor",
    "UserWeight": "user_weight_kg", "Floors": "floors", "Vo2": "vo2",
    "DrivePerc": "drive_perc", "AvgPeakForce": "avg_peak_force",
}
# Technogym measures left out on purpose. Mets and MetsMin are Technogym's own, undefined by
# its API, present on 3 of 498 sessions, and not METs: in each one Mets = MetsMin x minutes, the
# reverse of the names, and one rowing rate reads 427. Matt, 2026-10-09: bespoke, no value.
IGNORED = {"Mets", "MetsMin"}
# Splits are flat: each measure as a plain number under one name.
SPLIT = {"Duration": "duration_sec", "RowingDistance": "distance_m", "Calories": "kcal",
         "IsoReps": "strokes", "AvgSpm": "avg_spm", **EXTRA}
ORDER = list(IEEE) + list(EXTRA)

def num(v):
    return int(v) if float(v).is_integer() else v

def measures(pr, where):
    m = {}
    for p in pr:
        if p["n"] in IGNORED:
            continue
        if p["n"] not in IEEE and p["n"] not in EXTRA:
            raise SystemExit(f"{where}: unknown measure {p['n']!r}")
        if p["n"] in m:
            raise SystemExit(f"{where}: measure {p['n']!r} twice")
        m[p["n"]] = p["v"]
    return m

def activity(m, where):
    if "Floors" in m:
        return "stair_climbing.machine"
    if "RowingDistance" in m:
        return "rowing.machine"
    raise SystemExit(f"{where}: neither rowing nor stair climber: {sorted(m)}")

def session(s):
    on = s["on"]
    m = measures(s["performedData"]["pr"], on)
    if "Duration" not in m:
        raise SystemExit(f"{on}: no Duration")
    end = datetime.fromisoformat(on)
    start = end - timedelta(seconds=m["Duration"])
    start = (start + timedelta(microseconds=500000)).replace(microsecond=0)
    rec = {
        "type": "exercise.session",
        "id": f"technogym:{on}",
        "activity_name": activity(m, on),
        "effective_time_frame": {"time_interval": {
            "start_date_time": start.isoformat(timespec="seconds"),
            "duration": {"value": num(m["Duration"]), "unit": "sec"}}},
    }
    for name in ORDER:
        if name not in m:
            continue
        if name in IEEE:
            field, unit = IEEE[name]
            rec[field] = {"value": num(m[name]), "unit": unit}
        else:
            rec[EXTRA[name]] = num(m[name])
    rec["source"] = "Technogym mywellness"
    rec["ph_id"] = s["phId"]
    rec["facility"] = s["facility"]
    splits = []
    for sp in s["performedData"]["st"]:
        sm = measures(sp["pr"], f"{on} split")
        row = {"position": sp["pos"]}
        for name in ORDER:
            if name in sm:
                row[SPLIT[name]] = num(sm[name])
        splits.append(row)
    if splits:
        rec["splits"] = splits
    return rec

def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--out-dir", required=True)
    ap.add_argument("zip")
    a = ap.parse_args()
    p = Path(a.zip)
    sha = hashlib.sha256(p.read_bytes()).hexdigest()
    with zipfile.ZipFile(p) as z:
        names = [n for n in z.namelist() if Path(n).name.startswith("indooractivities-")]
        if len(names) != 1:
            raise SystemExit(f"want one indooractivities file, found {names}")
        sessions = json.loads(z.read(names[0]))
    ons = [s["on"] for s in sessions]
    if len(set(ons)) != len(ons):
        raise SystemExit("two sessions share an `on`; identity would collide")
    records = [session(s) for s in sorted(sessions, key=lambda s: datetime.fromisoformat(s["on"]))]
    doc = {"document": {"name": p.name, "sha256": sha},
           "extracted": {"method": "scripts/extract-technogym-json.py", "version": 2},
           "records": records}
    out = Path(a.out_dir) / f"technogym.{sha[:12]}.yaml"
    out.write_text(yaml.safe_dump(doc, sort_keys=False, allow_unicode=True, width=100))
    kinds = {}
    for r in records:
        kinds[r["activity_name"]] = kinds.get(r["activity_name"], 0) + 1
    print(f"{out.name}: {len(records)} sessions {kinds}, "
          f"{records[0]['id'][10:20]} to {records[-1]['id'][10:20]}")

if __name__ == "__main__":
    main()
