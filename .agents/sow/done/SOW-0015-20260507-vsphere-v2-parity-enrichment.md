# SOW-0015 - vSphere V2 Parity And Enrichment

## Status

Status: completed

Sub-state: Implementation is complete for the approved PR scope. The SOW has
been collapsed to the final shipped state by user decision. Final validation
passed on 2026-05-23, and this SOW is being moved to `.agents/sow/done/` with
the closeout commit.

## Requirements

### Purpose

Migrate the Go vSphere collector to framework V2 while preserving the useful V1
metric contract, adding approved vSphere parity and enrichment surfaces, and
removing transitional or high-cardinality features that should not ship in this
PR.

### User Request

The user requested a clean end state for the vSphere V2 migration and parity
work, including:

- framework V2 collection and chart templates;
- compatibility with the existing vSphere metric surface where accepted;
- default-safe additive object-level metrics;
- opt-in datastore-cluster, vSAN, tag/custom-attribute, and topology surfaces;
- removal of config/options/features that are not worth shipping now;
- charts.yaml as the single chart source of truth;
- collector taxonomy that passes the fatal taxonomy CI gate;
- tests converted toward table-driven and V2 metric-store assertions;
- a final SOW that records the end state rather than the development diary.

### Acceptance Criteria

- The vSphere collector registers and runs through framework V2.
- Current default VM, host, datastore, cluster, and resource-pool metric
  contexts remain available with accepted labels, dimensions, units, and values.
- Chart IDs may change to framework V2 instance chart IDs; `id` is the stable
  vSphere managed-object-reference label used for chart instances.
- `charts.yaml` is authoritative. The old Go chart-template mirror, runtime
  chart bridge, and V1 golden fixture are removed.
- Default-safe metrics for snapshots, VM/host/cluster state, datastore state,
  inventory counts, and aggregate power are implemented.
- Optional datastore-cluster and vSAN metrics are default-off and selector
  controlled.
- Optional vSphere tag/custom-attribute labels are default-off and allowlist
  controlled.
- Optional network topology discovery is default-off and exposed only through
  the cached topology Function.
- VM/host child-instance metric surfaces, generated ESXi/VM vnodes,
  inventory-path labels, VM guest labels, host/VM power-state config controls,
  and the power-metrics config knob are not part of the shipped public surface.
- `metadata.yaml`, `config_schema.json`, stock `go.d/vsphere.conf`,
  `charts.yaml`, health alerts, generated integration docs, and
  `taxonomy.yaml` are consistent with the final code.
- Local validation covers Go tests, vet, chart-template compilation, taxonomy
  checks, docs/schema parsing, and targeted reviewer feedback.

## Final Scope

### Framework And Runtime

- Registration uses framework V2 via `CreateV2`.
- `Collect(ctx)` writes directly to `metrix.CollectorStore`.
- The legacy `map[string]int64` collection bridge is gone.
- `charts.yaml` is embedded and returned by `ChartTemplateYAML()`.
- Runtime chart mutation is gone; chart lifecycle is handled by chartengine and
  chart-template `expire_after_cycles`.
- Metric writers emit final V2 metric names directly and consistently attach
  the `id` label plus resource-specific labels.

### Configuration Contract

Current vSphere-specific config keys:

- target and scheduling: `url`, `username`, `password`, `timeout`,
  `discovery_interval`, `update_every`, `autodetection_retry`, `vnode`;
- core include selectors: `host_include`, `vm_include`,
  `datastore_include`, `cluster_include`;
- optional labels: `tag_categories`, `custom_attributes`;
- optional datastore clusters: `collect_datastore_clusters`,
  `datastore_cluster_include`;
- optional vSAN: `collect_vsan`, `vsan_cluster_include`,
  `vsan_host_include`, `vsan_vm_include`;
- optional topology: `collect_network_topology`;
- inherited HTTP/TLS/proxy settings from `web.HTTPConfig`, with unused
  method/body-style fields hidden from the dynamic configuration UI.

