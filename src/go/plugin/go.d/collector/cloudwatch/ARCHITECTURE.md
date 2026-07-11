# AWS CloudWatch Collector Architecture

Maintainer-oriented map of `cloudwatch`, written to be read top to bottom as a
journey: the plain model and the big picture first, then the lifecycle and the
collection cycle, then each stage in detail, and finally the profile schema,
invariants, and where to change things. Per-service metric and chart details live
in the profile YAMLs (`config/go.d/cloudwatch.profiles/`) and the focused tests,
not here.

The one rule that shapes most of what follows:

> `GetMetricData` is billed per query, so the collector queries each metric only
> as often as its period and re-emits cached values in between — cost tracks the
> profiles you run, not how often Netdata collects.

## What It Does

`cloudwatch` is a framework-V2 go.d collector that pulls AWS CloudWatch metrics
for a curated set of AWS services and renders them as dynamic Netdata charts. It
is **profile-driven**: each service is a YAML profile (see Profiles) declaring its
CloudWatch namespace, the dimension set that identifies one instance, the
metrics/statistics to query, and a chart template — so adding or adjusting a
service is usually a YAML edit, not Go.

It does not generate per-account or per-resource nodes: AWS resources are chart
*instances* keyed by `by_labels`. All output follows the job's configured
`vnode`, when present, otherwise the Agent host; the collector creates no host
scopes of its own.

## Big Picture

Config picks the services; the collector discovers what actually exists, queries
it on a per-period schedule to keep the bill down, and renders the results as
charts under the `cloudwatch.` namespace.

```mermaid
flowchart LR
    cfg("Config<br/>credentials · targets · ordered rules")
    disc("Discover<br/>ListMetrics<br/>cheap / free tier")
    plan("Plan + schedule<br/>per target/region/period")
    query("Query<br/>GetMetricData<br/>billed — the cost driver")
    store("metrix store<br/>gauges + labels")
    charts("Dynamic charts<br/>cloudwatch.*")

    cfg --> disc --> plan --> query --> store --> charts

    classDef input fill:#eef1f4,stroke:#8b949e,color:#24292f;
    classDef sched fill:#ece2ff,stroke:#8250df,color:#3b1f6b;
    classDef effect fill:#ffe6cc,stroke:#e36209,color:#5a2a00;
    classDef commit fill:#d8f3e0,stroke:#2da44e,color:#0b3d1f;

    class cfg input
    class disc,plan sched
    class query effect
    class store,charts commit
```

Each collection cycle (`collect.go`), in order:

1. compile the raw configuration and resolve one AWS account id per target
   (`plan.go`, `identity.go`);
2. discover live instances per (target, profile, region) with `ListMetrics`
   (`discover.go`);
3. reuse or rebuild the query blueprint, grouped by (target, region, period)
   (`query_plan.go`);
4. execute the due queries in concurrent batches (`query_executor.go`);
5. write each value as a float gauge into `metrix`, stamped with
   `{account_id, region, <dimension labels>}`, and re-emit cached values for
   not-due series (`observe.go`);
6. serve a chart template built once from the selected profiles (`chart.go`).

## Lifecycle

Framework V2: `Init` → `Check` → repeated `Collect`; there is no background
`Run` (the collector does not implement `CollectorV2Runner`). `collector.go`
owns the `Collector` struct and lifecycle.

```mermaid
flowchart TD
    Init("Init<br/>applyDefaults + validate<br/>no AWS calls")
    Check("Check")
    Runtime("ensurePlan<br/>load catalog · compile rules<br/>build + cache chart template")
    Targets("ensureTargets<br/>concurrent STS GetCallerIdentity<br/>fail-soft · retry pending")
    Post("framework postCheck<br/>validate ChartTemplateYAML via chartengine")
    Collect("Collect<br/>every update_every")
    Cycle("collect cycle<br/>discover → plan → schedule → execute → observe")

    Init --> Check
    Check --> Runtime --> Targets --> Post
    Post --> Collect --> Cycle --> Collect

    classDef input fill:#eef1f4,stroke:#8b949e,color:#24292f;
    classDef sched fill:#ece2ff,stroke:#8250df,color:#3b1f6b;
    classDef commit fill:#d8f3e0,stroke:#2da44e,color:#0b3d1f;
    classDef core fill:#cfe4ff,stroke:#1f6feb,stroke-width:3px,color:#0b2948;

    class Init,Check input
    class Runtime,Targets sched
    class Post commit
    class Collect,Cycle core
```

