# Capture a representative Prometheus metrics dump

A validator can prove only what the dump contains. Capture the exact endpoint
and application configuration the profile is intended to support, with
Prometheus comments intact.

## Capture

For a local unprotected endpoint:

```bash
curl -fsS "http://127.0.0.1:PORT/metrics" -o metrics.txt
```

For an authenticated endpoint, use the operator's existing secret-safe curl or
service access method. Do not paste a bearer token into a command, prompt,
transcript, or repository file, and do not invent a credential-handling path.

Do not pipe through `grep` or another transformation during capture. `# HELP`
and `# TYPE` lines are part of the evidence.

## Check the evidence

```bash
grep -c '^# TYPE ' metrics.txt
grep -c '^# HELP ' metrics.txt
wc -l metrics.txt
```

Then inspect family names, types, label keys, and cardinality without copying
sensitive label values into durable artifacts.

Interpret missing metadata carefully:

- Missing `HELP` reduces semantic evidence but does not make the exposition
  unparsable; use authoritative exporter documentation/source.
- Missing `TYPE` makes scalar semantics ambiguous. Narrow `fallback_type` can
  recover known untyped gauges/counters, and `_total` is treated as a counter.
- Missing `TYPE` cannot reconstruct histogram/summary family structure. Obtain a
  better dump or authoritative fixture rather than pretending raw suffix series
  are a distribution.

## Make it representative

A quiet endpoint can hide important states even when metric names exist. Where
safe and authorized, capture a state that includes:

- successful and failed work;
- active and waiting/queued work;
- every intended entity/instance label combination;
- histogram buckets or summary quantiles with real values;
- optional subsystems the profile claims to support.

Zero is valid evidence that a family exists. Absence is not evidence that a
feature or metric never exists. A single-dump validation cannot certify an
optional surface absent from that dump; obtain another representative fixture
or state the limitation.

## Keep dumps private and immutable

Metrics may contain model names, routes, users, teams, hashed keys, endpoints,
or other identifiers. Store dumps under ignored `.local/` or user temporary
storage and never commit real operational dumps.

Record a hash for a validation run and do not edit the captured file in place.
The repository validator snapshots it again before the collector performs its
multiple scrapes, so one run uses immutable evidence.