Removed before merge:

- `max_*` resource caps;
- `collect_power_metrics`;
- `host_power_states`, `vm_power_states`;
- `collect_inventory_path_label`, `vm_guest_labels`;
- `esxi_vnodes`, `vm_vnodes`;
- VM child-instance options:
  `collect_vm_disks`, `collect_vm_disk_performance`, `vm_disk_include`,
  `collect_vm_nic_performance`, `vm_nic_include`;
- host child-instance options:
  `collect_host_nic_performance`, `host_nic_include`,
  `collect_host_disk_performance`, `host_disk_include`,
  `collect_host_storage_adapter_performance`,
  `host_storage_adapter_include`,
  `collect_host_storage_path_performance`, `host_storage_path_include`,
  `collect_host_cpu_instance_performance`,
  `host_cpu_instance_include`.

### Metric Surface

The final metric surface is documented in `collector/vsphere/metadata.yaml` and
charted in `collector/vsphere/charts.yaml`.

Default metric groups:

- inventory object counts;
- VM aggregate CPU, memory, swap, disk, network, power state, connection state,
  tools state, consolidation state, uptime, configuration, storage usage, and
  snapshot metrics;
- host aggregate CPU, memory, swap, disk, network, overall status, power state,
  connection state, maintenance state, uptime, power, and energy metrics;
- datastore aggregate I/O, IOPS, latency, space, accessibility, maintenance,
  multiple-host-access, and overall status metrics;
- cluster capacity, topology, utilization, DRS, HA, vMotion, VM operation,
  inventory, and overall status metrics;
- resource-pool CPU, memory, allocation, config, and status metrics.

Optional metric groups:

- datastore-cluster space, Storage DRS status, and overall status metrics behind
  `collect_datastore_clusters`;
- vSAN cluster, host, and VM capacity/performance/health metrics behind
  `collect_vsan`.

Aggregate VM and host disk/network metrics remain default-on. Per-disk,
per-NIC, per-storage-adapter, per-storage-path, and per-CPU-instance metrics are
excluded from this PR.

### Labels

Every emitted series includes the V2 `id` label.

Resource-specific labels:

- VM: `datacenter`, `cluster`, `host`, `vm`;
- host: `datacenter`, `cluster`, `host`;
- datastore: `datacenter`, `datastore`, `type`;
- cluster: `datacenter`, `cluster`;
- resource pool: `datacenter`, `cluster`, `resource_pool`;
- datastore cluster: `datacenter`, `datastore_cluster`;
- vSAN cluster: `datacenter`, `cluster`, `vsan_uuid`;
- vSAN host: `datacenter`, `cluster`, `host`, `vsan_node_uuid`;
- vSAN VM: `datacenter`, `cluster`, `host`, `vm`, `vm_instance_uuid`;
- inventory: `id=inventory`.

Optional enrichment labels:

- `vsphere_tag_<category>` for tag categories matched by `tag_categories`;
- `vsphere_custom_attribute_<name>` for custom attributes matched by
  `custom_attributes`.

Tag/custom-attribute names are sanitized for label keys. Multiple tags in one
category are sorted and joined with `|`. Users are warned not to allowlist
categories or attributes that may contain secrets or sensitive data.

Standalone-host dummy clusters are detected by `domain-s*` cluster IDs, not by
name equality.

### Functions And Topology

- `vsphere:readiness` is a read-only cached Function. It reports local cached
  readiness, configured optional gates, and discovered resource counts without
  issuing extra vCenter API calls.
- `topology:vsphere` is the public cached topology Function alias. It emits
  topology actors and links for datacenters, clusters, hosts, VMs, datastores,
  datastore clusters, and resource pools from cached discovery state.
- `collect_network_topology` adds vSphere Network and Distributed Virtual Port
  Group actors and host/VM network links to topology output only. It does not
  create charts or metric labels.
