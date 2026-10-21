# Faux Record for the Lab

This directory contains a faux (fake) Anam record for testing purposes in the lab environment.

## What

A synthetic but realistic personal health and activity record with facts across all live facet types:
- `health.weight`, `health.blood-pressure`, `health.measurement.withings`, `health.sleep-episode`
- `activity.steps`, `activity.day.garmin`, `health.heart-rate.day`, `health.heart-rate-alert`
- `activity.exercise-session` variants for garmin, concept2, technogym, strong
- `health.cpap-session.dreamstation`
- Medication courses and doses
- `record.edge` notes and links between facts
- `place.trip` variants
- `place.track`, `place.stay`
- `health.lab-result` and `health.lab-panel`
- `record.document`

## How to rebuild it

To regenerate the faux record:

```bash
# From the anam project root
./anam-bench gen --shape realistic --dir /path/to/faux-record --seed 42
```

The `--seed` parameter ensures deterministic generation - the same seed will always produce the same record.

## How to host it

To start a daemon serving the faux record:

```bash
# From the anam project root
./scripts/faux-lab.sh /path/to/faux-record 18790
```

This will:
1. Generate a record if one doesn't exist in the directory
2. Create a temporary token file
3. Start the daemon listening on localhost only
4. Print the token for accessing the daemon

## How to stop it

Simply press Ctrl+C to stop the daemon.

## Accessing the hosted record

Once running, you can access the record using the Anam CLI:

```bash
export ANAM_SERVER=http://127.0.0.1:18790
export ANAM_TOKEN=<token-shown-by-script>

./anam status
./anam timeline
./anam timeline --projection bp
```

Or access it via curl:

```bash
curl -H "Authorization: Bearer <token>" http://127.0.0.1:18790/v1/status
```

## Security

The daemon only listens on localhost (127.0.0.1) and uses a temporary token file, making it safe to run in the lab environment without exposing it to the network.