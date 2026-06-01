# Migrating go.d Collectors From V1 To V2

Requirement language follows the root `AGENTS.md` definitions.

This guide is for migrating an existing go.d collector from framework V1 to
framework V2. It is not the starting point for a new collector; use
`src/go/plugin/go.d/docs/how-to-write-a-collector.md` for new work.

V1 collectors are public integrations. A migration MUST preserve their existing
user-visible contracts unless the user explicitly approves a breaking change.

## Core Rule

A V1-to-V2 migration is compatibility work first. The clean end state is a V2
collector that behaves like the old collector from the user's point of view,
with the old public contracts preserved and the internal collection path moved
to `collectorapi.CollectorV2` and `metrix.CollectorStore`.

Do not combine a compatibility migration with new enrichment, new topology,
new host scopes, config expansion, chart redesign, or framework changes unless
that work is required for the migration itself. If the migration reveals useful
new work, split it into a later batch.

## Before Writing Code

Create a compatibility manifest before implementation. The manifest can live in
the active TODO or SOW. It MUST cover:

1. Module identity.
   - collector directory;
   - module name;
   - chart-template `context_namespace` and any group context namespaces;
   - `go.d.conf` toggle;
   - stock job config path;
   - service-discovery rules, if any, including
     `src/go/plugin/go.d/config/go.d/sd/` and
     `src/go/plugin/go.d/discovery/sdext/` references.
2. Registration and lifecycle.
   - current `collectorapi.Register` entry;
   - `Defaults`;
   - `Init`, `Check`, `Collect`, and `Cleanup` behavior;
   - any `Once`, reconnect, cache, or retry behavior.
3. Config contract.
   - YAML and JSON keys;
   - defaults;
   - validation;
   - `config_schema.json`;
   - stock `.conf`;
   - DYNCFG behavior.
4. Metric/chart contract.
   - chart IDs;
   - contexts;
   - dimension IDs and names;
   - algorithms;
   - units;
   - title, family, type, priority, multiplier, divisor, hidden, and float
     flags;
   - labels;
   - chart variables (`Vars`);
   - dynamic chart/instance generation rules;
   - chart lifecycle and obsoletion timing.
5. Integration artifacts.
   - `metadata.yaml`;
   - `taxonomy.yaml`;
   - health alerts;
   - generated integration page and README symlink;
   - `COLLECTORS.md` / plugin README entries when affected;
   - service-discovery docs such as `SERVICE-DISCOVERY.md`, when affected;
   - secrets docs such as `SECRETS.md`, when affected.
6. Tests and fixtures.
   - existing tests to preserve;
   - missing contract tests to add before or during migration;
   - real fixture coverage.

## What Must Change

A V2 migration MUST replace the V1 collection path:

- registration uses `CreateV2`;
- the collector implements `collectorapi.CollectorV2`;
- `New()` creates `metrix.NewCollectorStore()`;
- the collector stores `metrix.CollectorStore`;
- `Configuration()` preserves existing config return behavior;
- `VirtualNode()` preserves existing vnode behavior when the V1 collector has
  one;
- `MetricStore()` returns the store;
- `ChartTemplateYAML()` returns embedded `charts.yaml`;
- `Collect(ctx)` returns `error` and writes observations to `metrix`;
- the completed migration removes the V1 `Collect() map[string]int64` output
  path and any runtime bridge from V1 maps into V2 `metrix`.
- the completed migration removes `Charts()` and runtime `collectorapi.Charts`
  mutation from the production collection path.

Use `src/go/plugin/framework/collectorapi/collector.go` as the source of truth
for the interface.

## Parity During Development

Temporary V1 logic can be useful while developing the migration. For example,
tests can compare V1 map output against V2 `metrix` observations after mapping
both sides through the V1 chart manifest and the V2 chart identity snapshot.

That parity bridge is a development tool only. It MUST NOT remain in the final
migrated collector runtime path. A finished migration that runs as
V1-to-bridge-to-V2 is not a clean end state.

Runtime path means any code reachable from `Init(ctx)`, `Check(ctx)`,
`Collect(ctx)`, or `Cleanup(ctx)` during normal execution. V1 collection logic
MUST NOT remain reachable from that runtime path, regardless of function names
or return types.

If parity helpers are useful long term, keep only `_test.go` helpers, test-only
packages, or fixtures that do not compile into or participate in runtime
collection.