- **Init** does config only (`applyDefaults`, `validate`) — no network.
- **Check** compiles the configuration, builds the chart template, and resolves
  up to 64 target account IDs concurrently through STS. The template is built here because the
  framework's `postCheck`
  validates `ChartTemplateYAML()` through `chartengine.LoadYAML` before the
  first `Collect`; an unbuilt template would fail the job.
- **Collect** runs the whole cycle (below). `ensurePlan` short-circuits once
  compiled; `ensureTargets` retries only unresolved targets.
- **Cleanup** resets the collection plan, resolved targets, cached template,
  discovery/query snapshots, client caches, and observation schedule so a framework
  re-Init after failed autodetection starts clean. The `metrix` store is created
  once in `New` and persists — it is not recreated.
- **ChartTemplateYAML** returns the cached string; no work at call time.

## Collection Cycle

`collect.go` ties the stages together, in order:

```mermaid
flowchart TD
    A("ensure collection plan + target account ids")
    B("refreshDiscovery<br/>shared ListMetrics scans per compatible target scope")
    C("currentQueryPlan<br/>cached until target/discovery/tag inputs change")
    D("dueGroups + filterDueQueries<br/>keep only (target, region, period) groups due now")
    E("executeQueries<br/>batched · concurrent GetMetricData")
    F("advanceSchedule + observe<br/>write gauges · re-emit retained")

    A --> B --> C --> D --> E --> F

    classDef input fill:#eef1f4,stroke:#8b949e,color:#24292f;
    classDef sched fill:#ece2ff,stroke:#8250df,color:#3b1f6b;
    classDef effect fill:#ffe6cc,stroke:#e36209,color:#5a2a00;
    classDef commit fill:#d8f3e0,stroke:#2da44e,color:#0b3d1f;

    class A input
    class B,C,D sched
    class E effect
    class F commit
```

## Configuration Compilation

`config.go`, `config_validate.go`, `plan.go`, `plan_compiler.go`, and
`runtime.go` separate the raw operator contract,
compiler state, and installed execution plan:

- YAML and JSON use normal typed decoding; unknown keys are ignored. `Config.validate`
  decides whether the resulting configuration is valid during `Init` / `Check`.
  Known future rule fields (`filters`, `labels`, `series`, and `query`) remain typed
  so non-null values are rejected until their implementation phases land; null is
  equivalent to omission.
- Named credential sources describe only credential acquisition. Named targets
  describe monitored identities and optional role assumption. Ordered rules bind
  targets to profile selectors and regions.
- `compileConfig` coordinates a private staged compiler that resolves every reference, rejects unused credential/target
  definitions, applies profile defaults/include/exclude semantics, intersects
  intrinsic supported regions, enforces target/role partition consistency, and
  emits immutable ordered scopes.
- Exact duplicate target/profile/region scopes are removed statically with one
  bounded aggregate diagnostic per affected rule. Same-account cross-target overlap remains until discovery,
  where final emitted-series identity can be evaluated correctly.
- Fixed internal caps bound credentials, 64 targets, rules, list references,
  candidate-scope evaluation, and compiled scopes. Overflow fails compilation and never installs a partial plan;
  these safeguards are not public tuning settings.

## Discovery

*Which* profiles to query is decided by config, not by discovery (see Profiles):
the selected profiles are the CloudWatch namespaces `ListMetrics` runs against.
Discovery then finds which *instances* of those profiles exist per target and region.

