---
name: project-writing-go-modules-framework-v2
description: Use when creating or migrating a Go go.d collector to framework V2, touching CollectorV2, metrix.CollectorStore, ChartTemplateYAML/charts.yaml, charttpl/chartengine, V2 host scopes/vnodes, or V2 collector tests. Focuses on concise maintainer-preferred V2 collector patterns.
---

# Writing Go go.d Modules With Framework V2

Use with `project-writing-collectors`. Keep this skill loaded for style; read
source files for evidence.

## Read First

- Contract: `src/go/plugin/framework/collectorapi/collector.go`
- Framework-change workflow:
  `src/go/plugin/framework/docs/changing-framework-code.md`
- Canonical new-collector guide:
  `src/go/plugin/go.d/docs/how-to-write-a-collector.md`
- Helper-package guide:
  `src/go/plugin/go.d/docs/helper-packages.md`
- V1-to-V2 migration guide:
  `src/go/plugin/go.d/docs/migrate-v1-to-v2.md`
- Runtime/chart lifecycle: `src/go/plugin/framework/chartengine/README.md`
- Template format: `src/go/plugin/framework/charttpl/README.md`
- Host scopes/vnodes: `.agents/sow/specs/go-v2-host-scope.md`
- Primary modern example: `src/go/plugin/go.d/collector/cato_networks/`.
  Use focused pieces from it, not the whole collector shape.
- Older V2 collectors can still be useful for local patterns, but review them
  for stale style before treating them as examples.

## Decision Discipline

- You MUST aim for the clean end state, not the smallest collector diff. If a
  framework capability is missing and the problem is general, design the
  framework change instead of hiding the issue in collector-local glue.
- You MUST re-check scope after each coherent batch. If the work reveals an
  independent collector cleanup, framework fix, or integration-doc change,
  either defer it explicitly or land it separately before continuing.

## Core Style

- New collectors MUST implement `collectorapi.CollectorV2` from
  `src/go/plugin/framework/collectorapi/collector.go` and register via
  `CreateV2`.
- `New()` SHOULD own defaults, `metrix.NewCollectorStore()`, typed metric
  instruments, and test seams.
- V2 collectors MUST write metrics through `metrix.CollectorStore` during
  `Collect()` and provide chart template YAML through `ChartTemplateYAML()`;
  embedded `charts.yaml` is RECOMMENDED.
- `Collect(ctx)` MUST return `error` and write metrics to `metrix`; it MUST NOT
  return a V1 `map[string]int64`.
- Files SHOULD stay boring: public lifecycle methods in `collector.go`, setup
  helpers in `init.go` when needed, orchestration in `collect.go`, distinct
  upstream operations in `collect_<operation>.go`, metrics in `metrix.go` /
  `write_metrics.go`, focused tests.
- Before adding custom HTTP, selector, logging, command-execution, SQL, ping,
  or log-file plumbing, check `src/go/plugin/go.d/docs/helper-packages.md` and
  reuse an existing helper when it fits.
- If Functions exist, isolate them in a `<name>func/` subpackage with a narrow
  `Deps` interface declared there. The Function package MUST NOT import the
  collector package or hold `*Collector`.
- Public config options SHOULD stay small and justified. Internal tuning SHOULD
  use constants unless the operator has a real decision to make.

## Metrics And Charts

- Instruments SHOULD be created once when the metric surface is known.
- Use `store.Write().SnapshotMeter("")` for normal metrics.
- Use `Vec(...)` for labels, `Gauge` for current values,
  `Counter.ObserveTotal()` for source counters, and `StateSet` for fixed
  one-active-state values.
- Metric names MUST be stable and selected by `charts.yaml`.
- In `charts.yaml`: use `version: v1`, `context_namespace`, `instances.by_labels`,
  `label_promotion`, `algorithm: incremental` for counters, and `absolute` for
  gauges.
- Put multipliers, divisors, hidden flags, and float formatting in the chart
  template, not ad hoc chart-emission code.

## Compatibility Rules

- For V1-to-V2 migrations, start with
  `src/go/plugin/go.d/docs/migrate-v1-to-v2.md`.
- Temporary V1-to-V2 parity bridges MAY be used during development, but the
  finished collector MUST NOT keep a runtime V1 map-to-`metrix` bridge.
- For migrations, first create a compatibility manifest covering chart IDs,
  contexts, dimension IDs/names, labels, config keys, DynCfg schema keys,
  stock config, alerts, docs, and lifecycle behavior.
- Migrations MUST preserve existing public contracts unless the SOW records an
  explicit breaking decision.
- Migrations MUST keep old YAML/JSON field names. Add new config as opt-in when
  cardinality, cost, or user-visible identity could surprise existing users.
- `metadata.yaml`, `config_schema.json`, stock config, health alerts, and
  README MUST stay synchronized with code.
- MUST NOT log raw secrets, DSNs, bearer tokens, or URLs with embedded
  credentials.

## Hot-Path Logging

- Collectors MUST NOT emit `Warningf`/`Errorf` every collection cycle for a
  recoverable partial failure. Use the built-in logger limiter:
  `c.Limit("collector:stable-operation-key", 1, time.Hour).Warningf(...)`.
- Limiter keys MUST be stable and low-cardinality. Use operation names, not
  entity IDs, labels, URLs, raw errors, or user-controlled values.
- `Once()` is reset by `JobV2.runOnce()`, so it is useful inside one cycle only;
  it is not cross-cycle spam protection.
- Full collection failure SHOULD still return an error with context so the job
  retry path handles it. Limit only fail-soft warnings/errors where collection
  continues with partial or stale data.

## Host Scopes

- Host scopes SHOULD be used only after a product decision says the data belongs
  on a generated vnode.
- `ScopeKey` and `GUID` MUST be deterministic.
- Collector-generated vnodes MUST set `_vnode_type=<source>`.
- Host-scope cardinality MUST be bounded and documented. Collectors SHOULD NOT
  create VM/disk/NIC/path/sensor scopes by default.
- Scope identity MUST use stable IDs. Human-readable names SHOULD be hostnames
  or promoted labels only.

## Tests

At minimum, V2 work SHOULD include:

- config YAML/JSON serialization compatibility;
- `Init`, `Check`, `Collect`, and `Cleanup` lifecycle coverage;
- explicit metric-store cycle tests with `BeginCycle`, success commit, and abort
  on expected collection errors;
- chart-template schema/decode/validate/compile coverage;
- chart coverage assertions for fixtures expected to materialize all dimensions;
- host-scope tests when scopes/vnodes are used.

## Pre-PR Check

- A finished V1-to-V2 migration MUST NOT keep a runtime
  `map[string]int64` collection path or V1 map-to-`metrix` bridge.
- Existing public chart/metric/config identity MUST be preserved unless the SOW
  records an explicit breaking decision.
- New labels and scopes MUST be bounded and documented.
- Enrichment SHOULD be split from the V2 compatibility migration when possible.