## What Must Stay Stable

Unless the user approves a breaking change, the migration MUST preserve:

- module name and job identity;
- config field names and defaults;
- chart contexts;
- chart IDs;
- dimension IDs and names;
- dimension algorithms;
- chart title, family, type, priority, units, multiplier, divisor, hidden, and
  float flags;
- chart variable semantics used by health alerts;
- health alert lookups;
- metadata metric descriptions and units;
- source metadata content that drives generated integration docs;
- taxonomy coverage and CI behavior;
- service-discovery behavior;
- vnode behavior;
- user-facing lifecycle behavior.

If an existing collector has an accidental bug or inconsistent artifact, record
it separately. Fix it in the migration only when preserving the bug would make
the V2 collector incorrect or untestable; otherwise split the fix into its own
tracked batch or TODO item.

## Implementation Shape

Prefer the same file ownership as new V2 collectors:

```text
collector.go          # registration, New, public lifecycle, MetricStore, ChartTemplateYAML
init.go               # Init helper methods when setup is non-trivial
config.go             # Config, defaults, validation
collect.go            # Collect orchestration
collect_<area>.go     # separate upstream operations when there are several
metrix.go             # typed instruments built once
write_metrics.go      # observations into metrix
charts.yaml           # V2 chart template
*_test.go             # focused, table-driven tests
```

Keep public lifecycle methods in `collector.go`. Helper methods can move into
focused files when that makes ownership clearer.

## Lifecycle Rules

- `Init(ctx)` MUST perform setup and validation only. It MUST NOT collect the
  full metric set just to initialize state.
- `Check(ctx)` SHOULD be a cheap probe when the upstream API supports one. If
  V1 used full collection for autodetection, preserve the user-visible result
  while making the V2 path as light as the source allows.
- `Collect(ctx)` MUST write observations to `metrix` and return an error only
  when the cycle should abort. Fail-soft partial collection must be deliberate,
  tested, and logged without per-cycle spam.
- `Cleanup(ctx)` MUST preserve existing cleanup behavior and release any V2
  Function or client resources added by the migration.

## Metrics And Charts

Use typed `metrix` instruments and `charts.yaml`.

- Prefer creating instruments once when the metric surface is stable. Some
  collectors intentionally build instruments in the collection path when the
  surface is dynamic; if you keep that pattern, record why it is still the clean
  V2 shape for that collector.
- Use `StateSet` for fixed one-active-state values.
- Use `Counter.ObserveTotal()` when the upstream value is a source counter.
- Put multipliers, divisors, hidden flags, float formatting, `instances`, and
  `label_promotion` in `charts.yaml`, not ad hoc runtime chart code.
- Preserve V1 dimension algorithms exactly unless the old algorithm was wrong
  and the user approves the change.

When adding labels during migration, verify they are bounded and do not alter
chart identity unexpectedly. Labels that improve filtering are acceptable only
when they do not break existing chart/dimension contracts.

### Chart Variables

V1 `collectorapi.Chart.Vars` have no direct `charts.yaml` / `charttpl` support
today. Some shipped health alerts depend on those variables.

If the V1 collector uses `Vars`, the migration MUST NOT silently drop them.
Choose one of these paths before implementation:

- add clean framework support by following
  `src/go/plugin/framework/docs/changing-framework-code.md`;
- preserve the alert semantics through an approved equivalent design;
- get explicit user approval for a breaking alert change and update health,
  metadata, generated docs, and release notes accordingly.

### Obsoletion Timing

V1 collectors often obsolete dynamic charts immediately with `MarkRemove()` /
`MarkNotCreated()`. V2 chart templates expire unseen chart instances through
`lifecycle.expire_after_cycles` and related chartengine policy. Exact immediate
V1 timing is not always reproducible in V2.

For every V1 dynamic chart, record the old obsoletion timing and choose the V2
lifecycle policy deliberately. If the timing changes, document the behavioral
change and get approval when it affects alerts or user-visible chart lifetime.

### Dynamic IDs And Contexts

V1 collectors often build chart IDs with `fmt.Sprintf`. V2 templates derive
contexts from `context_namespace`, group context namespaces, and chart context
leaves. V2 autogen also has `engine.autogen.max_type_id_len` behavior.

