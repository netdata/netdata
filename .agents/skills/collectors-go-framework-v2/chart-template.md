# Chart Template Authoring

## Scope

The `charts.yaml` of a go.d V2 collector is the source of truth for what operators see: contexts, dimensions,
families, ordering and chart labels. This file owns how to write one; the format and its defaults are owned by
`src/go/plugin/framework/charttpl/README.md`, runtime behavior by `src/go/plugin/framework/chartengine/README.md`, the
required top-level keys by `src/go/plugin/go.d/docs/how-to-write-a-collector.md#metrics-and-charts`, and the metric
model by `docs/NIDL-Framework.md`. A template built at runtime follows the same rules (`./SKILL.md#metrics-and-charts`).

## Say Only What Differs From The Framework

- A template MUST NOT restate a framework default. Readers take every key as a deliberate deviation; a file that
  spells out defaults hides the real decisions. Defaults and their owners: `lifecycle`
  (`src/go/plugin/framework/chartengine/lifecycle_defaults.go`), chart `type` (`defaultChartType` in
  `src/go/plugin/framework/charttpl/defaults.go`), `algorithm` (resolved from the runtime metric kind, see
  `./SKILL.md#metrics-and-charts`), chart `id` (derived from the composed context), `priority` (engine default) and
  `label_promotion` (omitted means automatic intersection).
- Compose contexts through group `context_namespace` (`redfish` -> `drive` -> `nvme` -> `busy_time`); a chart states
  only its own leaf and omits `id`. Repeating the namespace or the chart ID per leaf is a smell left by generators.
- Put `instances` defaults on the section that most leaves share and override only the leaves that differ.

## Families Are Operator Navigation

- Family paths (`Storage/Drives/NVMe`) are the sidebar tree. Name every level in operator words (`Processors`,
  `Temperature Readings`), never in identifiers (`processor`, `reading_temperature`).
- All charts of one job form one family tree. Two groups with the same composed family merge, and a bare family that
  equals a section name lands inside that section. Give shared or generic groups their own section.
- Choose leaf granularity by what an operator searches for: a leaf name SHOULD be the word they look for
  (`Fans`, `Voltage Readings`), the tree SHOULD stay scannable when a section is expanded, and granularity SHOULD be
  consistent within a section. A leaf with one chart is fine when its name is the search term; a leaf MAY hold charts
  and sub-families when the sub-family is the explicit jump (`Drives` plus `Drives/NVMe`). Clicking a leaf is cheaper
  for the operator than scrolling a long one.
- Distinguish measurement leaves from resource-status leaves when both exist for the same physical thing
  (`Fans` holds fan resource status, `Rotational Speed Readings` holds the RPM readings), so the leaf says what it
  contains.
- Reserve `Overview` for the whole-system resources an operator checks first; do not park cross-domain groups there
  (a control loop that mixes thermal and power measures is its own section).

## Ordering

- Set `chart_defaults.priority` on top-level sections only, in the order operators should meet them, and set nothing
  below. Leaves and charts inside a section then follow their names, which the dashboard sorts alphabetically for
  equal priorities. Per-chart priorities encode file order, break on every insertion and are not how the fleet's V2
  templates are written.
- Pick section values in the collector range conventions of `src/collectors/all.h`; step them so a section can be
  inserted later.

## Statesets

- A stateset chart has one dimension with a bare selector and no `name`; chartengine creates one dimension per
  declared state. Do not enumerate the states in the template.
- The compiler accepts an unnamed selector only for a metric whose name ends in `_state`, `_status` or `_mode`
  (`supportsRuntimeInferredDimension` in `src/go/plugin/framework/chartengine/compiler.go`; `chartengine.Compile`
  returns an error otherwise). Name stateset metrics that way; the other V2 collectors already do.
- Inferred dimensions are created in alphabetical order, not declared order (`sortInferredDimensions` in
  `src/go/plugin/framework/chartengine/planner.go`). Only the dimension set is the contract; a chart that needs a
  semantic order must keep explicit names.
- A chart whose dimensions are separate gauges (one metric per severity, counting conditions) is not a stateset and
  keeps its explicit dimension names.

## Values

- The float flag belongs on the instrument (`metrix.WithFloat` in `src/go/pkg/metrix/options.go`), decided where the
  value is produced; chartengine inherits it, so `options.float` in a template only restates it. Multiplier, divisor
  and `hidden` are presentation and stay in the template.
- Aggregation and algorithm overrides follow `./SKILL.md#metrics-and-charts`.

## Labels

- Decide the attached label set at the source, not in `label_promotion`: attach a label only when an operator filters
  or groups charts by it (`manufacturer`, `model`), needs it to place the component (`slot`, `location`), or needs it
  to read a shared context (`reading_type`, `reading_role`). Inventory and diagnostics belong in a Function, not on
  every chart.
- A collector MUST NOT attach labels the framework or the agent already stamps: `_collect_job` (reserved in
  `src/go/plugin/framework/chartemit/apply.go`) and `_collect_module` (added by the agent from the chart's module,
  `src/database/rrdset-index-id.c`). A label that always equals another label is dead.
- Identity labels (`instances.by_labels`) are always on the chart; listing them in `label_promotion` is noise. When
  the attached set is the wanted set, omit `label_promotion` and let the automatic intersection promote it.
- `metadata.yaml` documents the attached set per scope (`.agents/skills/collectors-metadata-yaml/metrics.md`); keep an
  exported list of the keys in the collector and a test that ties list, code and documentation together.

## Shared Contexts

- A collector MUST NOT emit contexts owned by the Agent host's collectors (`system.*`, definitions under
  `src/collectors/common-contexts/`) unless its data describes the host the Agent runs on. A job that polls a remote
  target (a BMC, a device, a cloud API) publishes under its own namespace; node placement is the vnode's job
  (`./go-v2-host-scope.md`).
- Reusing a shared context also requires the same units, the same dimension names and states and equivalent labels
  as the other producers, because the dashboard merges every producer of a context into one chart.

## Tests

- Validate the template: `collecttest.AssertChartTemplateSchema` plus `charttpl.DecodeYAML` and `chartengine.Compile`.
- Compare artifacts with each other instead of restating them in Go: `collecttest.AssertMetadataDocumentsChartTemplate`,
  `collecttest.AssertHealthAlertsTargetChartTemplate` and `collecttest.AssertMetadataAlertsMatchHealthConfig`
  (`src/go/plugin/go.d/pkg/collecttest/artifacts.go`). A test that names a context, a dimension list or a family pins
  whatever the template said when it was written and MUST NOT exist.
- `collecttest.AssertChartCoverage` derives the expected charts and dimensions from the template; do not add
  `RequiredContexts` that repeat it.
- To prove a template refactor changes nothing, write every metric definition once into a store, prepare a
  chartengine plan and diff its actions before and after, normalizing only the field you meant to change.

## Review Checklist

- No restated defaults, no per-chart priorities, no explicit IDs equal to the derived one, no enumerated states.
- Families read as operator navigation; sections carry the only priorities.
- Labels attached at the source are the documented set; no framework labels re-added.
- No host-owned contexts from a remote-target collector.
- Go tests validate and cross-check artifacts; none restates template content.
