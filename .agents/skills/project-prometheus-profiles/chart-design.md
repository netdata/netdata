# Designing the dashboard the profile creates

A profile is an information architecture. The YAML hierarchy becomes navigation,
labels become identity/filtering/dimensions, chart composition determines what
can be compared, and the UI determines presentation order. Design the profile-owned
UX consequences deliberately.

## Contents

- [Start with operator expectations](#start-with-operator-expectations)
- [Make flat metrics self-describing](#make-flat-metrics-self-describing)
- [Model entities, modules, and operations](#model-entities-modules-and-operations)
- [Learn from common operator models](#learn-from-common-operator-models)
- [Assign labels by role](#assign-labels-by-role)
- [Compose charts around one comparison](#compose-charts-around-one-comparison)
- [Resolve common design conflicts](#resolve-common-design-conflicts)
- [Choose chart types for visual meaning](#choose-chart-types-for-visual-meaning)
- [Make titles, units, and conversions honest](#make-titles-units-and-conversions-honest)
- [Order the operator journey](#order-the-operator-journey)
- [Protect identity and cardinality](#protect-identity-and-cardinality)
- [Recognize bad patterns by their effects](#recognize-bad-patterns-by-their-effects)
- [Semantic review](#semantic-review)

## Start with operator expectations

Begin with:

> What do operators expect to see about this entity, module, or operation?

Then ask the diagnostic questions at that owner:

- Is it doing useful work?
- Are its outcomes failing or degraded?
- Where does its work wait?
- Which resource or limit constrains it?
- Is the condition transient, growing, or stuck?

For every proposed chart, be able to finish this sentence:

> When an operator sees this chart change, it helps answer ___ by comparing ___.

If the sentence has several unrelated answers, split or redesign the chart.

## Make flat metrics self-describing

Prometheus exposition provides flat labeled time series. A separately authored
dashboard can supply relationships by placing particular queries and panels
together.

Netdata's generic dashboards instead organize charts from the metadata created
by the profile:

- families are navigation;
- contexts are semantic chart types;
- instances are monitored entities;
- dimensions are bounded comparisons;
- labels are filtering/explanatory metadata.

The profile must therefore make the relationships reusable and self-describing.
It is not enough that one reviewer can infer a dashboard from metric names.

This difference is especially important for common protocols. Two metrics can
both be HTTP request rates without belonging to one Netdata context:

- If endpoint/module/path X and Y are distinct operator-managed entities or
  processing functions, give them distinct entity instances or semantic
  families.
- If method and status are bounded aspects of one endpoint and one operator
  question, they can be chart dimensions.
- Do not mix X and Y merely because both render as `requests/s`; the shared unit
  permits comparison only after ownership and entity scope agree.

## Model entities, modules, and operations

Operators commonly understand an application through a combination of three
structures:

1. **Entities and containment:** service → server → database → table → index, or
   cluster → workload → pod → container.
2. **Modules and capabilities:** listener, scheduler, cache, router, storage
   engine, backend pool, executor.
3. **Operations and processing stages:** receive → authenticate → queue →
   process → persist → respond, or accept → route → forward → retry.

NIDL families should express those structures in the vocabulary operators use.
For each proposed first- or second-level family, ask:

- What entity, module, operation, or stage does it represent?
- What work/state does it own?
- What does it receive, process, store, or hand off?
- What condition can it cause?
- At which entity level should dashboard filtering apply?

Then place workload, outcomes, errors, latency, saturation, capacity, and
resource pressure under that owner. These signal roles form a holistic
diagnostic view of the owner; they are not normally application-wide navigation.

Use the closest defensible owner:

- stage-local signals stay with the stage that produces, consumes, queues, or
  controls them;
- end-to-end signals stay with the operation or nearest lifecycle owner they
  span;
- requested limits/options stay with admission or the operation they shape;
- resource pressure stays with its consumer unless the resource is itself an
  operator-managed entity/subsystem; and
- the same data object can belong to different stages when it is received,
  cached, transformed, persisted, or emitted.

A small service-impact overview can answer “are users affected?” across owners.
It must not become a dumping ground for every workload, latency, error, or
resource metric.

Labels help reveal entities, but do not define them automatically. Check source
semantics, observed combinations, cardinality, and stability. A `database`,
`backend`, `pod`, or `handler` label may be entity identity. A `status`,
`method`, `reason`, or `operation` label may be a bounded dimension. The name
alone is insufficient.

## Learn from common operator models

These examples illustrate the reasoning; they are not fixed trees.

### Database server

Operators may need a containment lattice such as:

```text
server:   {server}
database: {server, database}
table:    {server, database, table}
index:    {server, database, table, index}
```

Place each signal at the entity that owns it:

- server-wide connections, process resources, and a shared buffer pool remain
  server-level;
- per-database transactions, locks, failures, and cache behavior belong to the
  database when the exporter actually labels them that way;
- table reads, writes, scans, latency, and storage belong to the table;
- index lookups, misses, maintenance, and size belong to the index.

“Connections” or “cache” does not have one universal level. Ownership and labels
decide whether it is server-wide, database-specific, or table-specific.

### Proxy

Operators may think in terms of:

```text
proxy service → frontend/listener → route or backend pool → backend server
```

- Frontend acceptance, client-facing outcomes, and client latency belong near
  the frontend/listener.
- Routing decisions, misses, and reroutes belong to routing.
- Upstream connections, retries, failures, and response latency belong to the
  backend pool/server that causes them.

A global `Latency` family mixing client, routing, connection, and upstream
timings destroys that causal path even though every chart uses seconds.

### Kubernetes-hosted microservice

Two structures can coexist:

```text
platform entities: cluster → namespace → workload → pod → container
application flow:  API → queue → worker → datastore
```

- Container CPU/memory belongs to the container or workload entity represented
  by the labels.
- API request outcomes and latency belong to the API operation/module.
- Queue depth and wait time belong to the queue.
- Worker throughput, retries, and processing latency belong to the worker stage.
- Datastore calls belong to the datastore/client operation, not a global
  application latency section.

Pod names may be short-lived while workload identity is stable. Do not turn
every Kubernetes label into chart identity; choose the entity level operators
expect to filter and whose history should remain meaningful.

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

The validator fails when a selected writer series lacks a label required by the
chart's effective instance identity: that chart cannot materialize at the
declared entity level. It also reports mixed leaf identities, descendant loss of
an explicitly declared parent identity, and siblings with no common explicit
identity as review prompts. The tool can prove structural contradictions; it
cannot decide which valid entity boundary matches the operator's mental model.

## Assign labels by role

A label can play several roles, but each role has different UX and cardinality:

| Role | Mechanism | Effect | Good candidates |
|---|---|---|---|
| Entity identity | `instances.by_labels` | creates separate chart instances and filter identity | server, database, table, device |
| Ownership path | promoted label and, when needed for uniqueness, `instances.by_labels` | places the entity in its containing hierarchy without changing its leaf type | cluster, database, pool, namespace |
| Comparable aspect | `name_from_label` or selector-specific static names | creates dimensions within one chart | status class, method, bounded phase |
| Descriptive detail | `label_promotion` | adds filter/group metadata without splitting identity | serial number, display name, version |
| Routing constraint | selector label match | includes only the intended series | role, operation, state |
| Collection transformation | relabeling | changes/drops names or labels before writing | normalization backed by a clear contract |

Ask what a label *means*, not merely whether it exists.

### Identity labels

Use the smallest stable set that uniquely identifies the entity the operator
selects. Too few labels merge different entities. Too many labels fragment one
entity whenever incidental metadata changes.

Distinguish the semantic instance type from the complete key needed to identify
one instance. A database label remains the owner of a table, but
`{database, table}` may both be required in `instances.by_labels` when table
names repeat across databases. The context still represents table instances;
the ownership path makes each table unambiguous and preserves the identity
lattice.

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

For example, raw disk-throughput series may identify `{disk, operation}`. If
`disk` is the monitored entity and `operation` is the bounded read/write aspect,
use one chart instance per disk and one dimension per operation. The dashboard
then counts disks, not disk-operation pairs.

Distinguish a closed enum from a configuration-dependent set:

- Use explicit selector/value dimensions when authoritative exporter semantics
  define a stable closed set and deliberate names/order matter.
- Use `name_from_label` when values can grow with configuration, workload, or
  exporter version.
- One dump showing two values does not prove a two-value enum. Hard-coding those
  values silently drops future values into autogen/unmatched behavior.

#### Optional dimension labels

An exporter may make a valid dimension label optional through configuration or
version. Establish that optionality from authoritative configuration or source;
`name_from_label` by itself is not evidence that the label can be absent. The
label MAY still be a dimension when it remains a bounded aspect of the same
entity, but it MUST NOT be the only route by which that metric can materialize
in the chart.

Preserve one context and instance type with two mutually exclusive routes:

```yaml
dimensions:
  - selector: 'disk_io_bytes_total{operation=~".*[^[:space:]].*"}'
    name_from_label: operation
  - selector: 'disk_io_bytes_total{operation!~".*[^[:space:]].*"}'
    name: unclassified
```

- The positive route creates the dynamic dimensions when the label is present
  and contains at least one non-whitespace character.
- The negative route catches missing, empty, or whitespace-only values and
  creates one fixed fallback dimension in the same chart.
- The fallback name MUST describe missing classification, not `total`: in a
  mixed population it contains only the unlabeled subset.
- The fallback name SHOULD be outside the label's authoritative value domain.
  A dynamic/static name collision maps both populations to one dimension.
- Do not use `.+` for the positive route. Chartengine trims and rejects
  whitespace-only dynamic names after selector matching, so that matcher would
  select a series that neither route can materialize.
- Do not add a broad static selector beside the dynamic selector in the same
  chart. Labeled series would match both and the chart would double-count them.

If the optional label identifies an entity rather than an aspect, this pattern
does not reconstruct that entity. Use `instances.optional_by_labels` when the
same source legitimately has a base identity without the label and a refined,
operator-useful identity when it is present. One series routes to one identity;
do not duplicate base and detailed views merely to expose both levels. If the
missing label would collapse non-additive gauge states, preserve a complete
identity or use a source-defined aggregate; optional identity does not make an
invalid reduction correct.

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

### Aggregation when labels are omitted

`instances.by_labels` and `instances.optional_by_labels` select the labels that
form chart-instance identity. They do not relabel source series or delete other
labels. When multiple selected series render to the same chart and dimension,
the chart's `aggregation` reducer combines their raw values.

Use that behavior deliberately:

- **Counters and histogram components:** `sum` is the correct rollup across
  disjoint populations of the same counted or measured quantity. Keep the chart
  algorithm incremental by omitting the override; the Netdata Agent, not the
  collector or profile, computes rates and handles counter resets.
- **Point-in-time gauges:** `sum` is correct only for a source-defined additive
  stock. Use `avg` only when an unweighted typical value is meaningful, and
  `min`/`max` only for an extremum or deliberate state reduction. Otherwise
  preserve the complete semantic identity.
- **Status-like gauges:** `min` and `max` can answer “all”/“any” questions for
  source-proven 0/1 states. Status categories themselves remain dimensions.
- **Summary quantiles:** No supported reducer creates a global quantile from
  per-source quantiles. Preserve the source identity.
- **No collector-side deltas:** A profile or collector MUST NOT precompute
  counter deltas, rates, or reset adjustments to make aggregation appear safe.
  That duplicates and conflicts with the Agent's incremental algorithm.
- **No meaning-based censorship:** Whether a label names a user, tenant, API
  key, endpoint, route, or another deployment-owned entity does not decide
  whether it is retained. Source semantics, mathematical correctness, operator
  usefulness, stability, cardinality, lifecycle, and resource cost do.

For exporters with configurable label contracts:

1. Use required identity for labels that must exist to define the chosen entity.
2. Use optional identity when one view should refine from a base chart to a
   stable per-value chart only when the label exists.
3. When an optional label is a bounded **dimension**, use complementary dynamic
   and `unclassified` routes in the same chart.
4. Keep non-additive gauges at their complete emitted identity when no reducer
   preserves meaning.
5. Do not duplicate a coarser chart solely because Netdata can already obtain
   that view by grouping the finer chart.

Record every intentional aggregation in the operator model and prove it with a
fixture containing at least two series that differ only on an omitted label. The
expected chart must contain the selected reducer's result. Also prove
representative gauges remain separate when any identity label differs.

### Promoted labels

Promotion is metadata, not identity. Use it for stable attributes that help the
operator filter or explain a chart. It does not make a missing identity label
available, and it cannot recover labels from writer-skipped `_info` metrics.

Ownership and descriptive-detail labels are valid promoted metadata only when
they are functionally stable for the chosen instance. If one purported serial
number, database owner, or display name varies across the dimensions of one
instance, the source evidence contradicts that classification: refine the
identity/aspect model instead of promoting an arbitrary value.

## Compose charts around one comparison

Metrics belong together only when all of these are true:

1. They answer one operator question.
2. Their complete rendered units mean the same thing, including the counted or
   measured object.
3. Their chart algorithm is compatible.
4. Their scale allows each signal to remain visible.
5. Their dimension cardinality remains readable.

Shared units are necessary, not sufficient. Requests/s and errors/s are related,
but a tiny error rate can disappear under large traffic. Used bytes and queue
bytes are both bytes, but they may describe unrelated resources.

Do not use an umbrella noun to make unlike units appear compatible. Batches/s,
records/s, retries/s, and bytes/s do not become a common `events/s` or
`operations/s` axis. The noun identifies the observation population and is part
of dimensional correctness. If the exact noun is unknown, research it; if the
nouns differ, split the chart even when the counters advance together.

An operation counter is not the same unit as a counter of objects produced by
that operation. Combining them under the object's unit silently assumes one
object per operation; combining them under `events/s` hides the same false
assumption. Keep both near their causal owner, but on honest axes.

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

### One signal spans several capabilities

Do not solve overlap by creating application-wide `Latency`, `Throughput`,
`Parameters`, or `Resources` drawers. Place a stage-specific signal with that
stage. Place a true end-to-end signal with the nearest common lifecycle owner.
When one chart deliberately compares stages, put it at their nearest common
domain owner and say what the comparison diagnoses.

Why: the operator usually starts from the affected operation, then follows its
causes. A global signal-role branch reverses that reasoning and forces the
operator to remember which lines belong to which stage.

### Same metric, multiple entity levels

Do not force one identity policy across all uses. Place charts under the entity
level they represent and scope defaults narrowly. A root `chart_defaults`
identity is harmful when descendants include global and per-entity series.

### Rich label, unbounded cardinality

Prefer a bounded aggregation already exported. Otherwise choose a truthful
chart reducer or preserve identity when the detail is operator-useful. If the
source-owned family has no useful bounded view, exclude it through source-backed
profile relabeling and record the lost question. Job relabel/drop is reserved
for deployment policy. Do not turn arbitrary values into dimensions and rely on
lifecycle caps to make the chart usable.

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

A profile cannot chart a series filtered or rejected before `metrix` ingestion. Decide the
job policy and profile together. An exclusion is correct only when losing that
signal is intentional and explained; `metrics:` declaration is never a drop or
coverage mechanism.

## Choose chart types for visual meaning

Use chart type as visual semantics, not decoration:

- `line`: rates, counts, latency/quantiles, ratios, state, and most time-varying
  signals. Lines preserve independent trends and crossings.
- `area`: an intentionally filled magnitude where the area itself helps answer
  the operator question, including a deliberate mirrored in/out view.
- `stacked`: dimensions that form an exact disjoint, exhaustive, additive
  partition of one whole. Additive categories are not automatically a useful
  composition view.
- `heatmap`: histogram bucket distribution. Bucket charts are forced to heatmap
  by the compiler; omit the redundant type in stock YAML.

Use `line` by default. Units alone cannot establish whether fill or composition
is truthful: request outcomes may form a valid partition while unrelated rates
may share units. The semantic design must state the intended relationship. The
validator warns on every explicit area/stacked choice so that relationship is
reviewed rather than guessed from unit words.

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

Perform the same dimensional analysis for time counters, then verify when the
source contributes the time. Process CPU seconds accumulate while the process
runs and therefore become used CPU cores. Histogram duration sums usually
increase only when observations complete; their incremental value is completed
work time in `seconds/s`, not the live population that was active during the
interval. Use a current Gauge for titles such as “in progress” or “in flight.”

Avoid conversions chosen only to make lines look similar. If two dimensions
need unrelated scale manipulation, they probably do not belong on one axis.

## Order the operator journey

Set chart `priority` deliberately when section order is part of the operator journey.
Omit it for unrelated charts; the runtime default remains `70000`. YAML position is
still not an ordering contract.

At the application level, order domain capabilities by the operator's causal
journey. Within each capability, a useful local order is health/workload →
failures/latency → queueing/resource pressure → detailed internals. Do not turn
that local diagnostic sequence into application-wide `Workload`, `Latency`, or
`Resources` branches.

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
- **Application-wide Workload/Latency/Errors/Distributions/Parameters
  families:** expose diagnostic lenses or observation form instead of the
  capability that owns the signal, forcing operators to join one causal story
  across multiple branches.
- **Repeated identical sibling family names:** render as the same navigation
  path, so repetition does not express distinct semantic owners. Give the
  branches domain names or nest them under their actual capability.
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
- **Priority on every chart:** obscures which few navigation boundaries truly
  require ordering and creates unnecessary maintenance. Set it only where it
  supports the operator journey.
- **Titles derived mechanically from metric names:** repeat exporter syntax
  instead of explaining operational meaning.
- **Passing the validator as definition of quality:** proves runtime behavior for
  evidence, not the semantic usefulness of the dashboard.

## Semantic review

After objective validation passes, review the dashboard design:

- Does the first screen answer the application's most urgent operator questions?
- Does navigation express the entities/containment, modules/capabilities, and
  operations/processing stages that operators actually use?
- For each first- and second-level family, is the answer to “what do operators
  expect to see about this?” coherent without relying on a shared signal role,
  metric form, parameter kind, or unit?
- Does each owner keep its available workload, outcomes, errors, latency,
  saturation, capacity, and resources close enough for holistic diagnosis?
- Is each context homogeneous in entity type?
- Does each displayed leaf family contain one effective entity identity?
- Does descendant identity retain its parent labels?
- Are identity labels minimal, stable, and filterable?
- Are dimensions bounded comparable aspects?
- Do sibling families share the intended parent identity?
- Does every chart satisfy question, units, algorithm, scale, and cardinality?
- Does every shared axis preserve the exact counted/measured noun, rather than
  hiding unlike objects under `events`, `operations`, `items`, or
  `observations`?
- Are rare failures visible beside high-volume traffic?
- Are titles and units mathematically true?
- Is each exclusion justified by the operator question lost?
- Is file order coherent for review without being presented as UI ordering?

Warnings demand reasoning, not mechanical edits. Record why the design is
intentional or change it when the UX consequence is wrong.