The migration MUST prove that generated chart IDs, contexts, and dimensions
match the old public contract, or explicitly record and approve any difference.

## Config Rules

Migrations MUST keep existing YAML and JSON field names. Do not rename config
keys to match new code style.

Do not add public config options as part of a migration unless they are required
to preserve existing behavior. Internal tuning SHOULD use constants. New user
choices belong in a later feature batch with schema, stock config, metadata,
and docs updated together.

`autodetection_retry`, `update_every`, and `vnode` are job/runtime fields in
many existing collectors. Preserve the migrated collector's current YAML/JSON
behavior and keep `config.go`, `config_schema.json`, stock config, and metadata
consistent. Do not copy another collector's schema treatment for these fields
without checking current framework expectations.

## Host Scopes, Functions, And Topology

Host scopes, Functions, and topology are product design choices, not automatic
migration side effects.

- If the V1 collector already has vnode behavior, preserve it.
- If adding host scopes would be useful but is not required for compatibility,
  split it into a later product decision.
- If the collector exposes Functions, isolate Function code in a dedicated
  `<name>func/` package behind a narrow `Deps` interface.
- New topology producers MUST use `src/go/pkg/topology/v1` and validate against
  `src/plugins.d/FUNCTION_TOPOLOGY_SCHEMA.json`.

## Tests

Migration tests MUST prove compatibility and V2 behavior.

At minimum:

- config YAML/JSON serialization compatibility;
- `Init`, `Check`, `Collect`, and `Cleanup` lifecycle coverage;
- metric-store cycle behavior, including `BeginCycle`, successful commit, and
  abort on expected hard collection errors;
- golden chart-identity snapshot coverage for chart IDs, contexts, dimension
  IDs/names, algorithms, units, type, family, priority, multiplier, divisor,
  hidden, float flags, label promotion, and lifecycle policy;
- chart template schema/decode/validate/compile coverage;
- chart coverage for fixture data expected to materialize all dimensions;
- health alert compatibility when alerts exist: each alert `on:` context must
  exist in the compiled template, and every variable referenced by alert
  calculations must still be provided by a dimension, variable-equivalent
  design, or approved alert change;
- generated integration artifact consistency when metadata/taxonomy changes;
- taxonomy coverage checks when chart contexts change;
- host-scope tests if scopes/vnodes are preserved or introduced.

`collecttest.AssertChartCoverage` is not a replacement for the golden
chart-identity snapshot. It verifies that emitted series and the template agree;
it can still pass when both writer and template were renamed consistently.

Prefer table-driven tests using `map[string]struct{}` keyed by case name when
cases share setup and assertion shape.

## Validation

Run the narrowest commands that prove the changed contract. Typical migration
validation includes:

```bash
cd src/go
go test -count=1 ./plugin/go.d/collector/<name>/...
go test -race -count=1 ./plugin/go.d/collector/<name>/...
```

Also run framework or integration checks when the migration touches those
contracts:

- chart template/framework changes:
  `go test -count=1 ./plugin/framework/charttpl ./plugin/framework/chartengine`
- `metrix` changes:
  `go test -count=1 ./pkg/metrix/...`
- runtime/framework changes:
  follow `src/go/plugin/framework/docs/changing-framework-code.md`
- metadata/taxonomy/generated docs:
  follow `.agents/skills/integrations-lifecycle/consistency.md`

Record exactly what ran. Full validation MUST NOT be claimed from a narrow
command.

## Commit Shape

Prefer small coherent commits:

1. Add missing compatibility tests, if needed.
2. Move registration and collection path to V2.
3. Convert charts to `charts.yaml` while preserving chart identity.
4. Update synchronized integration artifacts.
5. Add follow-up enrichment only in a separate batch.

If the migration requires a framework change, stop and follow
`src/go/plugin/framework/docs/changing-framework-code.md` before implementing
collector-local glue.

## Anti-Patterns

- Rewriting chart IDs, contexts, or dimensions only because the V2 template
  makes a new name easier.
- Adding labels, host scopes, topology, or Functions in the same commit as the
  compatibility migration without a product decision.
- Shipping a V1-to-bridge-to-V2 runtime path after the V2 store is in place.
- Hiding framework gaps in collector-local helpers.
- Treating generated integration pages or README symlinks as authoring sources.
- Claiming compatibility without a manifest and tests.
