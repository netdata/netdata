# Capture a representative Prometheus metrics dump

A validator can prove only what the dump contains. Capture the exact endpoint
and application configuration being observed, with Prometheus comments intact.
Do not silently treat that one configuration as the profile's complete support
surface.

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

Record provenance alongside the private dump:

- application/exporter version;
- configuration and enabled optional features, without secrets;
- source revision when known;
- capture time and endpoint role, using a sanitized endpoint description;
- SHA-256 of the immutable dump.

Interpret missing metadata carefully:

- Missing `HELP` reduces semantic evidence but does not make the exposition
  unparsable; use authoritative exporter documentation/source.
- Missing `TYPE` makes scalar semantics ambiguous. Narrow `fallback_type` can
  recover known untyped gauges/counters, and `_total` is treated as a counter.
- Missing `TYPE` does not always erase distribution structure. The parser can
  identify histogram buckets from `_bucket{le=...}`, summaries from a
  `quantile` label, and associate matching `_sum`/`_count` components with that
  structural witness. A scalar `_sum`/`_count` pair with no bucket or quantile
  witness remains ambiguous; obtain source evidence or a better dump instead of
  guessing.

## Make it representative

A quiet endpoint can hide important states even when metric names exist. Where
safe and authorized, capture a state that includes:

- successful and failed work;
- active and waiting/queued work;
- every intended entity/instance label combination;
- histogram buckets or summary quantiles with real values;
- optional subsystems the profile claims to support.

Zero is valid evidence that a family exists. Absence is not evidence that a
feature or metric never exists. A single snapshot proves exposition shape and
current state; it does not prove update cadence, monotonicity, reset behavior,
population relationships, or behavior under unobserved states.

Capture additional real states when safe, but do not stop at the available
deployment when the source defines other supported surfaces. Follow
[Build a source-derived synthetic fixture](build-synthetic-fixture.md) to cover
absent optional/configuration-gated families without misrepresenting them as
runtime observations.

## Keep dumps private and immutable

Metrics may contain workload names, routes, users, teams, hashed keys,
endpoints, or other identifiers. Store dumps under ignored `.local/` or user
temporary storage and never commit real operational dumps.

Record a hash for a validation run and do not edit the captured file in place.
The repository validator snapshots it again before the collector performs its
multiple scrapes, so one run uses immutable evidence.

Create committed regression fixtures only through the sanitized synthetic
workflow. Never copy a private operational dump into `testdata/` and then edit
individual labels until it appears safe.