`discover.go`. `refreshDiscovery` re-runs only when the snapshot TTL
(`discovery.refresh_every`, default 300s) has expired.

- `discoveryGroups` coalesces compiled scopes by target, region, namespace, and
  `RecentlyActive` behavior. `discoverAll` fans out over those groups concurrently
  (bounded by `apiConcurrency`), with one CloudWatch client per (target, region).
- `discoverProfileGroup` pages `ListMetrics` once for the shared namespace and
  applies every grouped profile matcher while streaming the response.
  `matchInstanceDimensions` keeps a returned metric only if its dimension-**name**
  set exactly equals the profile's set — same names, same cardinality. This
  collapses CloudWatch's multi-granularity fan-out to the chosen instance grain
  and dedups shared instances. A dimension pinned to a `constant` value is
  additionally kept only when the metric's value for it equals that constant
  (`constantDimensionsHold`, fail-closed), so a constant dimension can never merge
  distinct instances onto one unlabeled series.
- **Recently-active-only** is period-aware: the `ListMetrics RecentlyActive=PT3H`
  filter is applied only when every metric in the profile has a period ≤ 3h.
  PT3H is the only value CloudWatch accepts, so applying it to a daily profile
  (S3) would hide the metric most of the day. Configurable
  (`discovery.recently_active_only`, default true).
- **Snapshot + carry-forward**: `buildDiscoverySnapshot` stores instances for
  successful targets and **carries forward the previous instances for errored
  targets**, so a transient per-region/namespace failure never drops series.
  Only a first-ever pass where every scope errors is fatal; after any
  snapshot exists, discovery errors are warnings.
- A warning fires at ≥1000 discovered instances (a cost signal); collection is
  never truncated.

## Query Planning And Scheduling

`query_plan.go` + `observe.go`.

- `currentQueryPlan` reuses an immutable blueprint until a target resolves or a
  discovery/tag snapshot changes. `buildQueryPlan` emits one `plannedQuery` per
  `instance × metric × statistic` when that blueprint is invalidated.
  Identity labels are `{account_id, region}` plus one label per identifying
  instance dimension (a `constant` dimension is sent in the query but not
  labeled). The exported series name is `<profile>.<metric_id>_<statistic>`.
- Compiled scope order and a final-series identity set enforce rule precedence:
  the first rule/target that produces a series owns it. This catches dynamic
  overlap when distinct targets resolve to the same account and see the same resource.
- Queries are grouped by `queryGroupKey{target, region, period}` — the batch unit
  (shared client and time window) and the scheduling unit.
- The `observationStore` keeps a per-(target, region, period) `nextQueryAt`. `dueGroups`
  returns groups whose next time has arrived (or the first cycle);
  `filterDueQueries` drops the rest. So a period-86400 (S3 daily) group is
  queried far less often than the 60s collect cycle, while a 300s group runs
  every few cycles.
- The schedule advances **only for groups queried successfully**
  (`advanceSchedule`), so a failed region retries next cycle instead of skipping
  a whole period.

## Query Execution

`query_executor.go`. `executeQueries`:

- `indexPlan` groups the plan by (target, region, period); `resolveGroupClients`
  builds one client per (target, region) up front (errors recorded once per pass).
- `buildChunkJobs` computes the time window and splits each group into chunks of
  `metricsPerQuery` (500, the `GetMetricData` per-call hard maximum), one job
  per chunk. The chunk size is an explicit argument so tests can exercise the
  split without 500+ queries.
- `runChunkJobs` runs the chunk jobs concurrently (bounded by `apiConcurrency`,
  each under `timeout`).
- `runGetMetricData` calls `GetMetricData` with `ScanBy` descending (newest
  first), follows `NextToken`, and keeps the **first value per query id** (the
  newest). A per-result `InternalError` / `Forbidden` (or a missing result) marks
  the query id **unusable**, so its series gaps rather than zero-filling; a usable
  result with no datapoint (or `NaN` / `Inf`) is a clean no-data outcome that
  `observe` resolves per the metric's `nil_as_zero` policy (0 or gap). `PartialData`
  is normal pagination (followed via `NextToken`) and stays usable.
