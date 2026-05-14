---
name: project-writing-go-modules-framework-v2
description: Use when creating or migrating a Go go.d collector to framework V2, touching CollectorV2, metrix.CollectorStore, ChartTemplateYAML/charts.yaml, charttpl/chartengine, V2 host scopes/vnodes, or V2 collector tests. Focuses on concise maintainer-preferred V2 collector patterns.
---

# Writing Go go.d Modules With Framework V2

Use with `project-writing-collectors`. Keep this skill loaded for style; read
source files for evidence.

## Read First

- Contract: `src/go/plugin/framework/collectorapi/collector.go`
- Runtime/chart lifecycle: `src/go/plugin/framework/chartengine/README.md`
- Template format: `src/go/plugin/framework/charttpl/README.md`
- Host scopes/vnodes: `.agents/sow/specs/go-v2-host-scope.md`
- Closest examples:
  - `src/go/plugin/go.d/collector/azure_monitor/` for dynamic scopes/profiles.
  - `src/go/plugin/go.d/collector/ping/` for the smallest V2 shape.
  - `src/go/plugin/go.d/collector/mysql/` for migration compatibility.
  - `src/go/plugin/go.d/collector/powervault/` and `powerstore/` for remote
    discovery, labels, and chart templates.

## Core Style

- Register with `CreateV2`; expose `Config: func() any { return &Config{} }`.
- `New()` owns defaults, `metrix.NewCollectorStore()`, and test seams.
- Store `metrix.CollectorStore`; implement `MetricStore()`.
- Implement `ChartTemplateYAML()`; prefer embedded `charts.yaml`.
- `Collect(ctx)` returns `error` and writes metrics to `metrix`; it does not
  return a V1 `map[string]int64`.
- Keep files boring: `collector.go`, `collect.go`, `metrics.go`,
  `charts.yaml`, focused domain helpers, focused tests.

## Metrics And Charts

- Create instruments once when the metric surface is known.
- Use `store.Write().SnapshotMeter("")` for normal metrics.
- Use `Vec(...)` for labels, `Gauge` for current values,
  `Counter.ObserveTotal()` for source counters, and `StateSet` for fixed
  one-active-state values.
- Use stable metric names that `charts.yaml` selects.
- In `charts.yaml`: use `version: v1`, `context_namespace`, `instances.by_labels`,
  `algorithm: incremental` for counters, and `absolute` for gauges.
- Put multipliers, divisors, hidden flags, and float formatting in the chart
  template, not ad hoc chart-emission code.

## Compatibility Rules

- For migrations, first create a compatibility manifest covering chart IDs,
  contexts, dimension IDs/names, labels, config keys, DynCfg schema keys,
  stock config, alerts, docs, and lifecycle behavior.
- Preserve existing public contracts unless the SOW records an explicit breaking
  decision.
- Keep old YAML/JSON field names. Add new config as opt-in when cardinality,
  cost, or user-visible identity could surprise existing users.
- Keep `metadata.yaml`, `config_schema.json`, stock config, health alerts, and
  README synchronized with code.
- Never log raw secrets, DSNs, bearer tokens, or URLs with embedded credentials.

## Hot-Path Logging

- Do not emit `Warningf`/`Errorf` every collection cycle for a recoverable
  partial failure. Use the built-in logger limiter:
  `c.Limit("collector:stable-operation-key", 1, time.Hour).Warningf(...)`.
- Keep limiter keys stable and low-cardinality. Use operation names, not entity
  IDs, labels, URLs, raw errors, or user-controlled values.
- `Once()` is reset by `JobV2.runOnce()`, so it is useful inside one cycle only;
  it is not cross-cycle spam protection.
- Full collection failure should still return an error with context so the job
  retry path handles it. Limit only fail-soft warnings/errors where collection
  continues with partial or stale data.

## Host Scopes

- Use host scopes only after a product decision says the data belongs on a
  generated vnode.
- Keep `ScopeKey` and `GUID` deterministic.
- Add `_vnode_type=<source>` on collector-generated vnodes.
- Bound and document cardinality. Do not create VM/disk/NIC/path/sensor scopes
  by default.

## Tests

At minimum, V2 work needs:

- config YAML/JSON serialization compatibility;
- `Init`, `Check`, `Collect`, and `Cleanup` lifecycle coverage;
- explicit metric-store cycle tests with `BeginCycle`, success commit, and abort
  on expected collection errors;
- chart-template schema/decode/validate/compile coverage;
- chart coverage assertions for fixtures expected to materialize all dimensions;
- host-scope tests when scopes/vnodes are used.

## Pre-PR Check

- No V1 `map[string]int64` collection path remains unless intentionally kept for
  a compatibility bridge.
- Existing public chart/metric/config identity is preserved.
- New labels and scopes are bounded and documented.
- Enrichment is split from the V2 compatibility migration when possible.
