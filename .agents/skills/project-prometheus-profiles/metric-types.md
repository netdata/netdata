# Prometheus metric types: runtime facts and design choices

Prometheus type is not a dashboard taxonomy. It determines what the collector
can write and how chartengine reads it; the application meaning determines
whether and how it belongs in the dashboard.

## Contents

- [The ingestion model](#the-ingestion-model)
- [Missing TYPE metadata](#missing-type-metadata)
- [Source lifecycle owns the algorithm](#source-lifecycle-owns-the-algorithm)
- [Gauge](#gauge)
- [Counter](#counter)
- [Histogram](#histogram)
- [Summary](#summary)
- [Info families](#info-families)
- [Untyped families and fallback](#untyped-families-and-fallback)
- [Writer rejection and schema drift](#writer-rejection-and-schema-drift)
- [Collection filtering and relabeling](#collection-filtering-and-relabeling)
  - [Proving a replacement is actually redundant](#proving-a-replacement-is-actually-redundant)

## The ingestion model

Reason across four surfaces:

1. The exposition parser assembles lines into typed metric families.
2. Job selector/relabeling can remove, rename, or reshape series.
3. The Prometheus writer accepts supported families into metrix and rejects
   unsafe/unusable series.
4. `metrix.ReadFlatten()` exposes scalar names to chartengine.

A profile can only chart surface 4. A raw family missing from the writer is not
an autogen leak, but it still matters: the validation report must make that loss
visible so the author knows what evidence never reached the dashboard.

## Missing TYPE metadata

Do not equate a missing `# TYPE` line with a structurally unknowable family.
The parser can recover some distribution shapes from samples:

- `_bucket` samples carrying `le` identify histogram structure;
- samples carrying `quantile` identify summary structure;
- compatible `_sum` and `_count` components are associated with a structurally
  witnessed histogram or summary base.

The ambiguous case is an untyped scalar or a suffix-only `_sum`/`_count` set
without a bucket/quantile witness. Use narrow `fallback_type` only for
source-proven scalar gauges/counters. Obtain source evidence or a better fixture
instead of guessing a distribution from suffixes alone.

## Source lifecycle owns the algorithm

Prometheus wire type does **not** by itself determine the Netdata chart algorithm.
Before declaring `absolute` or `incremental`, trace the producer's complete value
lifecycle in source:

1. Find registration and initialization.
2. Find every update callsite, including assignments, additions, increments,
   decrements, swaps, and snapshot publication.
3. Find reset, reinitialization, and object-destruction paths.
4. Identify what one unit counts or measures at each update.

Apply these rules:

- A source value that represents a current population or state MUST use
  `absolute`, even when the exporter exposes it as a Prometheus counter. A value
  incremented on object creation and decremented on destruction is current state.
- A source value that represents cumulative work MUST use `incremental`, even
  when the exporter exposes it as a Prometheus gauge or republishes it as a
  periodic snapshot. Netdata receives the raw total and owns rate calculation
  and reset detection.
- A monotonically updated value MUST use the unit of the added quantity. An
  update such as `total += bytes` is byte throughput after `incremental`; it is
  not an event rate merely because the registration says counter.
- HELP text, metric-name suffixes, exporter wire type, and one observed scrape are
  evidence inputs, not substitutes for update-site verification. When they
  disagree with source lifecycle, follow the source and record the mismatch in
  the semantic ledger.

When `algorithm` is omitted, chartengine uses the written runtime series kind:
counters are incremental and gauges are absolute, regardless of the metric
name. If source lifecycle and runtime kind disagree, preserve the raw producer
value and declare the intentional chart algorithm explicitly. Do not compute
deltas or resets in the collector or profile.

## Gauge

Writer behavior:

- finite values are written as gauges;
- NaN and infinite values are skipped;
- the normal chart algorithm is `absolute`.

Design behavior:

- use gauges for current state: occupancy, queue depth, capacity, utilization,
  configured limits, temperature, or instantaneous ratios;
- a gauge named `*_total` remains absolute when the algorithm is omitted. The
  suffix is evidence to investigate a possible exporter mismatch, not an
  algorithm override;
- do not combine a large capacity gauge and a small operational state merely
  because both are absolute. Shared algorithm is not shared question or scale.

An epoch timestamp is not an uptime chart. A raw process-start or creation-time
gauge is a large, nearly flat seconds-since-epoch value; answering “how old is
it?” requires `now - start`, which the current profile schema cannot compute.
Either exclude it with the lost restart/age question disclosed, rely on a
verified integration that already computes uptime, or report the capability
gap. Do not chart the raw epoch merely to satisfy coverage.

## Counter

Writer behavior:

- finite values are written as counters;
- chartengine normally uses `incremental`, rendering change per second and
  handling counter resets through the metric store;
- a counter without a conventional suffix is still incremental when the
  algorithm is omitted.

Design behavior:

- chart event/work rates when they answer an operator question: requests/s,
  errors/s, operations/s, retries/s, bytes/s;
- preserve related dimensions when comparison is useful, such as response
  classes, but split a tiny error signal from huge traffic if the shared scale
  hides it;
- cumulative raw totals are rarely the useful live dashboard view. Do not label
  an incremental chart as a lifetime total.

The counted object is part of the unit. Do not combine counters for batches,
records, retries, and bytes by relabeling all of them `events/s`,
`operations/s`, or `items/s`. Those umbrella nouns hide different observation
populations rather than performing valid unit conversion. Research what one
increment means, use that noun in the chart, and split unlike objects even when
their algorithms and numeric scales happen to match.

An operation counter and a counter of objects produced by that operation are
also different. One operation can produce zero, one, or many objects. A shared
chart would assert a conversion ratio that the metrics do not compute, whether
it is mislabeled with the object's unit or disguised as generic `events/s`.

Apply unit algebra after the algorithm. A counter measured in seconds becomes
seconds/second under `incremental`, not a current duration. For
`process_cpu_seconds_total`, that dimensionless rate is CPU-core utilization
(`cores`) because the source accumulates CPU time while work runs. A duration
histogram `_sum` is normally updated only when an observation completes, so its
rate is completion-attributed work time (`seconds/s`), not live concurrency.
Titles and units must describe the rendered rate and contribution timing, not
only the raw counter name.

## Histogram

A valid histogram needs buckets, count, and sum. The writer validates bounds,
counts, and finiteness, then metrix flattens one logical histogram identity to:

| Flattened name | Runtime meaning | Typical design |
|---|---|---|
| `<base>_bucket` with `le` | non-overlapping bucket counter series | heatmap, inferred bucket dimension names |
| `<base>_count` | cumulative observation count | observation rate with `incremental` |
| `<base>_sum` | cumulative sum of observed values | semantic throughput or completed-work rate when meaningful |

Important consequences:

- The bare histogram base is not a flattened scalar dimension.
- Bucket values presented to the template are non-overlapping ranges, even
  though Prometheus exposition buckets are cumulative. Histogram bucket charts
  are forced to `heatmap`. Their dimension values are rates of observations in
  each range, so use `units: observations/s` with inferred or explicit
  `algorithm: incremental`; the bucket boundaries already communicate the
  observed value's unit.
- `+Inf` is synthesized from count by metrix after writer validation.
- A histogram with no buckets is rejected as a family; `_count` and `_sum` are
  not independently rescued.

Writer coverage and dashboard composition are different obligations. Every
flattened bucket, count, and sum series that survives the writer must be routed
to an authored dimension, but there is no mandatory “three charts per
histogram” or “four chart contract.” Choose chart boundaries from operator
questions:

- A latency heatmap can show distribution shape and outliers.
- Count rate can show workload volume.
- Sum rate can mean bytes/s for size observations or completion-attributed
  seconds/s for duration observations. Use a current population Gauge for live
  concurrency; unit cancellation alone cannot turn a completion observation
  into one.
- An average would require `sum rate / count rate`. The current profile schema
  cannot compute it, so do not fake an average from `_sum` alone.

Compatible roles from different related histograms may share a chart. For
example, same-unit count rates may compare observations across causal phases,
and same-unit duration-sum rates may compare accumulated work across those
phases. This is valid only when the populations, rendered meanings, algorithms,
and scales make one coherent operator comparison. It is not permission to
collect unrelated histogram mechanics under generic “Counts” or “Sums”
families.

Name one observation before combining `_count` rates. `observations/s` is a
transport unit, not a population: requests, batches, database rows,
transactions, and retries remain different things even when every selector
ends in `_count`. The chart title and dimension names must describe the real
population rather than relabel all observations as requests.

The same rule applies outside distributions. A generic `events/s` unit cannot
combine one counter whose increment is an operation with another whose
increment is an item processed by that operation.

Name what each `_sum` accumulates before combining sum rates. Equal
`seconds/s` does not prove additivity or even the same population: an
end-to-end duration can contain several phases, a time-to-first-result duration
can overlap both queue and processing, and a per-item average observed once per
request is not a phase duration. A useful comparison may show an explicit total
beside compatible components, but it must not imply a decomposition the
exporter does not guarantee.

Do not assume component distributions replace a directly exported total
distribution. Read/write, client/server, or enqueue/process histograms no
longer carry the per-observation correlation needed to reconstruct the total.
Their bucket sums can describe component marginals while the direct total
histogram answers the end-to-end question.

Keep bucket, count, and sum in separate charts when their units differ. Their
relationship belongs in nearby families/order, not in one chart with dishonest
units.

The roles are not made comparable by putting them all on an incremental chart:

- bucket/quantile surfaces describe distribution shape;
- `_count` is observations/s;
- `_sum` is observed-units/s.

For an items-per-batch histogram, count is batches/s while sum is items/s. One
`items/s` chart containing both would mislabel the count dimension.

## Summary

A valid summary needs count and sum; quantiles are optional. Metrix flattens:

| Flattened name | Runtime meaning | Typical design |
|---|---|---|
| `<base>` with `quantile` | current quantile value | absolute line chart, dimensions by quantile |
| `<base>_count` | cumulative observation count | observation rate |
| `<base>_sum` | cumulative observed sum | semantic throughput or completed-work rate when meaningful |

Writer safety changes what can be charted:

- A summary with no quantiles writes `_count` and `_sum` without fabricating a
  bare quantile series.
- A summary missing either count or sum is rejected instead of fabricating the
  absent component as zero.
- An all-NaN quantile window is skipped until a real quantile appears.
- A summary with some real quantiles may preserve a NaN quantile as a chart gap.
- Invalid/infinite quantiles, negative/non-finite count, or non-finite sum reject
  that logical summary series.

Quantiles are not histogram buckets. Do not stack them and do not add them: p50,
p90, and p99 are alternative distribution positions with the same units.

As with histograms, count and sum do not require dedicated per-family charts.
Every flattened series the writer produces still needs an authored route for a
complete profile. A quantile-free source therefore needs count and sum routes
but no quantile chart; a dead quantile chart is a stale profile assumption, not
evidence that the valid count/sum-only summary should be rejected.

## Info families

The writer skips a gauge family whose name ends in `_info`. Its constant value
and labels never enter metrix, so a profile cannot chart it or promote its
labels. The suffix alone is not a type: an explicitly typed counter ending in
`_info` remains writer-eligible and follows normal counter behavior.

Treat this as an evidence limitation, not an invitation to invent a workaround:

- if the metadata exists on another written series, promote or use it there;
- if it is operationally essential, the collector/exporter contract needs a
  different supported path;
- a job deny for `_info` can document intent but does not make labels available.

Some exporters expose the same metadata convention under a name that does not
end in `_info`, for example `runtime_node{node="a"} 1`. Production correctly
treats that family as a writable scalar gauge. A stock profile MAY leave the
numeric carrier unrendered only through the binding `metadata_only` case:

- source semantics prove a `constant` lifecycle, `count`/`one`/`none` unit
  algebra, and at least one metadata label;
- every replayed carrier value is exactly `1`;
- the design uses `outcome: retain_writable_unrendered`; and
- the profile denies that exact family from generic autogen.

This exception is not a numeric-value heuristic. A Boolean, state, or other
operator-meaningful gauge remains chartable even when one fixture happens to
contain only `1`.

## Untyped families and fallback

For unknown/untyped families:

- matching `fallback_type.gauge` writes a scalar gauge;
- matching `fallback_type.counter` writes a scalar counter;
- an untyped name ending in `_total` is treated as a counter;
- other untyped families are skipped.

Use fallback only with authoritative semantic evidence. It restores scalar type
behavior; it does not make an ambiguous suffix-only family a distribution.
Histogram/summary inference, when possible, happens earlier from structural
witnesses. A broad fallback pattern can silently convert unrelated families to
the wrong algorithm, so prefer the narrowest stable family set.

## Writer rejection and schema drift

The writer rejects unsafe series rather than letting invalid data corrupt the
metric store. Examples include:

- NaN/Inf scalar values;
- malformed histogram bounds or counts;
- invalid summary quantiles/count/sum;
- a histogram/summary instance whose bucket or quantile schema differs from the
  accepted schema for that family;
- a family over `max_time_series_per_metric`;
- incompatible type drift while a descriptor remains authoritative.

A family can be partially written: one identity is valid while another is NaN
or has a different distribution schema. Validation must report raw logical
identity count versus writer source identity count; a whole-family presence bit
would hide this loss.

Do not treat a writer rejection as profile coverage. The profile cannot route a
series it never receives. Decide whether the loss is an acceptable endpoint
fact, a job-policy defect, an exporter defect, or evidence that the dump is not
representative.

## Collection filtering and relabeling

The job selector filters efficiently by metric name/labels. Ordered relabeling
can keep/drop series, rename metrics, add/remove/normalize labels, and reshape
identity before family reassembly.

Relabeling is powerful because it changes the data contract:

- renaming only one member of a histogram/summary can split and corrupt it;
- removing the label that distinguishes distribution instances can merge them;
- mutating `le` or `quantile` can invalidate distribution structure;
- changing an instance label changes chart identity and cardinality.

The same whole-family caution applies to job selectors. Filtering a histogram
or summary `_count`, `_sum`, bucket, or quantile sample before family assembly
can make the writer reject the entire family; it is not a way to keep only the
visually convenient member. Curate every required structural role, or exclude
the whole family only when the exclusion policy permits losing its operator
question.

Read the relabel README in full and validate the exact ordered rules with the
profile.

Every exclusion trades away diagnostic capability. The binding exclusion cases
are defined in `proof-authoring.md`; a writer rejection is a pipeline limitation rather
than successful policy filtering. Weak reasons include “zero in this dump,”
“not interesting,” “deep dive,” “dashboard focus,” or “too hard to design.”
Process/runtime metrics are not categorically noise: CPU, memory, descriptors,
and GC can explain application failures. Keep or exclude them based on the
operator story and the binding cases, not a fixed deny-count quota.

### Proving a replacement is actually redundant

Before excluding one family as a replacement of another, compare:

- authoritative semantic definitions and lifecycle/deprecation guarantees;
- Prometheus type and update behavior;
- identity/aspect label sets and cardinality;
- histogram buckets or summary quantiles;
- units and the event/observation population being measured;
- observed behavior across representative states, not only one equal value.

Similar names, equal counts, or one snapshot of equal values are insufficient.
For distributions, equal `_count` does not imply equal populations, bucket
shape, sum, or diagnostic meaning.

Record the operator question lost by the exclusion and why the remaining
family answers that same question across the intended exporter versions and
deployment modes. If that case cannot be made, keep both surfaces and design
them around their distinct questions.
