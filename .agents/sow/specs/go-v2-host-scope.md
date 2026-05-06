# Go V2 Host Scope And Virtual Node Emission

## Scope

This spec records the framework contract for Go collector v2 host-scope routing.
It applies to `pkg/metrix`, `plugin/framework/jobruntime`,
`plugin/framework/chartengine`, and v2 collectors that emit metrics for remote
or virtual-node targets.

## Metrix Host Scope

- `metrix.HostScope{}` is the default host scope.
- Unscoped writes are equivalent to writes in the default scope.
- A non-default host scope carries:
  - `ScopeKey`: stable scope partition key;
  - `GUID`: Netdata host/vnode GUID;
  - `Hostname`: Netdata host/vnode hostname when defining the host;
  - `Labels`: deterministic host/vnode labels.
- Series identity includes host scope. The same metric name and labels can exist
  in default scope and multiple non-default scopes without collision.
- `Read()` without `ReadHostScope` returns default-scope series only.
- `Read(ReadHostScope(key))` returns only that scope's series.
- `Reader.HostScopes()` enumerates all scopes present in the snapshot and is not
  filtered by the reader's active host scope.
- Flattened synthetic series preserve the source host scope.
- Scope metadata conflicts in one collect cycle are data errors surfaced through
  `CommitCycleSuccess() error`, not panics.

## Jobruntime V2

- V2 jobruntime owns host/vnode orchestration. Chartengine remains host-agnostic.
- One `chartengine.Engine` is used per host scope for a job.
- Scope engines are created lazily.
- Default-scope metrics continue to emit under the job-level vnode when one is
  configured, otherwise under the global host.
- Explicit non-default scopes emit under their `metrix.HostScope` GUID and host
  metadata.
- Collection and `metrix.CommitCycleSuccess()` are still all-or-nothing.
- Post-collect plan/apply/commit is per-scope partial success. A failed scope
  rolls back its own registry changes and does not block unrelated scopes.
- Disappeared scopes are retained and read with empty scoped readers until the
  per-scope chartengine emits lifecycle removals. After successful removal
  emission, jobruntime releases scoped registry owners and destroys the scope
  engine.
- Job cleanup emits obsolete charts for each retained scope before releasing
  registry owners.

## Vnode Registry

- V2 vnode definitions go through the shared `framework/vnoderegistry` registry.
- Registry entries are keyed by host GUID and owner.
- Metadata is update-on-change. A new normalized metadata value for an existing
  GUID replaces retained metadata and causes another `HOST_DEFINE`.
- Owner release removes an entry only after the last owner for that GUID leaves.
- Job-level vnode owners and explicit scoped vnode owners use separate owner
  namespaces.

## Chartengine Runtime Metrics

- Per-scope engines do not register per-scope runtime components.
- Jobruntime feeds chartengine runtime samples into one job-level
  `chartengine.RuntimeAggregator`.
- Aggregated runtime metrics do not include host-scope/workload labels.
- Counter-like runtime metrics are summed across samples.
- Gauge-like size metrics represent the latest successful build rollup, summed
  across successful engines in that rollup.
- `build_seq_violation_active` is `1` when any observed engine reports a
  sequence violation in the rollup, otherwise `0`.

## Collector Contract

- V2 collectors that need per-target virtual nodes should write target metrics
  through `meter.WithHostScope(scope)` or equivalent scoped vec/instrument
  bindings.
- V2 collectors should leave metrics unscoped when those metrics belong to the
  default job vnode/global host.
- Collector-generated scope keys must be deterministic and stable for the target
  host/vnode identity.
- Collector-generated host labels should include `_vnode_type=<source>` when a
  collector creates virtual nodes from an internal mechanism rather than a
  user-defined vnode entry. The value must identify the mechanism or source
  without embedding high-cardinality target values.
- Azure Monitor resource-tag virtual nodes use `_vnode_type=azure_workload`.
  Their GUID and `ScopeKey` are the deterministic SHA1 UUID of
  `azure_monitor:` plus the trimmed, case-preserved tag value.
- Collectors are responsible for bounding or documenting scope cardinality risk.
