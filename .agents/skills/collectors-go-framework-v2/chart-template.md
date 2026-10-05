# Chart Template Authoring

## Scope

The `charts.yaml` of a go.d V2 collector is the source of truth for what operators see: contexts, dimensions,
families, ordering and chart labels. This file owns how to write a new or reworked template. A V1-to-V2 migration
preserves everything listed under "What Must Stay Stable" in `src/go/plugin/go.d/docs/migrate-v1-to-v2.md` and uses
these rules only where that document leaves a choice. Prometheus chart profiles use the same template keys under their
own rules (`.agents/skills/collectors-prometheus-profiles/SKILL.md`).

Owners this file relies on:

| Fact | Owner |
|---|---|
| Template format, defaults, inheritance, selectors | `src/go/plugin/framework/charttpl/README.md`, `src/go/plugin/framework/charttpl/defaults.go` |
| Runtime chart creation, lifecycle, label promotion | `src/go/plugin/framework/chartengine/README.md` |
| Required top-level keys, instrument choice, wiring | `src/go/plugin/go.d/docs/how-to-write-a-collector.md#metrics-and-charts` |
| Metric model and family design | `docs/NIDL-Framework.md` |
| Runtime contracts for algorithm, aggregation, identity | `./SKILL.md#metrics-and-charts`, `./SKILL.md#chart-label-identity` |
| Artifact drift checks | `src/go/plugin/go.d/pkg/collecttest/artifacts.go` |

Before the template: decide what is measured and what each value means
(`.agents/skills/collectors-go-design/SKILL.md#measurement-truth-table`), bound cardinality
(`.agents/skills/collectors-authoring/collector-practices.md#110-cardinality-discipline`), and choose instance
identity (`./SKILL.md#chart-label-identity`). A template built at runtime follows the same rules
(`./SKILL.md#metrics-and-charts`).

Terms: a *section* is a top-level family segment (a root group with a `family`, or a child of a transparent root);
a *leaf* is the innermost family that holds charts.

## Contexts

- **One namespace, set once.** Set the collector's context root once as the spec-level `context_namespace` next to
  `version: v1`; group namespaces only append segments below it (`redfish` -> `drive` -> `nvme` -> `busy_time`).
  Only the spec-level value prefixes autogen contexts (`src/go/plugin/framework/charttpl/README.md`), so per-group
  copies of the root leave unmatched metrics outside the namespace, and a missing root lets a group named after
  another collector emit into that collector's contexts. Use a root other than the module name only when the
  collector deliberately shares one with a sibling collector.
- **Derived ids.** Omit chart `id`; chartengine derives it from the composed context with dots replaced by
  underscores (`src/go/plugin/framework/chartengine/compiler.go`). An existing collector whose
  ids differ from the derived value keeps them: chart ids are a public contract
  (`.agents/skills/collectors-authoring/collector-practices.md#13-ids-are-public-contracts`).
- **What is a contract.** Contexts, chart ids, dimension names and instance labels are public. Metric names in the
  store and family paths are internal or display metadata and MAY be renamed when a V2 template is reworked; a
  migration preserves them.

## Say Only What Differs From The Framework

- A template MUST NOT restate a framework default; readers take every key as a deliberate deviation. The defaults
  and their owners:
  - `lifecycle`: `src/go/plugin/framework/chartengine/lifecycle_defaults.go`; deviate only for observed churn
    (`./SKILL.md#chart-label-identity`).
  - chart `type`: `defaultChartType` in `src/go/plugin/framework/charttpl/defaults.go`.
  - `algorithm`: resolved from the runtime metric kind (`./SKILL.md#metrics-and-charts`).
  - `id`: derived from the composed context.
  - `priority`: the engine default (`chartengine.Priority`); see Ordering for the one allowed explicit use.
  - `label_promotion`: omitted means automatic intersection.
- Put `instances` defaults on the section most leaves share and override only the leaves that differ.
- Titles are copied verbatim into `metadata.yaml` as the metric description; write them as operator-facing titles.

## Families Are Operator Navigation

- **Model.** How to choose sections and leaves, how fine a leaf may be, and what `Overview` holds are owned by
  `docs/NIDL-Framework.md#step-2-organize-into-families`.
