# Prometheus profile schema and runtime effects

This file is a profile-specific navigation aid. The authoritative profile
envelope is `src/go/plugin/go.d/collector/prometheus/profile-format.md`; the
authoritative embedded chart schema is
`src/go/plugin/framework/charttpl/README.md`. Consult the relevant sections when
authoring. A Prometheus profile embeds one chart-template **group**, not a full
standalone chart-template spec.

## Contents

- [Profile envelope](#profile-envelope)
- [Runtime capability and stock policy](#runtime-capability-and-stock-policy)
- [Groups are hierarchy and scope](#groups-are-hierarchy-and-scope)
- [Charts control presentation and identity](#charts-control-presentation-and-identity)
- [Instances, dimensions, and labels](#instances-dimensions-and-labels)
- [Type-dependent metric names](#type-dependent-metric-names)
- [Context and ID consequences](#context-and-id-consequences)
- [Unsupported shortcuts](#unsupported-shortcuts)

## Profile envelope

```yaml
match: 'exporter_*'
app: exporter

fallback_type:
  gauge: [exporter_open_connections]
  counter: [exporter_events]

relabeling:
  - match: exporter_legacy_requests_total
    metric_relabel_configs:
      - source_labels: [__name__]
        regex: exporter_legacy_requests_total
        target_label: __name__
        replacement: exporter_requests_total

template:
  family: Exporter
  context_namespace: exporter
  metrics: [exporter_up]
  charts:
    - title: Availability
      context: availability
      units: state
      dimensions:
        - selector: exporter_up
          name: up
```

- `match` is REQUIRED and uses Netdata simple patterns against scraped
  Prometheus **family base names**. It is an exporter-detection signature, not
  a coverage declaration or dimension selector.
- `match` defines the profile's source-family selection scope. An
  `autogen.selector` rule is evaluated later against the final post-profile
  family name and labels, but it affects fallback only for samples originating
  inside that profile scope. A contributed stock profile MUST remain open to
  unknown future families unless a bounded source contract proves an authored
  alias/normalization or deliberate exclusion.
- `relabeling` is OPTIONAL exporter-owned normalization. It runs after profile
  selection and before template routing. The first applicable selected profile
  normalizer owns each original source family; all selected templates consume
  the resulting shared stream.
- `fallback_type` is OPTIONAL profile-owned classification for source-proven
  untyped scalar families. Patterns match exact post-job, pre-profile family
  names inside the profile's `match` scope. Precedence is declared TYPE, job
  gauge, job counter, first selected profile classification, then implicit
  `_total` counter; gauge wins over counter inside one profile. Classification
  is bound before profile relabeling, so a later rename preserves rather than
  discovers the type. Use the narrowest stable patterns.
- The runtime format supports `autogen.selector.allow`, but contributed profiles
  MUST NOT use it: an allowlist suppresses every unknown family outside the
  list. This syntax remains available only for deployment-owned user policy.
- `autogen.selector.deny` MAY suppress fallback only for exact family base names
  present in the source-complete inventory. Every deny MUST be exercised by a
  `retain_writable_unrendered` proof exclusion owned by the same profile and
  naming the same final family. Wildcard, label-only, open-ended,
  fixture-absent, stale, and cross-profile denies are forbidden. A selector
  containing `{...}` is label-constrained policy, not an exact metric name. The
  collector store retains the samples; only generic charts disappear.
- Current source-complete evidence MUST still produce zero generic fallback and
  zero unmatched series. The validator tests future compatibility separately
  with a synthetic family derived from wildcard `match` scope, so an unknown
  future family remains generic without masking current profile incompleteness.
- Recommended-job selector denies MUST name exact exposition metric names
  present in the source-complete sample inventory.
  Dynamic source-proven alias grammars belong in bounded profile or job relabel
  `drop` rules; inverse `keep`/`keepequal` filters are forbidden for contributed
  profile and job pipelines. A
  non-empty job allow list MUST copy every positive wildcard
  term from `profile.match` into an unconstrained expression, copy the complete
  match expression, or use `*`; finite canaries and label constraints are not
  structural namespace coverage. Label-dependent sample discard MUST use an
  exact source-fixture metric block, and every exact-scope discard rule MUST
  actually drop a fixture sample at its ordered pipeline position. Wildcard
  blocks may discard only from `__name__`,
  with either a finite exact-name regex or one non-empty internal entity key
  between finite exporter prefixes and finite terminal metric suffixes. Every
  finite exact name or prefix/suffix branch MUST drop source-complete fixture
  evidence; open-ended terminal regexes and wildcard `dropequal` are forbidden.
  Before discard, that name MUST remain derived exclusively from the original
  metric name across all earlier relabel rules and blocks. A wildcard block also
  MUST NOT rewrite `__name__` from application labels because the mutation can
  itself rename or invalidate unknown future families. The exact-scope
  exception cannot infer name-only provenance from `labelmap.source_labels`:
  Netdata's relabel runtime ignores that field and maps application-label
  names. The exception applies only before any earlier metric-name write; a
  rewrite can route a future family into an apparently exact later block.
  Dynamic targets are evaluated with their regex and replacement together: a
  finite output language that cannot produce `__name__` is label-only, not a
  metric-name rewrite. Every positive wildcard term in a block that can discard
  or rename a metric MUST yield an in-scope synthetic probe that the ordered
  pipeline actually processes; one harmless term cannot cover another term
  whose exclusions hide every probe. After the pipeline, each primary future
  probe MUST still reach generic fallback or a bounded source-proven authored
  destination, MUST NOT accidentally collide with an authored dimension metric
  name, and MUST retain the identity promised by the declared transformation. Name
  provenance follows reachable ordered paths, including negative terms; a
  provably disjoint exact mutation does not taint a later exact block. Scope
  proof includes the complete glob language, including character classes. A
  subsequent name-only rewrite inherits any application-label provenance in
  its incoming `__name__`; it cannot clear taint.
- A wildcard name-derived `replace` MUST use the same finite exact-name or
  internal-dynamic-key-plus-terminal-suffix grammar and fixture evidence as a
  wildcard name drop. An internal-key rewrite MUST NOT reference the dynamic
  capture in its output; its finite output branches MUST all be authored
  canonical metric names. Before either dynamic rewrite form, an earlier
  `replace` in the same block MUST copy unchanged from `__name__` the capture
  that encloses the entire dynamic entity region into a reserved static
  non-`__name__` label; a nested capture covering only one alternative is
  incomplete. The target MUST be
  absent from source-fixture block inputs and preserved through every reachable
  later rule/block. A later label write sourced only from `__name__` is harmless
  when its regex is disjoint from all possible current names. Canonical outputs
  MUST keep every finite prefix/suffix branch distinct. One rewrite-only form is
  also allowed for a
  source-owned dynamic identity tail:
  `<canonical_name>_<non-empty-identity>` may rewrite
  exactly to `<canonical_name>`, and the source-complete fixture MUST reach that
  branch. An unrelated terminal catch remains open-ended and forbidden.
- Across exact and wildcard blocks, the complete recommended relabel pipeline
  MUST preserve every observed writer-admissible logical identity after normal
  histogram/summary assembly. Distinct source identities MUST NOT converge on
  the same final metric name and labels; label removal and name normalization
  do not aggregate the values they collapse. Writer-rejected samples such as
  non-finite scalars do not participate in this identity proof.
- Every source-fixture sample reached by a metric-name rewrite MUST retain a
  valid non-empty name. Empty/invalid replacement output is an implicit discard,
  not a rewrite; intentional exclusions belong in explicit bounded,
  source-evidenced `drop` rules.
- Ordered name-rewrite reachability MUST preserve literal replacement prefixes
  and suffixes around captures; collapsing every capture-bearing replacement to
  a prefix-only wildcard creates false reachability into disjoint later blocks.
- Prefer exporter-unique family patterns in `match`. Generic runtime families
  such as `process_*`, `python_*`, and `http_*` may be charted without being
  part of detection; including them can make unrelated endpoints eligible.
- Exact-mode validation forces the candidate by name, so its `PASS` does not
  prove that automatic profile selection is unique or safe.
- `app` is OPTIONAL and must match the profile-name syntax. The resolved job app
  becomes the application segment in contexts. Precedence is configured job
  `app`, then the first selected profile `app`, then job name.
- Stock application metadata examples omit job `profiles` and use default
  automatic selection. They also omit job `app` when the selected profiles
  provide one unambiguous identity. Proof descriptors keep explicit supporting
  profiles because isolated validation composition is evidence, not job policy.
- `template` is REQUIRED and must contain at least one chart. Its value is a
  recursive `charttpl.Group`.
- The file basename is profile identity. Use lowercase letters, digits, and
  underscores, starting with a letter.
- Unknown keys fail strict decoding.

The collector supplies the full chart-template `version`, `engine`, and
`prometheus[.<app>]` context namespace. Do not put a standalone-spec `version`,
top-level `groups`, or `engine` beside `match` in a profile.

## Runtime capability and stock policy

The chart-template runtime is a general user-facing format. Stock contributions
use a stricter minimal policy so intent is reviewable and defaults cannot drift:

| Runtime field | Generic capability | Stock contribution policy |
|---|---|---|
| `id` | explicit stable base ID | omit when the context-derived ID is sufficient |
| `priority` | numeric ordering hint, inheritable through `chart_defaults` | set at the nearest group for a deliberately ordered family subtree; use chart-local only for an exception |
| `instances.by_labels: ['*']` | all current labels become identity | use explicit source-backed identity |
| `lifecycle` | best-effort caps and expiry | omit; do not make coverage depend on silent caps |
| `options.float` | force floating wire precision | omit when inherited runtime precision already does so |
| `algorithm` | override runtime kind | omit except for a deliberate semantic override |
| `aggregation` | choose collision reducer | state it for deliberate many-to-one reduction; omit for collision-free routes |
| `type` | line, area, stacked, heatmap | omit default line and derived heatmap; prove explicit area/stacked intent |

These are contribution rules, not parser limitations. Deployment-owned user
profiles may use the full runtime surface when its consequences are intended.

## Groups are hierarchy and scope

A group can contain:

```yaml
family: Requests
context_namespace: requests
metrics: [exporter_requests_total]
chart_defaults:
  priority: 100
  label_promotion: [region]
  instances:
    by_labels: [service]
charts: [...]
groups: [...]
```

Each field affects a different part of the dashboard:

- `family` composes with ancestor families using `/`. This is navigation, so use
  application functions and entity levels rather than metric types.
- The profile template root MAY omit `family` when it is only a transparent
  container and the application already provides that navigation level. Keep a
  meaningful root for reusable instrumentation profiles. Nested groups MUST
  provide `family`; a chart directly under a transparent root MUST provide its
  own chart-level `family`.
- `context_namespace` composes with `.` into chart contexts. It is identity, not
  merely display text.
- `metrics` authorizes exact metric names for dimension selectors in the group
  and descendants. It does **not** route, chart, keep, or drop a series.
- `chart_defaults` reduces repetition. The nearest group default wins; a
  chart-local value replaces it. Lists and objects are replaced, not merged.
  Priority zero is unset/inherit; use explicit `70000` to reset a child subtree
  or chart to engine-default ordering.
- `groups` can nest to any depth. Put a metric at the highest scope that really
  needs it, not automatically at the root.

Why scope matters: a broad root declaration lets unrelated subtrees select the
same series accidentally. A narrow declaration makes the intended ownership
visible and lets the compiler catch cross-subsystem mistakes.

Remove declarations that no authored dimension in their scope uses. They do
not preserve denied or writer-rejected data; they only imply ownership and can
make a stale or excluded family look intentionally covered.

## Charts control presentation and identity

Required chart fields are `title`, `context`, `units`, and at least one
`dimension`. `priority` MAY be inherited or set locally to order charts in the operator journey.

Optional fields include:

- `id`: base chart ID; otherwise derived from `context` by replacing dots with
  underscores.
- `family`: a chart-level family leaf.
- `algorithm`: `absolute` or `incremental`, applied to every dimension in that
  chart.
- `aggregation`: `sum`, `min`, `max`, or `avg`, applied when several source
  series render to the same chart and dimension.
- `type`: `line`, `area`, `stacked`, or `heatmap`.
- `label_promotion`, `instances`, and `lifecycle`.

An omitted algorithm is resolved independently for each rendered dimension from
the matched runtime series kind: counters use `incremental`; gauges and other
kinds use `absolute`. Metric-name suffixes do not select the algorithm. Distinct
dimensions in one chart may therefore use different algorithms.

Set `algorithm` explicitly for an intentional runtime-kind override. The
generic runtime also permits it to resolve differently typed series collapsed
into one rendered dimension, but stock profiles must preserve distinct
gauge/counter dimensions instead. Do not force metrics with incompatible units
or semantics into one chart merely because a chart-wide algorithm compiles.

Priority behavior:

- an omitted or zero priority inherits the nearest nonzero group default;
- an effective non-positive priority after inheritance becomes `70000`;
- an explicit positive priority is preserved to the chart engine;
- YAML position still does not become runtime or UI presentation order.

Omit `priority` for unrelated charts. Set `chart_defaults.priority` at the nearest
group when a profile needs stable family ordering; reserve chart-local priority
for deliberate exceptions.

Presentation rules checked by the profile-validation tool:

- Every chart needs at least one visible dimension. Hidden dimensions MAY
  support a visible comparison but MUST NOT make the entire chart invisible.
- A chart selecting a histogram `_bucket` MUST use
  `units: observations/s`. It MUST use `algorithm: incremental` or omit the
  algorithm so the flattened counter kind selects incremental. Omit
  `type: heatmap` in stock YAML because the compiler derives it.
- Explicit `area` requires deliberate filled-magnitude intent.
- Explicit `stacked` requires an exact disjoint, exhaustive, additive partition
  of one whole. Units alone cannot prove or reject either relationship, so the
  validator emits a semantic-review warning rather than guessing from words.

## Instances, dimensions, and labels

### `instances.by_labels`

- No `instances` means one aggregate chart instance.
- Explicit labels create one chart per unique combination and are required on
  every routed series.
- `*` uses every label in identity; future exporter labels can therefore create
  new chart identities and cardinality.
- `!label` excludes a key; excludes win regardless of order.
- An `instances` block needs at least one positive token.

### `instances.optional_by_labels`

- Optional identity keys are used only when their value is present and
  nonblank. Missing or blank values keep the base chart identity.
- Each present optional key contributes both its key and value to the chart-ID
  suffix, so partial presence remains distinguishable.
- Optional keys are explicit only: `*` and `!label` are invalid. They cannot
  overlap required or excluded keys and cannot accompany `by_labels: ['*']`.
- Optional identity does not duplicate the stream into base and detailed
  charts. One series routes to exactly one identity: base when absent, refined
  when present.
- Use it only for a bounded, sufficiently stable, operator-useful axis. It
  still multiplies chart cardinality and identity churn when values change.

Use instance labels for the entity the operator selects: server, database,
table, backend, queue, endpoint, and so on. If a required identity label is
absent, the series does not route to that chart; the validator reports the
effective inherited identity, selector, and missing label keys directly.
Different raw identity values may also normalize to the same chart ID, so
validate observed values.

A finite or bounded category MAY be instance identity when filtering and
grouping that category is more useful than comparing it as dimensions in one
chart. Python GC `generation` uses `instances.by_labels: [generation]` and one
fixed dimension per chart. Netdata can aggregate those charts for a runtime-wide
view, so do not duplicate the aggregate chart.

The semantic instance type and its complete unique key are not always the same
list of concepts. An ownership label such as `database` does not turn a table
context into a database-table context, but `{database, table}` may both be
required in `by_labels` when table names repeat. Ownership and descriptive
labels that are not required for uniqueness belong in promoted chart metadata.

For required explicit identity, every routed series must provide each required
label. Use selector guards when the metric can also appear without them. For
optional identity, do not add such guards merely to force a detailed view: the
base/refined behavior is the feature's contract.

Treat nested identities as a lattice: a child entity retains its parent's
identity labels and adds the labels required for the narrower entity. Because
group and chart defaults replace lists rather than merging them, a child
`by_labels` override must repeat the parent labels deliberately. Charts that
compose to the same final family path should use one effective identity set;
otherwise the displayed leaf cannot filter as one entity type.

### Dimensions

Every dimension needs a `selector` with an explicit metric name. The metric
must be visible through `metrics` scope. Label matchers narrow the selected
series.

Choose exactly one naming mode:

- `name`: one static dimension name;
- `name_from_label`: a bounded label value becomes the dimension name;
- omit both only when chartengine can infer a histogram bucket (`le`), summary
  quantile (`quantile`), or supported state-like dimension.

A missing or empty `name_from_label` value makes that route unroutable. Dynamic
dimension labels should describe comparable aspects, not unbounded entities;
otherwise one chart becomes an unreadable cardinality sink.

When authoritative exporter source or configuration proves that it can omit a
valid dimension label, the chart MUST cover both label states with mutually
exclusive selectors. Do not infer optionality merely because a chart uses
`name_from_label`:

```yaml
dimensions:
  - selector: 'requests_total{status=~".*[^[:space:]].*"}'
    name_from_label: status
  - selector: 'requests_total{status!~".*[^[:space:]].*"}'
    name: unclassified
```

In the Netdata selector implementation, the positive matcher selects a present
value containing at least one non-whitespace character, and the negated matcher
also selects a missing label. The fixed fallback therefore covers missing,
empty, and whitespace-only values in the same context and chart instance
without overlapping the dynamic dimensions. Do not use `.+` here because
chartengine trims and rejects whitespace-only dynamic names after selector
matching.

- The fallback MUST describe the missing classification; it MUST NOT be named
  `total`, because it is only the unlabeled subset when labeled and unlabeled
  series coexist.
- The fallback name SHOULD be outside the authoritative dynamic value domain.
- A broad static selector MUST NOT accompany the dynamic selector in the same
  chart; the labeled series would enter both dimensions.
- This pattern does not make a missing entity identity available. Use
  `instances.optional_by_labels` when one source legitimately has a stable base
  identity without that key and a refined identity when it is present.
- This pattern does not authorize invalid gauge aggregation. Preserve the
  complete identity for non-additive states.

`options` supports integer `multiplier`, `divisor`, `hidden`, and `float`.
Conversions affect values but do not change source semantics. Use a negative
multiplier only for a deliberate mirrored visualization, such as inbound versus
outbound traffic.

### Label promotion

Promoted labels are mutable chart metadata for filtering/grouping; they are not
identity and do not split chart instances. The field has three states:

- omitted: automatically promote non-identity labels whose value is identical
  across every series contributing to the chart;
- non-empty: promote only listed non-identity labels whose value intersects;
- `[]`: promote no non-identity labels.

An unlabeled contributor makes the promoted intersection empty. Required and
present optional identity labels are always chart identity metadata. Promote
only stable descriptive labels. Do not expect promotion to rescue an `_info`
family that the Prometheus writer never materializes.

### Aggregation

Aggregation is used only when multiple routed series render to the same chart
ID and dimension. It does not reduce scrape/store cardinality.

- `sum`: additive counters, histogram components, or source-defined additive
  gauges;
- `avg`: an unweighted typical value for a non-additive gauge when that mean is
  meaningful;
- `min`/`max`: extrema or a deliberate reduction of status-like 0/1 values.

The default is `sum`. A chart uses one reducer for all dimensions. Summary
quantiles cannot be merged into a global quantile; preserve identity instead.
`avg` is unweighted, and a non-sum reducer over cumulative counters can create
misleading deltas when contributor membership changes. In stock YAML, state an
aggregation explicitly when the design deliberately allows many-to-one
collisions, including deliberate `sum`; omit it for collision-free routes.

### Lifecycle

`max_instances`, `expire_after_cycles`, `dimensions.max_dims`, and
`dimensions.expire_after_cycles` manage dynamic cardinality. Caps are
best-effort and can omit observed entities when undersized. Use them as a
cardinality policy with known consequences, not as a substitute for choosing
the right identity/dimension model. The validator rejects a cap that discards
entities or dimensions present in the supplied dump: a mechanical `PASS` must
not hide current evidence. A cap above observed cardinality still requires
reasoning about configurations and future values absent from the dump.

## Type-dependent metric names

The profile selects the flattened names exposed to chartengine:

- gauge/counter: family name;
- histogram: `<base>_bucket`, `<base>_count`, `<base>_sum`;
- summary: `<base>` for quantiles, plus `<base>_count` and `<base>_sum`.

Declare every flattened name a dimension selector uses. The bare histogram base
is not a flattened scalar. A summary base exists only when the source provides
quantiles; count and sum remain available for a valid quantile-free summary.
See `metric-types.md` for writer and design behavior.

## Context and ID consequences

The effective context is built from:

```text
prometheus.<resolved-app>.<group namespaces...>.<chart context>
```

When the profile root namespace equals the resolved app, the collector removes
that duplicate segment. Context and chart ID are durable identity surfaces:
renaming them creates new chart identities and can leave old metadata/history
to expire separately.

Two templates can derive the same rendered ID even when YAML paths differ. The
whole planner keeps one owner, so validate isolated charts and observed
instance values with the repository validator. Chart IDs, contexts, and
dynamic dimension names are sanitized again at public wire emission. Empty
values and cases where distinct effective values collapse to one wire value
are objective failures; intentional reuse of the same raw context remains a
dashboard-design judgment rather than a blanket uniqueness error.

## Unsupported shortcuts

The profile schema has no separate top-level `drop:` shorthand and no computed
`average` dimension. Use profile `relabeling` for exporter-owned exclusions
after selection; use the job selector or job relabeling for deployment policy
or exclusion needed before selection. Chart only values the collector actually
writes; do not document or emit speculative schema keys.