- The **query window** is `[end-period, end]` where
  `end = alignedNow - max(query_offset, period)`. Aligning to the period and
  offsetting by at least one period guarantees the queried bucket is already
  published and settled.
- If every chunk errors, the whole pass errors; otherwise partial failures are
  tolerated and their groups stay due.

## Observation And Metrics

`observe.go`. `observe` writes one snapshot frame per cycle:

- `meter := store.Write().SnapshotMeter("")`; each sample becomes
  `meter.WithLabels(labels...).Gauge(series, metrix.WithFloat(true)).Observe(v)`.
- **Always a gauge.** CloudWatch returns per-period aggregates (Sum, Average, …)
  as absolute points, so gauge last-write-wins maps directly. Every profile
  chart must declare `algorithm: absolute` (enforced by profile validation);
  there is no counter/delta tracking.
- **Float is a metric-level hint.** `metrix.WithFloat(true)` marks the metric
  family float-native; `chartengine` inherits that onto the chart dimension, so
  fractional values render at full precision without a per-dimension
  `options.float`. (Rate divisors, by contrast, *are* injected per dimension —
  see Charts.)
- **Retention re-emit (sample-and-hold).** This decouples two cadences: the
  **query cadence** is each metric's *period* (the paid `GetMetricData` calls,
  gated by the schedule), while the **write cadence** is the collect loop
  (`update_every`, free `metrix` writes). Netdata expects a value per dimension
  per collect cycle, so a series whose group was **not** queried this cycle (not
  due, or due-but-failed) is re-written from its last cached value — a long-period
  metric (e.g. daily S3) renders as a continuous step line instead of gaps, at no
  extra AWS cost. (Exception: a total-failure cycle — every due query fails —
  returns an error before `observe`, so nothing is written that cycle and the
  schedule stays due for retry.) A series in a *successfully queried* group that returned no
  datapoint is reconciled per the metric's no-data policy: metrics whose effective
  policy is `nil_as_zero: true` record **0**; the rest **drop** the cached value so
  the series gaps until fresh data, and a stale value is never re-emitted.
  Without an explicit override, only `rate: true` metrics at `sum` or
  `sample_count` default to zero; every other statistic gaps. The cache otherwise
  persists until the instance leaves discovery and `pruneObserved` drops it.
- `pruneObserved` drops both cached series and per-(target, region, period) schedule entries
  absent from the current plan when that immutable plan is rebuilt, so removed resources
  stop being re-emitted and a group that later reappears is queried on its first cycle
  back rather than waiting for a stale schedule entry to expire.

## Tag Enrichment (`tags.go`, `tagjoin.go`, `tagresolve.go`)

Opt-in (`tags` config, empty by default). When set, the collector attaches selected
AWS resource tags as **non-identity** chart labels, so `i-0abc123` also carries
`owner`/`project`/`name`/… without changing series identity. It slots into the cycle
between discovery and query planning:

```text
refreshDiscovery → refreshTags → buildQueryPlan → … → observe
```

- **Client + cache.** A Resource Groups Tagging API (RGTA) client is built per
  `{target, region}` (the same generic `clientCache[T]` as the CloudWatch client).
  `refreshTags` runs on the discovery TTL and is best-effort: a per-`{target, region}`
  failure keeps that scope's **last-known** tags (a first-run failure yields none) and
  never fails the cycle. With no tags configured, no RGTA client is ever built.
- **Resolution (`tagresolve.go`).** `resolveTagPlan` turns the allowlist into a
  per-profile `awsKey → label` plan, once. It is **non-fatal**: an entry whose label is
  empty/invalid, collides with a reserved (`account_id`/`region`) or dimension label, or
  duplicates another tag is **skipped with a warning** — never a config error, never an
  emitted duplicate key (which would panic `metrix`). `rename` resolves collisions; the
  default label is the sanitized key (`Name` → `name`).