- `opaqueNetwork-` managed-object IDs map to `vsphere_network`, so NSX-backed
  network links do not break.

### Dashboard And Documentation Artifacts

- `charts.yaml` is the single source for chart templates.
- `taxonomy.yaml` places vSphere under `containers-vms` and mirrors the
  existing cloud-frontend vSphere dashboard TOC:
  heads grid, Inventory, Clusters, Hosts, Virtual Machines, Resource Pools,
  Datastores, and Datastore Clusters.
- `metadata.yaml`, `config_schema.json`, stock `go.d/vsphere.conf`, generated
  integration markdown, and `health.d/vsphere.conf` match the final config and
  metric surface.

## Out Of Scope

These items are intentionally not part of this PR:

- vCenter/ESXi events and logs;
- collector-generated ESXi and VM vnodes;
- datastore vnodes;
- inventory-path labels;
- VM guest hostname, IP address, and guest OS labels;
- MAC, IQN, WWN, datastore path, and other sensitive device identity labels;
- VM and host child-instance metric families;
- deeper vSAN internals such as disk-group, disk, component, CMMDS, and all
  Telegraf-style entity-type metrics;
- live permission probes in readiness;
- VCSA appliance health metrics, which belong to the `vcsa` collector;
- ESXi hardware sensors, which are covered by SNMP `vmware-esx`;
- workload/container metrics inside VMs, which belong to guest agents or
  Kubernetes collectors.

Any of these requires a separate user-approved product/NIDL decision before
implementation.

## Analysis

Sources checked:

- `collector/vsphere/*.go` and subpackages;
- `collector/vsphere/charts.yaml`;
- `collector/vsphere/metadata.yaml`;
- `collector/vsphere/config_schema.json`;
- `collector/vsphere/taxonomy.yaml`;
- `config/go.d/vsphere.conf`;
- `health.d/vsphere.conf`;
- `integrations/check_collector_taxonomy.py`;
- `.agents/sow/specs/vsphere-parity-matrix.md`;
- `.agents/sow/specs/vsphere-v1-compatibility-manifest.md`;
- `.agents/sow/specs/go-v2-host-scope.md`;
- project skills for collector authoring, framework V2 modules, and integration
  lifecycle.

External/source evidence captured in
`.agents/sow/specs/vsphere-parity-matrix.md` includes Broadcom vSphere/vSAN API
documentation and checked open-source implementations from Datadog, Telegraf,
Grafana vmware_exporter, Elastic Beats, Zabbix, New Relic, OpenTelemetry
Collector Contrib, and Grafana Alloy.

Root-cause model:

- The pre-existing vSphere collector had a V1 runtime/chart model and a narrower
  metric surface.
- Framework V2 requires a static chart-template contract and metric-store
  writers instead of runtime chart mutation.
- Some parity candidates are useful object-level metrics; others are
  high-cardinality or sensitive identity surfaces that should not be exposed as
  broad public config in this PR.
- The final implementation keeps default behavior useful and bounded, makes
  costly/sensitive additions opt-in, and removes transitional APIs before merge.

Primary risks:

- vCenter/vSAN live API behavior can differ from simulator behavior. Local tests
  cover typed govmomi APIs and parser behavior, but no real production vCenter
  was available in this worktree.
- Chart ID continuity is intentionally broken by the V2 migration. Contexts,
  dimensions, labels, units, and values are preserved where accepted.
- Optional vSAN APIs require privileges and vSAN availability not represented by
  the simulator.
- Optional tag/custom-attribute enrichment can expose sensitive data if users
  allowlist sensitive categories or attributes; docs and config descriptions
  warn about this.

Sensitive data handling plan:

- Durable artifacts contain no real credentials, tokens, customer names,
  customer endpoints, private endpoints, or customer-identifying IP addresses.
