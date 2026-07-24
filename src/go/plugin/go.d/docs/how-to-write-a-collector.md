# How to Write a go.d Collector (V2)

This is the canonical starting point for new go.d collectors. New collectors MUST use framework V2. V1 collectors remain
in the tree for compatibility and maintenance only.

For migrating an existing V1 collector, use `src/go/plugin/go.d/docs/migrate-v1-to-v2.md` instead. Migration is
compatibility work and has different rules from new collector authoring.

Use `src/go/plugin/go.d/collector/cato_networks/` as the primary modern example. It is large, so copy the pattern, not
the whole shape. The useful references are called out below by responsibility.

## Before Writing Code

Do the design work first:

1. Read the upstream API or protocol docs. Do not infer current behavior from memory or from generated SDK types alone.
2. Check existing helper packages before implementing parser, HTTP, selector, command-execution, SQL, ping, log-reading,
   or log-limiting plumbing. Start with `src/go/plugin/go.d/docs/helper-packages.md`.
3. You MUST aim for the clean end state, not the smallest initial diff. If the clean collector design requires a
   framework improvement, surface that as a design decision and follow
   `src/go/plugin/framework/docs/changing-framework-code.md` instead of hiding it behind collector-local glue.
4. Decide the monitored entities and cardinality bounds. If one job collects remote resources that SHOULD be separate
   Netdata nodes, design V2 host scopes from the start.
5. Decide the minimal public config surface. Public config is a compatibility contract. Use constants for internal
   tuning such as page limits, scan cadence, retry limits, fan-out concurrency, and cache TTLs unless the operator has a
   real decision to make. A proposed config option MUST name that concrete operator decision; "operators may want to
   tune it" is not enough.
6. Decide whether the collector needs Functions or topology. Functions are interactive live/snapshot views; metrics are
   time series. New topology producers MUST use `src/go/pkg/topology/v1` and validate against
   `src/plugins.d/FUNCTION_TOPOLOGY_SCHEMA.json`.
7. Plan collector consistency using `.agents/skills/integrations-lifecycle/consistency.md`. Generated integration pages
   and README symlinks are outputs, not hand-authored sources.
8. Plan the first coherent batch and its boundaries. At each boundary, you MUST re-check whether new work has drifted
   out of scope; defer it or land it independently before continuing.

## Source References

Primary V2 reference:

- `src/go/plugin/go.d/collector/cato_networks/`

Read these files by responsibility:

- `collector.go`: registration, defaults, public lifecycle methods, `MetricStore()`, `ChartTemplateYAML()`, and Function
  wiring.
- `config.go`: config defaults, normalization, validation, and intentionally small public config.
- `collect.go`, `collect_metrics.go`, `collect_bgp.go`: collection orchestration and split domain operations.
- `metrix.go`, `write_metrics.go`, `charts.yaml`: typed instruments, metric writes, chart template, `StateSet`,
  `instances.by_labels`, and `label_promotion`. Audit every `instances.by_labels` identity choice instead of copying
  labels from the example blindly.
- `host_scope.go`: deterministic per-site V2 host scopes/vnodes.
- `func_deps.go`, `catofunc/`: Function subpackage boundary behind a narrow dependency interface.
- `topology_store.go`, `topology.go`, `topology_test.go`: immutable topology snapshot publishing and topology v1 schema
  validation.
- `config_test.go`, `collector_lifecycle_test.go`, `collector_collect_test.go`, `charts_test.go`: table-driven V2 tests
  and fixture validation.

Framework/API references:

- `src/go/plugin/framework/collectorapi/collector.go`
- `src/go/plugin/framework/docs/changing-framework-code.md`
- `src/go/plugin/go.d/docs/helper-packages.md`
- `src/go/pkg/metrix/README.md`
- `src/go/plugin/framework/charttpl/README.md`
- `src/go/plugin/framework/chartengine/README.md`
- `src/go/plugin/framework/functions/README.md`
- `src/go/tools/functions-validation/README.md`
- `.agents/skills/project-writing-go-modules-framework-v2/go-v2-host-scope.md`
- `.agents/skills/integrations-lifecycle/consistency.md`

## File Layout

Start with this layout and add focused files only when a responsibility needs its own boundary:

```text
src/go/plugin/go.d/collector/<name>/
|-- collector.go          # registration, New, public lifecycle, store/template
|-- init.go               # Init helper methods for clients/matchers/state
|-- config.go             # Config, defaults, validation
|-- collect.go            # Collect orchestration
|-- metrix.go             # typed metrix instruments built once in New
|-- write_metrics.go      # normalized state -> metrix observations
|-- models.go             # collector-local state/DTOs
|-- client.go             # API/client boundary
|-- charts.yaml           # V2 chart template
|-- config_schema.json    # DYNCFG schema
|-- metadata.yaml         # integration metadata source
|-- taxonomy.yaml         # dashboard TOC placement source
|-- integrations/         # generated integration page
|-- README.md             # symlink to generated integration page
|-- testdata/             # fixtures and config serialization files
`-- *_test.go             # table-driven tests
```

Common optional splits:

Most collectors need none of these optional files. Add one only when the collector has the corresponding product surface
or state boundary; do not create empty `host_scope.go`, `topology.go`, or `<name>func/` files just because Cato has
them.

- `init.go` when `Init()` needs helper setup for clients, matchers, caches, or other persistent state. Keep the public
  `Init()` method itself in `collector.go`; let it call focused helpers such as `initClient()` or `initSiteSelector()`.
- `collect_<operation>.go` when the collector has multiple distinct collection operations, such as discovery, account
  metrics, BGP, or inventory.
- `normalize_<operation>.go` when API payload normalization would otherwise dominate `collect.go`.
- `host_scope.go` when the collector emits generated vnodes.
- `<name>func/` plus `func_deps.go` when the collector exposes Functions.
- `topology.go` and `topology_store.go` when the collector emits topology.

Avoid files whose names hide their responsibility. For example, a file named `diagnostics.go` SHOULD NOT contain only
error classification.

## Registration And Lifecycle

New collectors MUST implement `collectorapi.CollectorV2` from `src/go/plugin/framework/collectorapi/collector.go` and
register via `CreateV2`. In practice, `collector.go` should:

- embed `config_schema.json` for `JobConfigSchema`;
- embed `charts.yaml` for `ChartTemplateYAML()`;
- expose `Config: func() any { return &Config{} }`;
- return a new collector from `CreateV2`;
- add `Methods` and `MethodHandler` only when the collector has Functions.

`New()` SHOULD own defaults and test seams:

- create `metrix.NewCollectorStore()`;
- build typed instruments once from that store;
- set default config values;
- set injected seams such as client factories or clocks;
- create the Function router when Functions exist.

Public lifecycle and framework-contract methods MUST stay in `collector.go`:

- `Configuration() any`
- `Init(context.Context) error`
- `Check(context.Context) error`
- `Collect(context.Context) error`
- `Cleanup(context.Context)`
- `MetricStore() metrix.CollectorStore`
- `ChartTemplateYAML() string`

Collectors that need a long-running side-effect loop MAY additionally implement `collectorapi.CollectorV2Runner` with
`Run(context.Context) error`. Use this only when work must start with the running job lifecycle but must not wait for
the next globally aligned `Collect()` tick, such as an agent-wide Function state refresh. The runtime starts `Run()`
only after the job starts, never during autodetection or DynCfg `test`, cancels it on stop, and waits for it before
`Cleanup()`. The implementation MUST return promptly after `ctx.Done()` and SHOULD make in-flight I/O cancellation-aware
where the underlying library allows it.

`Init()` validates config, prepares matchers/clients, and initializes persistent state. Explicit setup details SHOULD
live in helper methods, preferably in `init.go`, so the public method reads as the lifecycle sequence. `Check()` MUST be
a cheap auth/connectivity probe, not a full collection. `Collect()` MUST run the real write path through `metrix`.
`Cleanup()` closes idle connections and forwards Function cleanup.

## Config

Config SHOULD stay small and operator-oriented:

- connection identity and credentials;
- endpoint and standard HTTP/TLS/proxy fields when applicable;
- `update_every`, `timeout`, and `vnode` when relevant;
- selectors that let users intentionally scope cardinality.

Implementation tuning SHOULD use constants:

- discovery refresh cadence;
- page sizes and maximum pages;
- per-cycle fan-out concurrency;
- cache TTLs;
- retry/backoff internals;
- API batching constraints.

You MUST NOT add a config option just because it is easy to expose. Once shipped, it is hard to remove and MUST stay
synchronized across `Config`, `config_schema.json`, stock `.conf`, metadata, generated docs, and tests. A proposed
config option MUST name the concrete operator decision it enables; "operators may want to tune it" is not enough.

For SaaS/API credentials, examples SHOULD prefer secret indirection such as `${env:COLLECTOR_API_KEY}` or
`${file:/run/secrets/collector_api_key}` instead of realistic-looking inline credentials. Schema fields that carry
secrets MUST be marked sensitive and use password-style UI handling where the schema supports it.

Selectors SHOULD use existing matcher packages such as `src/go/pkg/matcher` unless the upstream API forces a different
grammar. Document the exact matching input, for example "site name when present, otherwise site ID."

## Collect Flow

`Collect()` SHOULD stay orchestration, not a large parser. A typical flow is:

1. ensure the client is initialized;
2. refresh stable discovery only when needed;
3. fetch the current snapshot/state needed for this cycle;
4. enrich with optional or slower data;
5. normalize API payloads into collector-local state;
6. publish any immutable Function/topology snapshot;
7. write metrics to `metrix`.

When the collector performs several upstream calls or collection operations, those operations SHOULD be split into
focused files named by operation, for example `collect_metrics.go` or `collect_bgp.go`. `collect.go` SHOULD explain the
cycle; the operation files SHOULD own the operation-specific API calls, fail-soft behavior, and merge rules.

Fail-soft behavior MUST be used only when partial data is still truthful. If one optional operation fails, log a
rate-limited warning and omit or preserve only values that remain honest. If the core operation fails, return an error
with context.

`Collect()` MUST preserve context cancellation. If the context is canceled during a partial path, `Collect()` MUST
return the context error so the runtime aborts the cycle instead of committing a stale or partial frame.

## Metrics And Charts

Metric instruments SHOULD be built once in `New()` when the metric surface is known. Use a typed collector metrics
struct so write code is a value mapping, not repeated dynamic instrument lookup.

Use the right instrument:

- `SnapshotGaugeVec` for labeled current values; use scalar `SnapshotMeter.Gauge` only when the metric is intentionally
  unlabeled;
- `Counter.ObserveTotal()` for source counters;
- `StateSet` SHOULD be used for fixed mutually exclusive states, such as connected vs disconnected or up vs down.

`charts.yaml` is the chart contract. Every template MUST define:

- `version: v1`;
- `context_namespace`;

Templates SHOULD also group charts by operational area, use `instances.by_labels` for stable instance identity when
charts are entity-scoped, use `label_promotion` for descriptive labels that should not define uniqueness, and keep the
default lifecycle unless a concrete reason exists to override it.

Metric labels and chart instance labels MUST be bounded and stable. Use IDs for identity. Mutable display names SHOULD
be promoted with `label_promotion`. Do not blindly copy `instances.by_labels` from Cato or any other example; audit
every label used for chart identity and record why it is stable enough for that collector.

## Host Scopes And Vnodes

Use `metrix.HostScope` when one job emits data for remote entities that SHOULD appear as separate Netdata nodes. Labels
alone are not enough for that product semantics.

Rules:

- `ScopeKey` and `GUID` are deterministic and based on stable IDs.
- Hostname may use a human-readable name when safe, with stable fallback.
- Add `_vnode_type=<source>` and useful source labels.
- Route every metric for that remote entity through the same host scope.
- Keep the default host scope empty unless the metric truly belongs to the agent/job host.

Use `.agents/skills/project-writing-go-modules-framework-v2/go-v2-host-scope.md` for the framework contract.

## Functions

Functions MUST NOT freely access collector internals. Put Function code in a dedicated subpackage, for example
`<name>func/`, with a narrow `Deps` interface declared by that subpackage.

Non-topology Function responses MUST conform to `src/plugins.d/FUNCTION_UI_SCHEMA.json`; topology Function responses
MUST conform to `src/plugins.d/FUNCTION_TOPOLOGY_SCHEMA.json`. Function payloads MUST be validated with
`src/go/tools/functions-validation/` or an equivalent schema-validation test, and the validation method must be
recorded.

Pattern:

- collector package owns state and implements a small adapter in `func_deps.go`;
- Function package owns method IDs, router, handlers, presentation, and tests;
- Function package MUST import only framework/function types and other allowed dependencies, not the collector package;
- the Function package `Deps` interface MUST expose only the methods the Function needs and MUST NOT expose, return, or
  embed `*Collector`;
- `Collector.Cleanup()` forwards cleanup to the Function router.

Use `catofunc/` as the primary example. Test the Function package with fake deps so the boundary is compile-enforced.

## Topology

New topology producers MUST use `src/go/pkg/topology/v1`. The non-v1 root `src/go/pkg/topology` payload model has been
retired and MUST NOT be reintroduced for topology payloads.

Rules:

- build topology from normalized collector state, not directly from raw API payloads;
- publish immutable snapshots for Function readers;
- MUST NOT mutate a published topology value;
- use `src/go/plugin/go.d/collector/cato_networks/topology.go` as the concrete construction reference for actors, links,
  detail tables, and telemetry fields;
- MUST validate topology payloads in tests with both `topologyv1.ValidateDecodedData` and
  `src/plugins.d/FUNCTION_TOPOLOGY_SCHEMA.json`; see `src/go/plugin/go.d/collector/cato_networks/topology_test.go`
  `validateCatoTopologyV1Data` for the full marshal/decode/schema check shape;
- follow `.agents/skills/project-create-topology/SKILL.md` for actor/link/table design.

## Repository Wiring

For a new collector `<name>`:

1. Add the collector package under `src/go/plugin/go.d/collector/<name>/`.
2. Import it in `src/go/plugin/go.d/collector/init.go`.
3. Add the default stock config: `src/go/plugin/go.d/config/go.d/<name>.conf`.
4. Add the module toggle in `src/go/plugin/go.d/config/go.d.conf`.
5. Add or update `src/go/plugin/go.d/README.md`.
6. Add health alerts under `src/health/health.d/<name>.conf` only when alerts are useful and backed by emitted chart
   contexts.
7. If adding or changing service-discovery rules under `src/go/plugin/go.d/config/go.d/sd/` or `sdext`, update generated
   service-discovery documentation through the integrations lifecycle recipe.
8. Generate `integrations/<slug>.md` and the README symlink from `metadata.yaml`. Single-integration collector
   directories normally use the symlinked README. Multi-integration plugin directories may keep a hand-authored umbrella
   README; follow `.agents/skills/integrations-lifecycle/consistency.md`.

Use `.agents/skills/integrations-lifecycle/recipes/add-go-collector.md` for the integration-generation commands and
taxonomy pipeline details.

The PR description or design note MUST enumerate the relevant collector consistency artifacts and justify every artifact
that did not need a matching change. Most of this is not CI-enforced; it must be reviewer-visible.

## Tests

Tests SHOULD be table-driven with `map[string]struct{}` when cases share setup and assertion shape.

Recommended test coverage:

- config JSON/YAML serialization with `collecttest.TestConfigurationSerialize`;
- config validation, including required credentials and unsafe URLs;
- `Init`, cheap `Check`, `Collect`, `Cleanup`, `MetricStore`;
- hard failure, partial failure, context cancellation, and recovery behavior;
- chart-template schema validation with `collecttest.AssertChartTemplateSchema` and chart-template compile validation
  through the chartengine path used by nearby V2 collectors;
- post-collect chart coverage with `collecttest.AssertChartCoverage`;
- state-set values for every known state and unknown fallback;
- host-scope routing when scopes/vnodes are used;
- Function handler tests with fake deps when Functions exist;
- topology schema validation when topology exists;
- fixture validity and attribution when fixtures come from public third-party projects.

Do not let tests depend on real credentials or live services unless the test is explicitly an integration test gated
outside the default unit-test path.

## Validate Locally

From `src/go`, run the narrow collector tests:

```bash
go test -count=1 ./plugin/go.d/collector/<name>/...
```

Verify that go.d can load the module:

```bash
timeout 15s go run ./cmd/godplugin -m <name> -d
```

Success means the module is registered, a job starts, and the command keeps running until the timeout stops it. Treat
`unknown module`, `no jobs started`, config-load errors, or an immediate exit before the timeout as failures. Use
`-c <config-dir>` when the test config lives outside the normal go.d config search path.

When the collector uses concurrency or Functions, also run:

```bash
go test -race -count=1 ./plugin/go.d/collector/<name>/...
```

When integration metadata, generated pages, taxonomy, or health alerts change, run the relevant integrations pipeline
checks from `.agents/skills/integrations-lifecycle/`.

Do not claim full-project validation from a narrow collector command. State exactly what was run.

## Anti-Patterns

- New collector using `Collect() map[string]int64`.
- Full live collection from `Check()`.
- Starting operational background polling from `Check()` or `Init()`; use the optional V2 runner hook when polling must
  be tied to the running job lifecycle.
- Public config knobs for internal implementation details.
- Custom selector or retry framework when existing package/framework behavior is enough.
- Collector-local singleton, adapter, or glue code that substitutes for a missing shared framework capability.
- Per-cycle warning/error logs for recoverable partial failures.
- Metric charts for collector internals when logs are enough.
- Mutable names used as chart or vnode identity.
- Function package holding `*Collector`.
- New topology producer using legacy topology payloads.
- Hand-written `README.md` when the integration page should be generated.