- **ARN↔dimension join (`tagjoin.go`).** RGTA returns ARN + tags; the cache is keyed by
  the profile's ARN-projectable `joinKey`. A per-profile mapper (a default
  last-resource-segment extractor plus overrides for ALB/NLB/target-group/ECS/OpenSearch/
  Step-Functions) derives the `joinKey` from the ARN, and the instance side projects its
  dimension values onto the same key. **Safe failure mode:** a wrong ARN assumption is a
  cache *miss* (no tags), never *wrong* tags, because the instance side uses real
  dimension values. Parent-resource profiles (S3, DynamoDB-operation, ALB-target) key on
  the parent dimension so children inherit its tags. Unregistered profiles
  (auto_scaling, bedrock, and the ambiguous cloudfront/api_gateway/msk/elasticache) carry
  no tags.
- **Emission split.** Identity `labels` (`{account_id, region, <dims>}`) and `tagLabels`
  travel separately through `plannedQuery`/`querySample`/`observedSeries`. `writeSample`
  emits `labels + tagLabels` in a fresh slice; `observedKey` (the retention/scheduling
  identity) uses **identity labels only**, so a tag change never churns scheduling, and
  retention re-emit carries the last-known `tagLabels`. `chartengine` `auto_intersection`
  then promotes the tag labels as non-identity chart labels — no template change.
- **INV.2.** Tags only ADD labels; they never gate series existence. A custom profile
  that names a tag in `instances.by_labels` (or uses `["*"]`) is out of scope — that is
  the operator's own chart-identity choice.

## Dynamic Charts

`chart.go`, built once by `ensurePlan` and cached in `chartTemplateYAML`.

```text
for each selected profile:
  group := profile.Template.Clone()          # typed deep copy; never mutate the catalog
  group.Metrics = sorted(visible series)     # collector-owned visible-series list
  injectDimensionOptions(group, series)      # sets options.divisor = period on rate dims; keeps authored options
assemble charttpl.Spec{Version, ContextNamespace: "cloudwatch", Groups}
return spec.MarshalTemplate()                # Validate + yaml.v2 marshal, then cached
```

- Uses the framework `charttpl` primitives: `Group.Clone()` (typed deep copy, so
  the shared profile catalog is never mutated) and `Spec.MarshalTemplate()`.
- `selectorSeriesName` extracts a dimension selector's target metric with the
  same `metrix/selector` library the chart engine matches with, so the injected
  divisor always agrees with the engine.
- `float` is not injected here (it is the metric-level hint above); only the
  rate `divisor` is.

## Profiles (`cwprofiles/`)

`profile.go` defines the schema; `catalog.go` loads and resolves; stock profiles
live under `config/go.d/cloudwatch.profiles/default/` (one YAML per service; a
user file with the same basename overrides its stock counterpart). The public
authoring contract lives in [profile-format.md](profile-format.md); keep it in
lockstep with every profile-schema change.