- Examples use placeholders or generic local names.
- SOW evidence cites file paths, commands, and sanitized findings rather than
  raw vCenter data.

## Pre-Implementation Gate

Status: satisfied.

Affected contracts and surfaces:

- Go collector registration and runtime lifecycle;
- vSphere discovery and scraping;
- chart contexts, dimensions, labels, units, priorities, and lifecycle;
- dynamic configuration schema;
- stock configuration;
- integration metadata and generated documentation;
- health alerts;
- collector taxonomy;
- project specs and SOW lifecycle.

Existing patterns reused:

- framework V2 `CollectorStore`, `ChartTemplateYAML`, and chartengine tests;
- `collecttest.AssertChartCoverage` and V2 metric-store assertions;
- `web.HTTPConfig` embedding with UI-hidden unused fields;
- optional allowlist and selector patterns for bounded/sensitive data;
- cached read-only Functions for supportability/topology surfaces;
- fail-soft collection for optional enrichment and partial API failures.

Implementation plan:

1. Migrate collector runtime and charts to framework V2.
2. Preserve accepted V1 metric semantics and add default-safe object-level
   parity metrics.
3. Add approved opt-in datastore-cluster, vSAN, label enrichment, and topology
   surfaces.
4. Remove rejected or superseded public config and high-cardinality child
   metric surfaces.
5. Make `charts.yaml` and `taxonomy.yaml` authoritative source artifacts.
6. Rewrite tests around V2 metric-store output and chart-template coverage.
7. Update docs, schema, stock config, health alerts, specs, and SOW.

Validation plan:

- focused unit tests for parsers, matchers, discovery, writers, Functions, and
  review feedback;
- full vSphere package tests;
- Go vet;
- chart-template schema/decode/compile checks;
- taxonomy gate and exact metadata-to-taxonomy ownership checks;
- JSON/YAML parse checks;
- generated integration docs when metadata/config changes;
- same-failure grep for removed public keys and bridge symbols.

Open decisions:

- None for the implemented PR scope.

## Final User Decisions

1. Keep the vSphere work in one PR and split by focused commits.
2. Use framework V2 and accept framework V2 chart ID changes.
3. Preserve contexts, dimensions, labels, units, and meaning where accepted.
4. Use `charts.yaml` as the only chart-template source.
5. Use final V2 metric-store assertions instead of legacy runtime-chart maps.
6. Keep datastore-cluster and vSAN metrics opt-in.
7. Keep tag/custom-attribute labels opt-in with allowlists.
8. Remove generated ESXi/VM vnodes from this PR.
9. Remove inventory-path and VM guest labels from this PR.
10. Remove VM and host child-instance metric surfaces from this PR.
11. Remove host/VM power-state config controls.
12. Remove `collect_power_metrics`; aggregate power metrics are part of the
    shipped metric surface when vSphere exposes the counters.
13. Remove `max_*` caps from this collector before merge.
14. Keep vCenter/ESXi events out of this metrics PR.
15. Add `taxonomy.yaml` because the taxonomy gate is fatal.
16. Base vSphere taxonomy shape on the existing cloud-frontend vSphere TOC.
17. Collapse SOW-0015 to final-state evidence instead of development history.

## Implementation Summary

Runtime:

- Migrated collector registration and collection to framework V2.
- Added direct V2 gauge observation path.
- Removed V1 runtime chart bridge, Go chart mirror, V1 compatibility golden
  fixture, and legacy metric-map assertions.
- Kept deterministic sorting helpers for stable output and tests.

Discovery and scraping:

- Discovery includes selected non-powered hosts/VMs for property/status metrics
  while skipping real-time performance scraping where vSphere has no useful
  samples.
- Datastore, cluster, and resource-pool property refresh failures skip stale
  property metrics and let chartengine lifecycle handle expiry.
- Missing performance counters warn once per stable key and do not abort the
  whole collector.
- vSphere client cleanup handles partial initialization and session logout.

Metrics:

- Added VM snapshot count, maximum age, and maximum chain depth.
- Added VM/host/cluster/datastore/resource-pool property/status metrics.
- Added inventory object counts.
- Added aggregate host/VM power and energy metrics.
- Added optional datastore-cluster metrics.
- Added optional vSAN metrics.
- Preserved aggregate VM/host datastore/network/disk/cluster/resource-pool
  metrics accepted from V1.

Configuration and selectors:

- Added typed include selector types for core path includes and optional
  datastore-cluster/vSAN selectors while preserving YAML/JSON keys.
- Extracted reusable ordered simple-pattern list matching to `pkg/matcher`.
- Preserved config-specific validation and error-message shapes.
- Removed public keys that should not ship in this PR.

Labels and enrichment:

- Added optional vSphere tag and custom-attribute labels with allowlists.
- Preserved empty gates so REST/CIS/tag/custom-attribute clients are not used
  when enrichment is disabled.
- Added privacy warnings for user metadata labels.

Functions and topology:

- Added cached readiness Function.
- Added cached topology Function.
- Added optional network/DVPG topology discovery.
- Added `opaqueNetwork-` topology ID support.

Tests:

- Reworked collector tests to V2 metric-store reads and table-driven cases where
  setup/assertions are shared.
- Added chart-template schema/decode/priority/compile validation.
- Added `AssertChartCoverage` and selector-match checks for default and
  optional surfaces.
- Added focused tests for matchers, vSAN parsing, datastore clusters, power
  metrics, labels, topology, readiness, discovery, client cleanup, and reviewer
  feedback.
- Replaced fixed task lifecycle sleeps with deterministic `task.wait()`.

Artifacts:

- Updated `charts.yaml`, `metadata.yaml`, `config_schema.json`, stock
  `go.d/vsphere.conf`, health alerts, generated integration markdown, and
  `taxonomy.yaml`.
- Updated specs for vSphere parity and the superseded V1 compatibility manifest.
- Added/updated project skill guidance for framework V2 collector work.

## Validation

Acceptance criteria evidence:

- `collector/vsphere/charts.go`,
  `collector/vsphere/chart_template_sets.go`,
  `collector/vsphere/compat_manifest_test.go`, and
  `collector/vsphere/testdata/v1_compat_manifest.json` are removed.
- Grep for runtime bridge symbols such as `chartTemplateSets`, `legacyDimID`,
  `v2MetricName`, `writeChartMetrics`, `chartExpireAfterCycles`, and old
  `*ChartsTmpl` names returns no production hits.
- Grep for removed config keys under vSphere code/config/docs surfaces returns
  no non-SOW hits.
- `metadata.yaml`, `charts.yaml`, and taxonomy coverage agree on the current
  vSphere contexts.

Tests and checks run during final state:

- `go test -count=1 -timeout 300s ./collector/vsphere/...` passed from
  `src/go/plugin/go.d`.
- `go vet ./collector/vsphere/...` passed from `src/go/plugin/go.d`.
- `go test -count=1 -run '^Test_task' ./collector/vsphere` passed from
  `src/go/plugin/go.d`.
- `../../../../.venv/bin/python ../../../../integrations/check_collector_taxonomy.py --pr-diff upstream/master...HEAD`
  passed from `src/go/plugin/go.d`.
- `../../../../.venv/bin/python ../../../../integrations/gen_taxonomy.py --check-only`
  passed from `src/go/plugin/go.d`.
- Manual taxonomy ownership check reported
  `metadata=115 owned=115 referenced=4 missing=0 extra=0 duplicates=0`.
- `python3 -m json.tool collector/vsphere/config_schema.json` passed.
- YAML parse checks for `collector/vsphere/metadata.yaml`,
  `collector/vsphere/charts.yaml`, `collector/vsphere/taxonomy.yaml`, and stock
  `config/go.d/vsphere.conf` passed during the PR work.
