# Designing the dashboard the profile creates

A profile is an information architecture. The YAML hierarchy becomes navigation,
labels become identity/filtering/dimensions, chart composition determines what
can be compared, and priority determines the reading order. Design those UX
consequences deliberately.

## Contents

- [Start with operator questions](#start-with-operator-questions)
- [Build the application and entity hierarchy](#build-the-application-and-entity-hierarchy)
- [Assign labels by role](#assign-labels-by-role)
- [Compose charts around one comparison](#compose-charts-around-one-comparison)
- [Resolve common design conflicts](#resolve-common-design-conflicts)
- [Choose chart types for visual meaning](#choose-chart-types-for-visual-meaning)
- [Make titles, units, and conversions honest](#make-titles-units-and-conversions-honest)
- [Order the operator journey](#order-the-operator-journey)
- [Protect identity and cardinality](#protect-identity-and-cardinality)
- [Recognize bad patterns by their effects](#recognize-bad-patterns-by-their-effects)
- [Semantic review](#semantic-review)

## Start with operator questions

Write the questions before the charts. Typical first questions are:

- Is the service doing useful work?
- Are users receiving errors or degraded responses?
- Where is work waiting?
- Which resource or limit is saturated?
- Which entity is responsible?
- Is the condition transient, growing, or stuck?

The right set depends on the application. A request-processing service may lead
with throughput, outcomes, queueing, and latency. A database may lead with
workload, transactions, locks, buffer/cache behavior, and storage. Use the
application's vocabulary, not Prometheus mechanics such as “Counters,”
“Gauges,” or “Histograms.”

Why: operators arrive with a system problem, not a metric type. Organizing by
export format makes them translate the exporter before they can diagnose the
application.

For every proposed chart, be able to finish this sentence:

> When an operator sees this chart change, it helps answer ___ by comparing ___.

If the sentence has several unrelated answers, split or redesign the chart.

## Build the application and entity hierarchy

NIDL families are recursive navigation. Use them to tell a stable story:

1. application capability or pipeline stage;
2. subsystem or diagnostic lens;
3. entity-specific detail where useful.

This is guidance, not a fixed tree. The best hierarchy follows causal reasoning:
traffic enters, work queues, execution consumes resources, output succeeds or
fails. Put high-signal health/workload views before supporting internals.

Cross-cutting overview charts can be valuable, but a generic observability class
must not become a dumping ground. Keep a small service-level SLI overview when
it answers “are users affected?” across stages. Put stage-specific latency,
throughput, saturation, and errors with the stage that causes them, because that
placement turns a symptom into a causal path. A family named “Latency” that
contains queue, read, compute, and write timings forces the operator to rebuild
the pipeline mentally.

Classify signal roles, but use them as diagnostic lenses *inside* each
capability. A useful capability section can place its workload, outcomes,
errors, latency, saturation, and resource pressure together or in adjacent
subfamilies so the operator sees a holistic explanation of that function. Do
not gather every workload signal, every error, or every same-unit metric from
unrelated functions into application-wide buckets. Shared role or unit tells
how signals may be compared; it does not tell where the operator looks for
their cause.

Entity level and functional hierarchy are independent axes. A service-level
section can contain HTTP, process, runtime, and garbage-collection signals, but
sharing global identity does not make a long flat list coherent. Nest those
subsystems under the service entity so navigation still reflects what the
application does.

Entity level matters as much as application stage. Distinguish, for example:

- service-wide state;
- cluster/server/database instance;
- worker/replica;
- endpoint/operation;
- device/table/queue.

A context should represent one homogeneous entity type. Mixing service-wide and
per-worker charts under the same semantic context produces confusing filters,
cardinality, and history.

### Build a monotonic identity lattice

Treat each entity type as an identity-label set. A narrower descendant retains
the labels that identify its parent and adds only what identifies the narrower
entity. For example:

```text
service:  {}
server:   {server}
database: {server, database}
table:    {server, database, table}
index:    {server, database, table, index}
```

The names are illustrative; derive the real entity types and stable labels
from the exporter. The structural property is the subset relationship:

```text
parent identity ⊆ child identity
```

This matters because a filter applied at the parent should continue to select
every descendant. If a database child drops `server`, or an index child drops
`database`, the hierarchy may look nested while filtering no longer describes
one coherent entity path.

Each displayed leaf family should contain charts for one effective entity
identity. Do not place a global chart, a per-server chart, and a per-table chart
under the same displayed leaf merely because they explain the same capability.
Move them to explicit entity-level branches, or choose a common identity only
when every selected series truly carries it.

Group `chart_defaults.instances.by_labels` can make an entity boundary explicit
and reduce repetition. A child override replaces rather than extends the
parent default, so repeat the parent labels when adding a narrower entity label.
Never add a label that the selected series does not actually carry merely to
make the lattice look consistent.

### Shared identity across sibling families

Sibling second-level families often participate in one section-wide filter. In
that case their charts should share the parent entity identity labels. If one
subtree is per-database and another is global, a database filter cannot behave
uniformly.

That mismatch can be intentional. Resolve it explicitly:

- move global charts to a service-level sibling;
- add the common parent identity when the series truly carries it;
- nest table-specific charts under the database-level section;
- retain the mismatch only when the UI boundary is meant to change entity level,
  and explain it.

The validator reports mixed leaf identities, descendant loss of an explicitly
declared parent identity, and siblings with no common explicit identity. These
are structural prompts, not a domain verdict: it cannot decide which entity
boundary matches the operator's mental model.

## Assign labels by role

A label can play several roles, but each role has different UX and cardinality:

| Role | Mechanism | Effect | Good candidates |
|---|---|---|---|
| Entity identity | `instances.by_labels` | creates separate chart instances and filter identity | server, database, table, device |
| Comparable aspect | `name_from_label` or selector-specific static names | creates dimensions within one chart | status class, method, bounded phase |
| Descriptive metadata | `label_promotion` | adds filter/group metadata without splitting identity | region, version-like stable attribute |
| Routing constraint | selector label match | includes only the intended series | role, operation, state |
| Collection transformation | relabeling | changes/drops names or labels before writing | normalization backed by a clear contract |

Ask what a label *means*, not merely whether it exists.

### Identity labels

Use the smallest stable set that uniquely identifies the entity the operator
selects. Too few labels merge different entities. Too many labels fragment one
entity whenever incidental metadata changes.

`instances.by_labels: ['*']` is open-ended: any future exporter label can change
identity and multiply charts. Use it only when the exporter contract itself says
all labels are identity. Explicit labels are easier to reason about and keep
stable.

### Dimension labels

A dimension label should have bounded values that are useful side by side. HTTP
status class can be a good comparison; raw URL, request ID, user ID, or arbitrary
error text usually creates an unreadable and unbounded chart.

If the values identify independently filterable entities, they normally belong
in instances. If the values are aspects of one entity, dimensions are usually
better. This distinction preserves both readability and filtering.

Distinguish a closed enum from a configuration-dependent set:

- Use explicit selector/value dimensions when authoritative exporter semantics
  define a stable closed set and deliberate names/order matter.
- Use `name_from_label` when values can grow with configuration, workload, or
  exporter version.
- One dump showing two values does not prove a two-value enum. Hard-coding those
  values silently drops future values into autogen/unmatched behavior.

The design review must account for every observed label key, including labels
intentionally aggregated away. Aggregation is a decision because it removes
the ability to compare that label in the dashboard.

The validator warns when selected series carry a label that the chart does not
use for identity, dimension naming, promoted metadata, selector routing, or an
explicit `by_labels` exclusion. One observed value does not make the warning
irrelevant: a later second value may be a distinct entity that would be merged.
Resolve the warning by reasoning about the exporter's label contract and
expected cardinality. Do not mechanically promote every label or add it to
identity; intentional aggregation is valid when the lost comparison is stated
and correct for the operator story.

### Promoted labels

Promotion is metadata, not identity. Use it for stable attributes that help the
operator filter or explain a chart. It does not make a missing identity label
available, and it cannot recover labels from writer-skipped `_info` metrics.

## Compose charts around one comparison

Metrics belong together only when all of these are true:

1. They answer one operator question.
2. Their units mean the same thing.
3. Their chart algorithm is compatible.
4. Their scale allows each signal to remain visible.
5. Their dimension cardinality remains readable.

Shared units are necessary, not sufficient. Requests/s and errors/s are related,
but a tiny error rate can disappear under large traffic. Used bytes and queue
bytes are both bytes, but they may describe unrelated resources.

A useful composition is often a bounded breakdown of one whole: response
classes, cache hit/miss outcomes, pipeline phases, input/output directions, or
queue states. The comparison should reduce diagnostic time rather than save
chart count.

Do not optimize for few charts. Optimize for a coherent operator scan with no
important signal hidden.

### Coverage is not one metric per chart

Objective coverage asks whether every writer-surviving flattened series reaches
an authored dimension. It does not prescribe a chart count or a chart boundary.

- Route every gauge, counter, bucket, quantile, count, and sum role.
- Compose compatible same-unit roles from related metric families when one
  chart creates a useful comparison.
- Split when the operator question, event population, rendered meaning, scale,
  algorithm, or cardinality differs.
- Do not create generic metric-type sections merely to prove that each
  histogram produced separate bucket/count/sum charts.

This separation preserves both completeness and design judgment: coverage
prevents silent evidence loss, while composition determines whether the
dashboard teaches the application and shortens diagnosis.

Before merging dimensions, state in one sentence what comparison the chart
answers and what one unit of every dimension represents. A long legend can be
correct, but it is a warning sign when the sentence needs several unrelated
clauses, when actual work is mixed with requested limits, or when a total is
presented as one of its own phases. Move secondary comparisons into the causal
family that owns them rather than turning Overview into a coverage inventory.

## Resolve common design conflicts

### Related signals, incompatible scale

Split them into adjacent charts under the same family and order. Relationship is
preserved through navigation and proximity; visibility is preserved through
separate axes.

Current-versus-capacity pairs deserve the same check. Open descriptors and
descriptor limit share units and one resource story, but a very large limit can
flatten all movement in the open count. If the schema cannot compute a useful
utilization ratio, separate current and capacity charts rather than sacrificing
the operational signal. The validator's observed-scale warning is evidence from
one dump, not proof of every deployment.

“These values are conventionally shown together” does not resolve a scale
warning. Relatedness explains why an operator may compare the signals; it does
not make the smaller line visible. Keeping the shared chart requires evidence
that the UI still exposes meaningful movement or that the deployment range
keeps the ratio readable. Otherwise use adjacent charts so the relationship
remains clear without flattening the actionable signal.

### Same metric, multiple entity levels

Do not force one identity policy across all uses. Place charts under the entity
level they represent and scope defaults narrowly. A root `chart_defaults`
identity is harmful when descendants include global and per-entity series.

### Rich label, unbounded cardinality

Prefer a bounded aggregation already exported. If none exists, consider job
relabel/drop only after explaining the lost detail. Do not turn arbitrary label
values into dimensions and rely on lifecycle caps to make the chart usable.

### Distribution shape versus volume

Keep histogram heatmap/summary quantiles close to count and semantic sum charts,
but do not combine different units in one chart. Distribution answers “how is
latency/size shaped?”; count answers “how much work?”; sum rate may answer
“how much resource/throughput?”

### Errors are both outcomes and diagnostics

A response-class chart can show error proportion, while a focused error-rate
chart can keep rare failures visible. This is not duplication when the charts
answer different questions. Avoid duplicating identical visualizations merely
under multiple families.

### Job policy versus profile coverage

A profile cannot chart a series filtered or rejected before metrix. Decide the
job policy and profile together. An exclusion is correct only when losing that
signal is intentional and explained; `metrics:` declaration is never a drop or
coverage mechanism.

## Choose chart types for visual meaning

Use chart type as visual semantics, not decoration:

- `line`: rates, counts, latency/quantiles, ratios, state, and most time-varying
  signals. Lines preserve independent trends and crossings.
- `area`: physical volume, space, bandwidth, or I/O where filled magnitude is
  meaningful, including a deliberate mirrored in/out volume.
- `stacked`: non-overlapping physical volumes where total and component share
  are meaningful.
- `heatmap`: histogram bucket distribution. Bucket charts are forced to heatmap
  by the compiler. Declare `type: heatmap` explicitly so source intent matches
  the UI that runs.

Discrete work/event, count, state, and time charts MUST use line. Their
dimensions may add to a total, but additive categories are not physical volume:
stacking status classes, GC generations, request outcomes, or operation types
turns diagnostic trends into an area-composition display the user did not ask
for. Bandwidth remains a valid filled rate because the fill represents physical
flow. The validator rejects filled charts when units make the non-volume
violation unambiguous and warns when physical-rate or ambiguous units still
need design judgment.

A negative multiplier is a presentation convention, not negative data. Use it
only when the below-zero direction communicates a real pair such as inbound and
outbound traffic, and make the title/units clear.

## Make titles, units, and conversions honest

A title is a promise about what the chart computes.

- “by database” requires actual per-database instances or dimensions.
- “by endpoint,” “per backend,” and similar titles require an instance label,
  dynamic dimension, or selector split that preserves that entity/aspect.
  Merely carrying the label in the source is insufficient: if it is used by
  none of those mechanisms, chartengine aggregates it away.
- “average latency” requires an average; `_sum` rate alone is not an average.
- “requests” versus “requests/s” must match the algorithm.
- “utilization” should state percentage/ratio semantics rather than expose a raw
  limit or count.

Keep unit strings consistent within a chart. Use integer multiplier/divisor for
supported conversions and verify dimensional analysis. For example, bytes
multiplied by 8 and divided by 1000 becomes kilobits; a counter with incremental
then becomes kilobits/s.

Perform the same dimensional analysis for time counters. Incremental seconds
per second is a dimensionless concurrency/utilization quantity. Process CPU
seconds become used CPU cores; a duration sum can become accumulated concurrent
work. Calling either “time” after applying `incremental` describes the raw
counter instead of the rendered chart.

Avoid conversions chosen only to make lines look similar. If two dimensions
need unrelated scale manipulation, they probably do not belong on one axis.

## Order the operator journey

Every chart needs an explicit positive priority because missing/zero becomes
`70000`; YAML order is not runtime priority.

Use both channels deliberately:

- order families and charts in the file exactly as a reviewer should read the
  dashboard;
- assign priorities that express the intended runtime presentation;
- prefer unique increasing priorities when there is a total order;
- never decrease priority as the file proceeds;
- allow a tie only when a total order is unnecessary and the fallback ordering
  is acceptable.

A useful default story is health/workload → failures/latency → queueing/resource
pressure → detailed internals. Change it when the application's actual failure
modes demand another order.

## Protect identity and cardinality

Context, chart ID, instance labels, and dimension names become durable runtime
identity. Changes can create parallel charts and leave old metadata/history to
expire.

Before choosing dynamic identity:

- inventory observed cardinality and label combinations;
- check whether label values are stable across scrapes/restarts;
- consider optional future labels documented by the exporter;
- validate values that normalize similarly, such as punctuation variants;
- use lifecycle only with an understood loss/expiry policy.

A schema-valid collision can silently suppress or merge charts. The objective
validator checks observed cross-template IDs, same-template instance collapse,
lifecycle dimension loss, and public-wire normalization of chart IDs,
contexts, and dimensions. Unseen future values remain a review risk.

## Recognize bad patterns by their effects

- **Families named Gauges/Counters/Histograms:** expose transport mechanics and
  force operators to reconstruct the application.
- **One metric per chart by default:** loses useful comparison and creates a long
  dashboard; separate only when meaning/scale requires it.
- **Everything on one chart:** hides smaller signals, mixes questions, and
  produces dimension overload.
- **Every label in identity:** creates unstable cardinality and fragmented
  history.
- **Every label as a dimension:** turns entity cardinality into an unreadable
  legend.
- **Broad deny list:** makes validation pass by deleting diagnostic surface
  instead of designing it.
- **Default priority everywhere:** avoids deciding the operator journey and
  leaves order to unrelated IDs.
- **Titles derived mechanically from metric names:** repeat exporter syntax
  instead of explaining operational meaning.
- **Passing the validator as definition of quality:** proves runtime behavior for
  evidence, not the semantic usefulness of the dashboard.

## Semantic review

After objective validation passes, review the dashboard design:

- Can the first screen distinguish traffic loss, user-visible failure,
  queueing, and saturation?
- Does navigation follow application capabilities and causal diagnosis?
- Is each context homogeneous in entity type?
- Does each displayed leaf family contain one effective entity identity?
- Does descendant identity retain its parent labels?
- Are identity labels minimal, stable, and filterable?
- Are dimensions bounded comparable aspects?
- Do sibling families share the intended parent identity?
- Does every chart satisfy question, units, algorithm, scale, and cardinality?
- Are rare failures visible beside high-volume traffic?
- Are titles and units mathematically true?
- Is each exclusion justified by the capability lost?
- Do file order and explicit priorities tell the intended operator story?

Warnings demand reasoning, not mechanical edits. Record why the design is
intentional or change it when the UX consequence is wrong.