A `Profile` declares: `namespace` (e.g. `AWS/EC2`), optional
`supported_regions` (an intrinsic restriction for services such as CloudFront),
`period`, `instance`
dimensions (the CloudWatch dimension names that identify one instance; each is
either mapped to a Netdata `label`, or pinned to a `constant` value — a
match-and-query-only dimension that is matched and queried but not emitted as a
label, for a constant CloudWatch dimension such as CloudFront's `Region=Global`),
`metrics` (with `statistics`, optional `rate`,
optional per-metric `period`, and optional `nil_as_zero` — record 0 vs gap on a
no-datapoint result, defaulting to `rate`), and a `charttpl.Group` `template`.

Load and resolution (`catalog.go`):

- Stock profiles ship under the stock dir; user profiles under the user dir. A
  user profile **overrides** a stock profile with the same basename.
- Invalid **stock** profiles are fatal; invalid **user** profiles are logged and
  skipped.
- Decode is **non-strict** (unknown keys ignored), so a profile that merely *adds*
  optional fields a newer collector understands still loads on an older binary —
  but only while old validation still passes. It is not a blanket guarantee: a
  profile that relies on a newer field to validate is rejected by an older binary
  (e.g. a dimension using `constant` omits `label`, which an older binary still
  requires).
- **Profile selection is rule-driven**, not discovery-driven. Omitting
  `rules[].profiles` includes every default-enabled profile. `defaults: false`
  with `include` selects only named profiles; `defaults: true` with `include`
  adds disabled opt-in profiles; `exclude` removes names from the union. Explicit
  include/exclude overlap is invalid. The compiler intersects each rule's regions
  with `supported_regions`: an incompatible defaults-selected profile is skipped
  with a startup diagnostic, while an explicitly included incompatible profile
  fails configuration.

Profile validation invariants (`profile.go`) — these are load-bearing:

- Every chart's `instances.by_labels` must include `account_id`, `region`, and
  every identifying instance-dimension label (a `constant` match-and-query-only
  dimension has no label and is excluded). Chart-instance identity is built solely
  from `by_labels`; a missing label would silently merge distinct AWS resources
  onto one chart instance.
- Every chart must declare `algorithm: absolute` (CloudWatch aggregates are not
  cumulative counters; this also blocks incremental suffix inference).
- `template.metrics` must be empty — the collector owns the visible-series list
  and fills it at build time.
- `rate: true` requires a `sum` or `sample_count` statistic (the per-second value
  divides a per-period total — the summed value or the observation count — by the
  period).

## Credentials, Targets, And Account Identity

`internal/awsauth` is collector-local. A named credential source is either the
AWS SDK default chain (environment, shared config, EC2 instance profile, EKS
IRSA) or explicit static/session credentials. A target uses one source directly
or layers one `AssumeRole` provider over it, so static credentials can bootstrap
cross-account roles without environment indirection. The collector builds an
`aws.Config` per (target, region).

Credential sources are a list of named entries. Each entry uses a required
`type` selector. `type: default` has no branch-specific configuration;
`type: static` requires a `type_static` object containing `access_key_id`,
`secret_access_key`, and an optional `session_token`. Keeping branch-specific
fields nested makes the runtime model match the conditional DynCfg form. After
validation, the plan compiler builds a private name index for target reference
resolution; list order has no credential precedence semantics.

Each target's AWS account id is resolved through STS `GetCallerIdentity`
(`identity.go`, `ensureTargets`) and stamped on its series. Resolution is
fail-soft and retried: an unresolved target stays pending while healthy targets
continue; only a cycle with no resolved target fails. Named targets are never
deduplicated by account id because their permissions and visible resources may
differ. Dynamic final-series overlap is deduplicated later by ordered rule/target
precedence. AWS does not require an explicit IAM permission grant for
`GetCallerIdentity`.

Partition consistency is enforced per target. A target cannot span AWS
partitions, and an assumed-role ARN partition must match the selected regions
(aws / aws-us-gov / aws-cn / aws-iso / aws-iso-b / aws-iso-e / aws-iso-f /
aws-eusc).

## Concurrency

- `apiConcurrency` (5) bounds both the discovery fan-out and the GetMetricData
  chunk execution, via `conc/pool`. `metricsPerQuery` (500) is the GetMetricData
  batch size. Both are internal constants, not config.
- `clientCache` builds at most one CloudWatch client per (target, region), under
  a mutex, caching only successes (a transient credential error is retried next
  call).
- The top-level `collect` stages run sequentially, and the per-cycle `store`
  write is single-threaded. Only discovery and query execution fan out.

## Cost Model

The two CloudWatch APIs bill differently, and the design leans on that:

- **`ListMetrics` (discovery) is cheap** — it falls under CloudWatch's free API
  tier (~1M requests/month), then ~$0.01 per 1,000 requests, and returns only
  metric metadata (no per-datapoint charge). So `discovery.refresh_every`
  (default 300s) barely moves the bill; the recently-active filter further trims
  result pages. Raise it only to cut API load on very large accounts.
- **`GetMetricData` (query) is the cost driver** — it is always billed (~$0.01
  per 1,000 metrics requested) and scales with metric count × query frequency.
- **Per-`(target, region, period)` scheduling is the governor** — a metric is queried
  once per its own period (a 300s metric every 300s, a daily metric every ~24h),
  not every collect cycle, so cost tracks profile periods rather than
  `update_every`. Between queries, values are re-emitted from cache at zero AWS
  cost (see Observation And Metrics).

Narrow the bill with focused rule target/profile/region selections or longer
profile periods; discovery frequency is a minor lever. (Rates are AWS's published model —
verify current per-region prices on the CloudWatch pricing page.)

## Key Invariants

- **Instance identity = exact dimension-name match** — a metric joins a profile
  only if its dimension-name set equals the profile's exactly.
- **Query window is a settled bucket** — offset by `max(query_offset, period)`
  with period alignment.
- **No-data policy is per-metric** — a successfully-queried empty result records
  0 (`nil_as_zero`, defaulting on only for `rate: true` `sum`/`sample_count`) or
  gaps. Not-due series and groups skipped by client/chunk transport failures
  re-emit their last value; per-result `InternalError`, `Forbidden`, missing
  results, and successful gap-policy no-data drop the stale value.
- **Schedule advances only on success**, per (target, region, period).
- **Fail-soft discovery** — carry forward on error; only a first-ever total
  failure is fatal.
- **Job-level placement** — AWS resources are chart instances via `by_labels`.
  They use the configured job `vnode` or the Agent host; no per-target/resource
  HostScope is generated.
- **Tags are additive** — the opt-in `tags` allowlist adds non-identity labels
  only; it never gates series existence and never overwrites an identity label
  (a colliding tag is skipped with a warning).
- **Compiled configuration** — operator-facing credentials, targets, and ordered
  rules are validated and expanded once. Discovery, `query_offset`, `timeout`,
  the opt-in `tags` allowlist, `vnode`, and the framework's common fields remain
  job-level settings.
  Concurrency, batch size, and the recently-active period bound are internal
  constants.

## Where To Change Things

- **Add or adjust a monitored AWS service**: add or edit a profile YAML under
  `config/go.d/cloudwatch.profiles/default/` (namespace, instance dimensions,
  metrics, chart template). A new service that fits the profile model needs no
  Go change. Add a matching `metadata.yaml` monitored-instance entry and
  regenerate the integration docs.
- **Change discovery** (matching, fan-out, snapshot/TTL, recently-active):
  `discover.go`.
- **Change query batching, windows, pagination, or gap handling**:
  `query_plan.go` (planning + window) and `query_executor.go` (execution).
- **Change per-period scheduling or retention/re-emit**: `observe.go`.
- **Change how samples become metrics** (labels, float hint, gauge): `observe.go`.
- **Change chart generation** (rate divisor, namespace, template assembly):
  `chart.go`, plus the profile `template`.
- **Change auth**: `internal/awsauth` (collector-local; extract to a shared package only when a second AWS consumer appears).
- **Add or change a config option**: `config.go` + `config_schema.json` +
  `metadata.yaml` + stock `cloudwatch.conf` + regenerated `integrations/` docs
  (collector consistency). Prefer internal constants over new options unless the
  option names a real operator decision.

## Validation Checklist

```text
cd src/go
go test -count=1 ./plugin/go.d/collector/cloudwatch/...
go vet ./plugin/go.d/collector/cloudwatch/...
gofmt -l plugin/go.d/collector/cloudwatch/
```

If `metadata.yaml` changed, regenerate and commit the integration docs
(collector consistency; see the `integrations-lifecycle` skill):

```text
python3 integrations/gen_integrations.py
python3 integrations/gen_docs_integrations.py -c go.d.plugin/cloudwatch
```