- Generated integration markdown was regenerated when metadata/config changed.
- `git diff --check` passed for final touched files.

Reviewer findings:

- All accepted GitHub review feedback was fixed:
  readiness client-state check, opaque network topology IDs, discoverer pointer
  receiver warning dedup, datastore-cluster Storage DRS unknown state,
  dummy-cluster label detection, datastore writer exact metric count, and task
  lifecycle test synchronization.
- Findings rejected as out of scope or not applicable are reflected in the final
  out-of-scope section and specs.

Same-failure scans:

- Removed public config keys were searched across vSphere code, config, schema,
  metadata, stock config, and tests.
- Removed chart bridge symbols were searched across production Go code.
- Taxonomy contexts were compared against metadata contexts with missing, extra,
  and duplicate checks.

Sensitive data gate:

- Durable artifacts contain placeholders only for credentials and endpoints.
- No raw secrets, bearer tokens, private keys, session cookies, customer names,
  customer-identifying non-private IP addresses, private endpoints, or
  proprietary incident data were added.

Artifact maintenance gate:

- `AGENTS.md`: no final update required; existing collector consistency and SOW
  rules already cover this work.
- Runtime project skills: framework V2 collector guidance exists under
  `.agents/skills/project-writing-go-modules-framework-v2/`; integration
  lifecycle guidance was followed for taxonomy/metadata/doc artifacts.
- Specs: `.agents/sow/specs/vsphere-parity-matrix.md` records final parity
  classifications; `.agents/sow/specs/vsphere-v1-compatibility-manifest.md`
  records the superseded V1 baseline and current V2 validation replacements.
- End-user/operator docs: `metadata.yaml`, stock config, generated integration
  markdown, and health alerts were updated with the shipped config/metric
  surface.
- End-user/operator skills: no public operator skill changed because this PR
  changes collector behavior/docs, not AI skill workflows.
- SOW lifecycle: this closeout marks SOW-0015 `completed` and moves it to
  `.agents/sow/done/` with the closing commit.

Specs update:

- vSphere parity matrix is the current WHAT contract for included, excluded,
  covered-elsewhere, and non-metric surfaces.
- V1 compatibility manifest is explicitly superseded and retained only as
  historical baseline evidence.

Project skills update:

- Framework V2 collector skill captures reusable HOW-to-work guidance for this
  migration pattern.
- No additional skill update is required by the final SOW rewrite.

Documentation update:

- vSphere integration docs are generated from `metadata.yaml`.
- `README.md` follows the generated integration markdown symlink pattern.
- `taxonomy.yaml` is now present and passes the fatal taxonomy gate.

Lessons:

- High-cardinality child-instance surfaces should not be added only because
  vendor APIs expose them. They need a clear user need and bounded product
  contract.
- V2 migrations should move to direct metric-store assertions early; keeping a
  runtime V1 chart bridge makes tests and implementation harder to reason about.
- Collector taxonomy is a required source artifact when metric contexts change.
- Fixed sleeps in goroutine lifecycle tests should use explicit synchronization
  primitives when the implementation exposes them.

## Outcome

Implementation is complete for the approved PR scope. The collector now has a
clean framework V2 runtime, authoritative YAML chart/taxonomy artifacts,
approved parity/enrichment surfaces, and tests that assert the final V2 metric
store and chart-template behavior.

## Followup

No follow-up SOW is required for the implemented scope.

Excluded work that requires a separate user-approved SOW before implementation:

- vCenter/ESXi event/log ingestion;
- generated ESXi/VM/datastore vnodes;
- sensitive identity labels such as guest IP, inventory path, MAC, IQN, WWN, and
  datastore paths;
- per-child-instance VM/host metric families;
- deeper vSAN internals beyond the shipped opt-in subset;
- live vCenter permission probes in readiness;
- context propagation through all govmomi calls.

## Regression Log

No active regression remains for the final shipped scope.