- **Composition.** Family segments join with `/` (`composeFamily` in `src/go/plugin/framework/chartengine/compiler.go`).
  All charts of one job share one wire type (`TypeID` in `src/go/plugin/framework/jobruntime/job_v2.go`), so they form
  one family tree: two groups with the same composed path are the same node, and a bare family equal to a section name
  becomes part of that section. The dashboard renders the `/`-separated path as a tree; that rendering is not owned
  by this repository.
- **Measurement versus status.** When a physical thing has both a resource-status leaf and a readings leaf, the leaf
  names MUST say which is which (`Fans` for fan resource status, `Rotational Speed Readings` for the RPM readings).
- **Cross-domain groups.** A group that mixes domains (a control loop with thermal and power measures) gets its own
  section rather than a place in `Overview` or in either domain.

## Ordering

- Ordering is optional. When the alphabetical order of section names is not the order operators should meet them,
  set `chart_defaults.priority` on top-level sections only. Set nothing below a section and never per chart.
- Start from the engine default (`chartengine.Priority`, also `collectorapi.Priority`) and step sections so one can
  be inserted later; the C-plugin ranges of `src/collectors/all.h` do not apply to go.d. The first section MAY state
  the default explicitly as the anchor of the ladder; that is the documented reset value, not a restated default.
- Inside a section, charts with equal priority follow their names in the dashboard (dashboard behavior, not owned by
  this repository), so leaf and chart names carry the order. Per-chart priorities encode file order and break on
  every insertion.

## Statesets

- **Inferred dimensions.** A stateset dimension is a bare selector with no `name`; chartengine creates one dimension
  per declared state. Enumerate states only when the chart needs a semantic order or different names; the state label
  key is the metric name, so the selector is `<metric>{<metric>="<state>"}` (`src/go/pkg/metrix/reader_flatten.go`).
- **Compiler rule.** chartengine infers dimension names only for selectors it can name at runtime: histogram
  buckets (`le`), summary quantiles (`quantile`), metrics whose name ends in `_bucket`, `_state`, `_status` or
  `_mode`, and metrics whose last dot-segment is exactly `state`, `status` or `mode`
  (`supportsRuntimeInferredDimension` in `src/go/plugin/framework/chartengine/compiler.go`; `chartengine.Compile`
  returns an error otherwise). Name stateset metrics with one of the state suffixes.
- **Order.** Dimensions with a literal `name` keep their declared order and come first; inferred and
  `name_from_label` dimensions follow sorted by name
  (`orderedDimensionNames` and `lessDynamicDimension` in `src/go/plugin/framework/chartengine/planner_lifecycle.go`).
  An ordered vocabulary (`ok, warning, critical`) therefore renders alphabetically when inferred. Prefer inference and
  accept that order; keep explicit names only where the band order carries meaning the operator reads.
- **Shape.** A one-hot stateset SHOULD be `type: stacked`, so the active state reads as the filled band; collectors
  that already ship stateset charts as `line` are not required to change.
- **Not a stateset.** A chart whose dimensions are separate gauges (one metric per severity, counting conditions)
  keeps explicit dimension names.

## Values

- The float flag belongs on the instrument (`metrix.WithFloat` in `src/go/pkg/metrix/options.go`), decided where the
  value is produced. When `options.float` is still needed is owned by `src/go/plugin/framework/charttpl/README.md`;
  the effective flag is the OR of instrument flag, `options.float` and `avg` aggregation
  (`src/go/plugin/framework/chartengine/matcher.go`).
- Multiplier, divisor and `hidden` are presentation and stay in the template.
- Aggregation and algorithm overrides follow `./SKILL.md#metrics-and-charts`.

## Labels

- **Decide at the source.** Attach a label to a metric only when an operator filters or groups charts by it
  (`manufacturer`, `model`), needs it to place the component (`slot`, `location`), or needs it to read a shared chart
  (`reading_type`, `reading_role`). Inventory and diagnostics belong in a Function, not on every chart.
- **Never re-add stamped labels.** A collector MUST NOT attach `_collect_job` (reserved, dropped if supplied and
  stamped by `src/go/plugin/framework/chartemit/`), `_collect_plugin` or `_collect_module`
  (added by the agent in `rrdset_update_permanent_labels`, `src/database/rrdset-index-id.c`). A label whose value
  always equals another label's value is dead.
