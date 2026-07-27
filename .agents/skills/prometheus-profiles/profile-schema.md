# Prometheus profile schema and runtime effects

This file is a profile-specific navigation aid. The authoritative profile
envelope is `src/go/plugin/go.d/collector/prometheus/profile-format.md`; the
authoritative embedded chart schema is
`src/go/plugin/framework/charttpl/README.md`. Read both in full before
authoring. A Prometheus profile embeds one chart-template **group**, not a full
standalone chart-template spec.

## Contents

- [Profile envelope](#profile-envelope)
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

template:
  family: Exporter
  context_namespace: exporter
  metrics: [exporter_up]
  charts:
    - title: Availability
      context: availability
      units: state
      priority: 100
      dimensions:
        - selector: exporter_up
          name: up
```

- `match` is REQUIRED and uses Netdata simple patterns against scraped
  Prometheus **family base names**. It is an exporter-detection signature, not
  a coverage declaration or dimension selector.
- Prefer exporter-unique family patterns in `match`. Generic runtime families
  such as `process_*`, `python_*`, and `http_*` may be charted without being
  part of detection; including them can make unrelated endpoints eligible.
- Exact-mode validation forces the candidate by name, so its `PASS` does not
  prove that automatic profile selection is unique or safe.
- `app` is OPTIONAL and must match the profile-name syntax. The resolved job app
  becomes the application segment in contexts. Precedence is configured job
  `app`, then the first selected profile `app`, then job name.
- `template` is REQUIRED and must contain at least one chart. Its value is a
  recursive `charttpl.Group`.
- The file basename is profile identity. Use lowercase letters, digits, and
  underscores, starting with a letter.
- Unknown keys fail strict decoding.

The collector supplies the full chart-template `version`, `engine`, and
`prometheus[.<app>]` context namespace. Do not put a standalone-spec `version`,
top-level `groups`, or `engine` beside `match` in a profile.

## Groups are hierarchy and scope

A group can contain:

```yaml
family: Requests
context_namespace: requests
metrics: [exporter_requests_total]
chart_defaults:
  label_promotion: [region]
  instances:
    by_labels: [service]
charts: [...]
groups: [...]
```

Each field affects a different part of the dashboard:

- `family` composes with ancestor families using `/`. This is navigation, so use
  application functions and entity levels rather than metric types.
- `context_namespace` composes with `.` into chart contexts. It is identity, not
  merely display text.
- `metrics` authorizes exact metric names for dimension selectors in the group
  and descendants. It does **not** route, chart, keep, or drop a series.
- `chart_defaults` reduces repetition. The nearest group default wins; a
  chart-local value replaces it. Lists and objects are replaced, not merged.
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
`dimension`. This skill additionally requires an explicit positive `priority`.

Optional fields include:

- `id`: base chart ID; otherwise derived from `context` by replacing dots with
  underscores.
- `family`: a chart-level family leaf.
- `algorithm`: `absolute` or `incremental`, applied to every dimension in that
  chart.
- `type`: `line`, `area`, `stacked`, or `heatmap`.
- `label_promotion`, `instances`, and `lifecycle`.

Algorithm omission is inference, not neutrality:

- `*_total`, `*_count`, `*_sum`, and `*_bucket` infer `incremental`.
- other names infer `absolute`.
- mixed counter-like and gauge-like selectors are ambiguous and fail unless the
  chart states an algorithm.

Set `algorithm` explicitly when suffix and semantics disagree or when a reader
should not have to reverse-engineer the choice. Do not force metrics with
incompatible units/semantics into one chart merely because an explicit
algorithm makes compilation possible.

Priority behavior:

- omitted or non-positive priority becomes `70000`;
- YAML position does not become runtime priority;
- the planner materializes chart IDs in sorted ID order, not profile order.

Therefore every chart needs an explicit positive priority. Keep source order
aligned with intended presentation and never decrease priority as the profile
proceeds. Unique increasing values are usually the clearest total order, while
intentional ties remain a design choice because runtime placement then falls
back to chart-ID ordering.

Presentation invariants enforced by the validator:

- Every chart needs at least one visible dimension. Hidden dimensions MAY
  support a visible comparison but MUST NOT make the entire chart invisible.
- A chart selecting a histogram `_bucket` MUST use
  `units: observations/s`. It MUST use `algorithm: incremental` or omit the
  algorithm so suffix inference selects incremental, and SHOULD declare
  `type: heatmap` explicitly even though the compiler forces that runtime type.
- Discrete work/event, count, state, and time units MUST use `line`.
  `area`/`stacked` require physical volume, space, bandwidth, or I/O semantics.

## Instances, dimensions, and labels

### `instances.by_labels`

- No `instances` means one aggregate chart instance.
- Explicit labels create one chart per unique combination and are required on
  every routed series.
- `*` uses every label in identity; future exporter labels can therefore create
  new chart identities and cardinality.
- `!label` excludes a key; excludes win regardless of order.
- An `instances` block needs at least one positive token.

Use instance labels for the entity the operator selects: server, database,
table, backend, queue, endpoint, and so on. If a required identity label is
absent, the series does not route to that chart. Different raw identity values
may also normalize to the same chart ID, so validate observed values.

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

A missing `name_from_label` value makes that series unroutable. Dynamic
dimension labels should describe comparable aspects, not unbounded entities;
otherwise one chart becomes an unreadable cardinality sink.

`options` supports integer `multiplier`, `divisor`, `hidden`, and `float`.
Conversions affect values but do not change source semantics. Use a negative
multiplier only for a deliberate mirrored visualization, such as inbound versus
outbound traffic.

### Label promotion

Promoted labels are mutable chart metadata for filtering/grouping; they are not
identity and do not split chart instances. Promote stable descriptive labels.
Do not expect promotion to rescue an `_info` family that the Prometheus writer
never materializes.

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
is not a flattened scalar. A summary base exists only when the writer accepts a
summary with quantiles. See `metric-types.md` for rejection and design behavior.

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

The current profile schema has no profile-level `drop:` and no computed
`average` dimension. Use job selector/relabeling for exclusion. Chart only
values the collector actually writes; do not document or emit speculative
schema keys.