- **Identity is not promotion.** `instances.by_labels` labels are always on the chart; listing them in
  `label_promotion` is noise. When the attached set is the wanted set, omit `label_promotion`; list labels only to
  promote a strict subset, or `[]` to promote none. Promotion keeps a label only while every series routed to the
  chart carries the same value for it (`./SKILL.md#chart-label-identity`), which matters when several readings share
  one chart.
- **Document the set.** `metadata.yaml` documents the attached labels per scope
  (`.agents/skills/collectors-metadata-yaml/metrics.md`). Export the key lists from the collector and tie them to the
  code and the documentation with tests (`measurement.ResourceLabelKeys` and `TestMetadataDocumentsChartLabels` in
  `src/go/plugin/go.d/collector/redfish/` show the shape).

## Shared Contexts

- A collector MUST NOT emit contexts owned by the Agent host's collectors (`system.*`, definitions under
  `src/collectors/common-contexts/`) unless its data describes the host the Agent runs on. A job that polls a remote
  target (a BMC, a device, a cloud API) publishes under its own namespace; node placement is the vnode's job
  (`./go-v2-host-scope.md`). The one-namespace rule above makes this checkable in one line.
- Reusing a shared context requires the same units, dimension names and states and equivalent labels as the other
  producers: the agent merges title, family, units, type and priority of every instance of a context into one
  definition (`rrdcontext_merge_with` in `src/database/contexts/rrdcontext-context.c`), and the model requires
  identical dimensions across instances (`docs/NIDL-Framework.md`).

## Tests

- **Validate.** `collecttest.AssertChartTemplateSchema` (`src/go/plugin/go.d/pkg/collecttest/chart_template_schema.go`),
  then `charttpl.DecodeYAML` and `chartengine.Compile`. Native sets validate through `chartengine.NewTemplateSet`;
  follow `src/go/plugin/framework/chartengine/README.md#named-active-template-sets` for runtime membership changes.
- **Cross-check artifacts.** `collecttest.AssertMetadataDocumentsChartTemplate`,
  `collecttest.AssertHealthAlertsTargetChartTemplate`, `collecttest.AssertMetadataAlertsMatchHealthConfig` and
  `collecttest.AssertHealthAlertsMatchMetadata` (`src/go/plugin/go.d/pkg/collecttest/artifacts.go`) compare
  metadata.yaml, health.d and the template with each other. The `...With` variants take options: `HealthAlertsCheck`
  narrows a shared health.d file by context prefix (and, for the metadata comparison, selects the module by `meta.id`);
  `MetadataAlertsCheck` selects the module and accepts a shared health.d file whose other alerts this module does not
  document. The metadata check compares stateset dimensions in declared order, not rendered order.
- **Never restate an artifact.** A test that names one of the template's own contexts, dimension lists or families, or
  pins an alert's thresholds, lookup, cadence or recipients, restates what the artifact said when it was written and
  MUST NOT exist; health.d owns alert policy. An independent oracle, such as a V1 parity manifest during a migration or
  upstream alert rules with a recorded mapping, is not a restatement.
- **Coverage.** `collecttest.AssertChartCoverage` (`src/go/plugin/go.d/pkg/collecttest/chart_coverage.go`) derives the
  expected charts and dimensions from the live static or native provider and store, per host scope; do not add
  `RequiredContexts` that repeat it.
- **Refactor proof.** To show a template change alters nothing, a throwaway local test writes every metric definition
  once into a store, builds the plan with `chartengine.New`, `Engine.LoadYAML` and `Engine.PreparePlan`, and diffs the
  plan actions before and after, normalizing only the field the change was meant to affect. No shipped helper does
  this.

## Review Checklist

- One spec-level `context_namespace`; every context starts with the collector's root; no explicit `id` equal to the
  context with dots replaced by underscores; no restated default.
- Families read as operator navigation; sections carry the only priorities, if any.
- Statesets use inferred dimensions unless order or naming needs explicit ones; state metrics carry a state suffix.
- Labels attached at the source are the documented set; no stamped labels re-added.
- No host-owned contexts from a remote-target collector.
- Go tests validate and cross-check artifacts; none restates template or alert content.
