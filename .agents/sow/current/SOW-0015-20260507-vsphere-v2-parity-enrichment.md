# SOW-0015 - vSphere V2 Parity And Enrichment

## Status

Status: in-progress

Sub-state: Reopened on 2026-05-09 for PR CI regression triage and pre-merge hardening. Framework V2 migration, VM snapshot aggregate metrics/alerts, datastore aggregate enrichment, VM/host power-state controls, VM/host property-status metrics, cluster DRS/HA property-status metrics, inventory object counts, opt-in inventory/VM guest labels, opt-in vSphere tag/custom-attribute labels, opt-in VM disk capacity/performance, opt-in VM network-interface performance, opt-in host physical network-interface performance, opt-in host disk/LUN/device performance, opt-in host storage-adapter performance, opt-in host storage-path performance, opt-in host CPU-instance performance, opt-in host/VM power metrics, opt-in datastore cluster metrics, opt-in vSAN cluster space/health plus cluster/host/VM performance metrics, read-only readiness Function, cached vSphere inventory topology Function, and opt-in Network/DVPG topology discovery are implemented. Local fixes are in progress for yamllint, SonarCloud, DynCfg tab layout, metadata linting, supportable vSphere error messages, and hard removal of optional ESXi/VM vnode support before closing again. Remaining parity feasibility is complete; vCenter/ESXi events and collector-generated vSphere vnodes are explicitly out of this PR by user decision.

### 2026-05-20 Vnode Removal Decision

The user directed hard removal of vnode-related vSphere changes before merge.
This supersedes the earlier 2026-05-07/2026-05-08 decisions that allowed
default-off ESXi and VM vnode options.

Removal scope:

- delete the vnode implementation and tests;
- remove `esxi_vnodes` and `vm_vnodes` config, schema, metadata, stock-config,
  test-fixture, and readiness surfaces;
- remove `vcenterInstanceUUID` runtime plumbing used only by generated vnodes;
- remove all metric writer `metrix.HostScope` routing call sites instead of
  leaving stubs or disabled dead code;
- keep the existing job-level `vnode` configuration because it predates this PR
  and is not part of the generated ESXi/VM vnode feature.

Evidence and reason:

- The review found generated ESXi/VM vnode GUIDs were derived from vCenter
  managed-object references, which are vCenter-database-local and can change
  after vCenter rebuilds or object re-addition.
- The hard removal path has lower long-term maintenance risk than leaving
  default-off dead plumbing, and reintroducing vnodes later should happen in a
  focused design/identity PR.

Validation after removal:

- `src/go/plugin/go.d/collector/vsphere/config_schema.json` and
  `src/go/plugin/go.d/collector/vsphere/testdata/config.json` parsed with
  repo-root `.venv`.
- `src/go/plugin/go.d/collector/vsphere/metadata.yaml`,
  `src/go/plugin/go.d/collector/vsphere/testdata/config.yaml`, and
  `src/go/plugin/go.d/config/go.d/vsphere.conf` parsed with `ruamel.yaml` from
  repo-root `.venv`.
- Grep for `ESXIVnodes`, `VMVnodes`, `esxi_vnodes`, `vm_vnodes`,
  `vcenterInstanceUUID`, `resourceHostScope`, `HostScope`, `WithHostScope`,
  `esxiHostScope`, `vmHostScope`, `newResourceHostScope`, `_vnode_type`, and
  old scoped-gauge call signatures under the vSphere collector, stock config,
  and vSphere health file returned no matches.
- `go test -count=1 ./collector/vsphere/...` passed from
  `src/go/plugin/go.d`.

## Requirements

### Purpose

Make the Netdata vSphere collector fit for DevOps/SRE production use by moving it to the Go collector v2 framework while preserving existing users' metric contexts, dimensions, labels, configuration, and dynamic-configuration behavior. Existing users must only see more data after upgrade; chart IDs are the one accepted compatibility break for the V2 migration.

### User Request

The user requested:

- Move the vSphere collector to framework v2.
- Maintain 100% compatibility for existing metrics, contexts, dimensions, and labels.
- Maintain 100% backward compatibility for configuration and dynamic configuration.
- Add per-VM vCenter snapshot information: snapshot age, chain depth, and count.
- Add health alerts: warning when snapshot chain depth is greater than 3, critical when snapshot age is greater than 24 hours.
- Add datastore capacity, performance, and status information.
- Enrich Netdata's vSphere coverage to be equal to or a superset of LogicMonitor's VMware vSphere monitoring.
- Enrich Netdata's vSphere coverage to be equal to or a superset of Datadog's vSphere integration.
- Research other vSphere monitoring solutions in local mirrored repositories and make Netdata equal or a superset where feasible.
- Ensure the result is NIDL-framework friendly.
- Critical compatibility rule: existing vSphere users should collect more data without broken setups.

### Assistant Understanding

Facts:

- The existing collector is `src/go/plugin/go.d/collector/vsphere/`.
- One vSphere collector job targets one endpoint URL, normally a VMware vCenter Server, using credentials in `go.d/vsphere.conf`.
- The collector's monitored instance is documented as `VMware vCenter Server`, and the integration says it monitors hosts, VMs, datastores, clusters, and resource pools from vCenter servers.
- A single vCenter-backed job discovers all included datacenters, folders, clusters, ESXi hosts, VMs, datastores, and resource pools reachable through that vCenter.
- The collector is registered as framework v1 through `Create: func() collectorapi.CollectorV1 { return New() }` in `src/go/plugin/go.d/collector/vsphere/collector.go:26`.
- Existing collection returns a `map[string]int64` from `Collect()` in `src/go/plugin/go.d/collector/vsphere/collector.go:164`.
- Existing config keys are `vnode`, `update_every`, `autodetection_retry`, embedded web HTTP options, `discovery_interval`, `host_include`, `vm_include`, `datastore_include`, and `cluster_include` in `src/go/plugin/go.d/collector/vsphere/collector.go:66`.
- Existing discovery retrieves VM properties `name`, `runtime.host`, `runtime.powerState`, and `summary.overallStatus`, but not `snapshot`, in `src/go/plugin/go.d/collector/vsphere/discover/discover.go:99`.
- Existing VM metric selection is a small curated set: CPU usage, memory, network, summary disk metrics, and uptime in `src/go/plugin/go.d/collector/vsphere/discover/metric_lists.go:93`.
- Existing datastore coverage already includes capacity/free/used/used percentage, overall status, IOPS, latency, and read/write throughput in `src/go/plugin/go.d/collector/vsphere/collect.go:134` and `src/go/plugin/go.d/collector/vsphere/charts.go:434`.
- Existing datastore capacity and free-space values are only trusted when VMware reports the datastore as accessible in `src/go/plugin/go.d/collector/vsphere/collect.go:201`.
- The repository already has v2 collector patterns using `CreateV2`, `metrix.NewCollectorStore()`, and chart templates, for example `src/go/plugin/go.d/collector/powervault/collector.go:26`.
- The repository has a v2 host-scope spec that says unscoped metrics continue under the job-level vnode/global host, and per-target virtual nodes require stable `metrix.HostScope` values in `.agents/sow/specs/go-v2-host-scope.md:29` and `.agents/sow/specs/go-v2-host-scope.md:70`.
- Broadcom's current vSphere Web Services API documents VM snapshot hierarchy through `VirtualMachine.snapshot`, `VirtualMachineSnapshotInfo.rootSnapshotList`, and `VirtualMachineSnapshotTree.createTime` / `childSnapshotList`.
- Broadcom's current vSphere Web Services API documents `DatastoreSummary.accessible`, `capacity`, `freeSpace`, `uncommitted`, `type`, and `maintenanceMode`.
- Datadog's current vSphere docs state that the integration collects metrics and events, supports collection levels, and can collect property metrics.
- LogicMonitor's current vSphere docs state that its package covers vCenter, ESXi hosts, VMs, clusters, resource pools, datastores, topology, VM snapshots, VM disk capacity, VM interfaces, VM status, network state, HA, hardware sensors, and troubleshooting.

Inferences:

- A framework v2 migration is technically feasible because local v2 collector patterns already exist.
- 100% compatibility for the accepted surface is feasible only if the migration starts with an explicit compatibility manifest for every current context, dimension name, label key, config key, and the V1 chart IDs that intentionally change.
- Full Datadog/LogicMonitor parity cannot safely mean "enable every expensive VMware performance counter and every per-instance series by default" because Datadog, LogicMonitor, and Telegraf all expose collection-level/performance-load warnings or tuning knobs.
- NIDL-friendly enrichment likely requires grouping metrics by resource type and stable label sets instead of mixing VM, host, cluster, datastore, resource-pool, hardware-sensor, and event concepts in the same context.

Initial unknowns, now resolved or tracked below:

- Whether "equal or superset" should include vCenter events/log-like events in this collector SOW, or whether event ingestion belongs to a separate logs/events pipeline.
- Whether Netdata should create v2 host scopes / virtual nodes for ESXi hosts, VMs, datastores, or vCenter objects by default, or preserve current job-level emission and labels for compatibility.
- Whether high-cardinality per-instance data such as VM virtual disks, VM NICs, host NICs, storage adapters, storage paths, hardware sensors, tags, and custom attributes should be enabled by default or gated behind opt-in selectors.
- Whether snapshot `age` and `chain depth` should emit zero when a VM has no snapshots or omit those dimensions while still emitting count zero.

### Acceptance Criteria

- A compatibility manifest exists before implementation and lists every existing vSphere context, dimension ID/name, label key/value source, metric key, health alert, config key, schema property, stock config example, and V1 chart ID for traceability.
- The collector registers through framework v2 and uses v2 metric/chart mechanisms, while preserving existing vSphere contexts, dimension names, dimension algorithms/scales, old labels, and units. Chart IDs intentionally change to the framework V2 instance-ID format by user decision on 2026-05-08. V2 adds one stable `id` instance label required to derive per-resource instance chart IDs.
- Existing user YAML configs and dynamic-configuration JSON payloads accepted before this change still parse and behave the same with no required new keys.
- Existing include filters for hosts, VMs, datastores, and clusters keep their current semantics.
- Existing chart contexts and dimensions are still emitted for the same discovered objects with the same old labels when run against the same synthetic fixture; chart IDs intentionally change and V2 adds only `id` as an instance label.
- New metrics are additive and use new contexts/dimensions only.
- VM snapshot count, maximum snapshot age, and maximum snapshot chain depth are collected per VM from vCenter snapshot data.
- Health alerts warn when VM snapshot chain depth is greater than 3 and critical when VM snapshot age is greater than 24 hours.
- Datastore capacity, performance, and status metrics are preserved and enriched with any missing safe fields discovered in the parity matrix.
- A parity matrix maps LogicMonitor, Datadog, and mirrored open-source vSphere monitoring surfaces to Netdata support status: supported by existing chart, newly supported, opt-in supported, intentionally excluded with evidence, or impossible/unavailable with evidence.
- No parity matrix row remains `unknown` before implementation closes.
- New contexts are NIDL-friendly: one resource type per context, stable dimensions, bounded labels, deterministic units, no sensitive labels by default, and explicit cardinality controls.
- Tests cover config compatibility, dynamic config schema compatibility, chart compatibility, snapshot tree traversal, datastore accessibility semantics, discovery filters, and v2 chart-template generation.
- Real-use validation uses `govmomi` / `vcsim` or equivalent synthetic vSphere fixtures; no production vCenter is required.

## Analysis

Sources checked:

- `src/go/plugin/go.d/collector/vsphere/collector.go`
- `src/go/plugin/go.d/collector/vsphere/collect.go`
- `src/go/plugin/go.d/collector/vsphere/discover/discover.go`
- `src/go/plugin/go.d/collector/vsphere/discover/metric_lists.go`
- `src/go/plugin/go.d/collector/vsphere/charts.go`
- `src/go/plugin/go.d/collector/vsphere/metadata.yaml`
- `src/go/plugin/go.d/collector/vsphere/config_schema.json`
- `src/go/plugin/go.d/config/go.d/vsphere.conf`
- `src/health/health.d/vsphere.conf`
- `src/go/plugin/go.d/collector/powervault/collector.go`
- `src/go/plugin/go.d/collector/azure_monitor/collector_runtime.go`
- `.agents/sow/specs/go-v2-host-scope.md`
- `.agents/sow/specs/sensitive-data-discipline.md`
- Broadcom vSphere Web Services API, current latest:
  - `VirtualMachine`: https://developer.broadcom.com/xapis/vsphere-web-services-api/latest/vim.VirtualMachine.html
  - `VirtualMachineSnapshotInfo`: https://developer.broadcom.com/xapis/vsphere-web-services-api/latest/vim.vm.SnapshotInfo.html
  - `VirtualMachineSnapshotTree`: https://developer.broadcom.com/xapis/vsphere-web-services-api/latest/vim.vm.SnapshotTree.html
  - `DatastoreSummary`: https://developer.broadcom.com/xapis/vsphere-web-services-api/latest/vim.Datastore.Summary.html
  - `PerformanceManager`: https://developer.broadcom.com/xapis/vsphere-web-services-api/latest/vim.PerformanceManager.html
- Datadog vSphere docs: https://docs.datadoghq.com/integrations/vsphere/
- LogicMonitor VMware vSphere docs: https://www.logicmonitor.com/support/vmware-vsphere-monitoring
- Project skills:
  - `.agents/skills/project-writing-collectors/SKILL.md`
  - `.agents/skills/integrations-lifecycle/SKILL.md`
  - global `mirrored-repos` workflow

Current state:

- The vSphere collector is a v1 Go collector with dynamic charts and `map[string]int64` metric emission.
- The collector is centralized around the vCenter endpoint: one Netdata job can collect one vCenter inventory, including a cluster with many physical ESXi hosts, its VMs, datastores, and resource pools.
- Current coverage is useful but narrower than Datadog, LogicMonitor, Telegraf, Zabbix, Elastic, New Relic, and Grafana exporter references.
- Current VM discovery does not read snapshot data.
- Current datastore collection already has the core capacity/status/performance surface, but parity references expose additional status, queue, latency, and storage-capacity concepts.
- Current chart contexts and labels are public contracts and must be preserved.
- Current config is small and must remain valid in both stock config and dynamic configuration.

Framework v2 compatibility clarification:

- Framework v2 is not inherently backwards incompatible with v1 from a user-facing contract point of view.
- V2 changes the collector implementation contract:
  - V1 collectors return chart definitions through `Charts()` and samples through `Collect() map[string]int64`.
  - V2 collectors write samples into a `metrix.CollectorStore` and expose chart definitions through `ChartTemplateYAML()`.
- Configuration can remain backwards compatible:
  - both V1 and V2 use the same creator-level `JobConfigSchema` and `Config` wiring;
  - both expose `Configuration() any`;
  - existing YAML and dyncfg JSON keys can be preserved if the config struct and schema are preserved.
- Metric/chart compatibility remains backwards compatible for the accepted surface if the V2 chart template and metric-store series intentionally preserve every existing context, dimension name, algorithm, scaling, unit, old label key/value source, and metric meaning. Chart IDs are recorded for traceability but are not preserved. V2 adds `id=<managed-object-id>` as the instance label for stable chart materialization.
- Vnode compatibility can remain backwards compatible because v2 default-scope metrics continue under the job-level vnode only when one is configured; otherwise they stay under the global/default host scope.
- Therefore, the compatibility risk in this SOW is not "V2 breaks V1"; the risk is an incorrect migration that accidentally changes chart templates, metric selectors, dimensions, labels, lifecycle behavior, or host-scope routing.
- The implementation must include a golden compatibility harness to prove old config and old metric identity survive the V2 migration.

Compatibility harness design:

- Build a V1 compatibility manifest before rewriting the collector.
  - Generate it from the current collector using the existing `govmomi/simulator`-backed test fixture and mock scraper values.
  - Commit it as a stable test fixture.
  - The manifest must be sorted and deterministic.
- Manifest contents:
  - chart ID, recorded as the V1 identity that intentionally changes in the V2 migration;
  - chart context;
  - chart units, family, type, priority, and update interval where set;
  - chart labels and label values for representative VM, host, datastore, cluster, and resource-pool objects;
  - dimension ID;
  - dimension name;
  - dimension algorithm;
  - dimension multiplier/divisor;
  - hidden/obsolete/detail flags where externally visible;
  - metric sample key/value for every existing dimension in the fixture;
  - lifecycle expectations for charts that are created only after perf data arrives.
- Config compatibility tests:
  - keep the current YAML/JSON round-trip test using `testdata/config.yaml` and `testdata/config.json`;
  - add explicit old-config fixtures that omit every new field;
  - assert old YAML and old dyncfg JSON still parse into the same effective config;
  - assert no existing schema property is removed, renamed, made required, or changes type;
  - assert all new fields are optional and defaults preserve old behavior.
- V2 template compatibility tests:
  - compile `ChartTemplateYAML()` with the chartengine schema tests used by existing V2 collectors;
  - render V2 charts for the same simulator resources and compare the existing context/dimension/label/unit subset exactly with the V1 manifest, excluding chart IDs;
  - new charts are allowed only if their IDs/contexts are not present in the V1 manifest.
- V2 metric-store compatibility tests:
  - collect through the V2 `metrix.CollectorStore`;
  - read raw series from the default host scope;
  - compare every old V1 metric sample to the manifest after applying the same dimension naming/scaling rules;
  - fail if any old metric is missing, renamed, re-labeled, re-scoped, or has a changed value.
- Lifecycle compatibility tests:
  - preserve current behavior where datastore and cluster perf charts are created only after perf data is received;
  - preserve current obsolete-chart behavior for disappeared hosts, VMs, datastores, clusters, and resource pools;
  - verify V2 chart lifecycle obsoletes the corresponding resource/context charts even though chart IDs differ.
- Scope/vnode compatibility tests:
  - with no `vnode`, all existing metrics remain in the default/global scope;
  - with existing `vnode`, existing metrics remain under the configured job-level vnode behavior;
  - collector-generated ESXi/VM host scopes are excluded from this PR by the
    2026-05-20 hard-removal decision.
- Acceptance gate:
  - The V2 migration PR cannot proceed unless compatibility tests pass before enrichment metrics are added.
  - Enrichment tests must prove new metrics are additive and do not alter the V1 manifest subset.

Risks:

- Compatibility risk: framework v2 can change chart identity if chart templates, scope, vnode routing, or label values are not deliberately preserved.
- Performance risk: Datadog/LogicMonitor parity includes high-cardinality and high-cost categories; enabling all counters by default can overload vCenter or cause collection gaps.
- Cardinality risk: per-VM disks, per-VM NICs, per-host NICs, storage adapters, storage paths, hardware sensors, tags, custom attributes, and snapshot details can multiply series count.
- Sensitive-data risk: VM names, host names, datastore names, folder paths, inventory paths, tags, custom attributes, guest hostnames, guest IPs, and snapshot names/descriptions may identify customers or infrastructure.
- Scope risk: creating vnodes/host scopes for VMs or datastores by default could surprise users and may break current dashboard/chart assumptions.
- Parity-risk: "equal or superset" is not implementable safely until a normalized parity matrix defines exact Datadog/LogicMonitor/open-source coverage and default-vs-opt-in policy.
- Framework V2 chart-ID risk accepted by user decision: current chartengine instance IDs append label-derived suffixes after the literal chart ID, while vSphere V1 public chart IDs use the VMware managed-object ID as a prefix. A normal V2 `instances.by_labels` migration will therefore rename existing vSphere chart IDs and lose time-series continuity for by-instance views.

Framework V2 chart-ID compatibility blocker:

- vSphere V1 chart builders format resource IDs into chart and dimension IDs:
  - `src/go/plugin/go.d/collector/vsphere/charts.go:657` formats VM chart IDs as `<vm-id>_<chart>`.
  - `src/go/plugin/go.d/collector/vsphere/charts.go:683` formats host chart IDs as `<host-id>_<chart>`.
  - `src/go/plugin/go.d/collector/vsphere/charts.go:719` formats datastore chart IDs as `<datastore-id>_<chart>`.
  - `src/go/plugin/go.d/collector/vsphere/charts.go:1252` formats cluster chart IDs as `<cluster-id>_<chart>`.
  - `src/go/plugin/go.d/collector/vsphere/charts.go:1266` formats resource-pool chart IDs as `<resource-pool-id>_<chart>`.
- Current chartengine templates are literal-only:
  - `src/go/plugin/framework/chartengine/internal/program/placeholders.go:5` says templates are for chart IDs and dimension names.
  - `src/go/plugin/framework/chartengine/internal/program/placeholders.go:7` says phase-1 templates are literal-only without placeholder/transform syntax.
  - `src/go/plugin/framework/chartengine/template_parse.go:12` through `src/go/plugin/framework/chartengine/template_parse.go:19` stores only the trimmed raw string.
- Current V2 instance rendering appends the instance suffix after the base chart ID:
  - `src/go/plugin/framework/chartengine/identity.go:59` renders the base literal ID.
  - `src/go/plugin/framework/chartengine/identity.go:67` renders an instance suffix.
  - `src/go/plugin/framework/chartengine/identity.go:77` returns `baseID + suffix`.
  - `src/go/plugin/framework/chartengine/identity.go:114` builds suffixes as `"_" + strings.Join(parts, "_")`.
  - `src/go/plugin/framework/chartengine/planner_test.go:533` asserts `win_nic_traffic_eth0`.
- Current V2 job runtime loads a collector's `ChartTemplateYAML()` once after `Check()`:
  - `src/go/plugin/framework/jobruntime/job_v2.go:382` reads the template.
  - `src/go/plugin/framework/jobruntime/job_v2.go:390` stores it with revision `1`.
- Conclusion: without a framework extension, V2 can express `cpu_usage_total_host-21`, but not the existing public ID `host-21_cpu_usage_total` for dynamically discovered resources. The user accepted this chart-ID break on 2026-05-08 to avoid framework changes the maintainer is unlikely to accept.

High-cardinality definition for this SOW:

- Existing per-object vSphere metrics for VMs, ESXi hosts, datastores, clusters, and resource pools are current behavior and are not considered new opt-in high cardinality by themselves.
- New aggregate per-object metrics with stable labels are also candidates for default collection when they reuse the existing object cardinality class. Examples: per-VM snapshot `count`, `max_age`, and `max_chain_depth`; datastore aggregate `accessible`, `maintenance_mode`, `capacity`, `free`, `used`, `uncommitted`, and aggregate latency/throughput/IOPS.
- High-cardinality metrics are new signals whose series count scales as `resource_count * child_instance_count`, or whose label values are user-defined/unstable/sensitive. These need opt-in selectors or explicit bounds unless the user accepts the operational cost.
- High-cardinality metric groups include:
  - per-VM virtual disk performance and capacity by disk/device instance;
  - per-VM network interface throughput, packets, errors, and drops by NIC/backing instance;
  - per-host physical NIC metrics by NIC instance;
  - per-host disk-device metrics by disk/LUN/device instance;
  - per-host storage adapter and storage path metrics by adapter/path instance;
  - per-host CPU core/thread metrics by CPU instance;
  - hardware health sensors by sensor instance, including fans, power, processor, memory, and storage sensors;
  - vSAN performance entities, including disk groups, cache/capacity disks, DOM/client/owner, and host/cluster vSAN objects;
  - vSphere tags, custom attributes, inventory paths, guest hostnames, guest IP addresses, and other user-defined metadata if emitted as labels or host-scope identity;
  - per-snapshot details such as snapshot name, description, ID, state, and create-time labels or dimensions;
  - topology edges and network/distributed-port-group objects if emitted as metric labels or per-edge metrics.
- Events are not metrics, but vCenter event ingestion is high-volume/high-cardinality operationally and should be treated as a separate surface unless explicitly included.

Cardinality sizing model:

- Use these symbols:
  - `V` = VMs discovered by the collector.
  - `H` = ESXi hosts discovered by the collector.
  - `D` = datastores discovered by the collector.
  - `C` = clusters discovered by the collector.
  - `RP` = resource pools discovered by the collector.
  - `vdisk` = average virtual disks per VM.
  - `vnic` = average virtual NICs per VM.
  - `snap` = average snapshots per VM.
  - `pnic` = average physical NICs per host.
  - `lun` = average LUNs/devices/datastores visible per host.
  - `path` = average storage paths visible per host.
  - `hba` = average storage adapters per host.
  - `sensor` = average hardware sensors per host.
- Current Netdata object-level fanout is roughly:
  - VM metrics: about `23 * V` dimensions before new work.
  - Host metrics: about `25 * H` dimensions before new work.
  - Datastore metrics: up to about `14 * D` dimensions when datastore perf counters are available.
  - Cluster/resource-pool metrics add their own per-object fanout, but they do not multiply by child device instances today.
- New safe aggregate additions are still object-level:
  - VM snapshot aggregate metrics: about `3 * V` dimensions.
  - Datastore extra aggregate status/capacity fields: about `small_number * D` dimensions.
- High-cardinality child-instance fanout examples:
  - VM virtual disk metrics: `V * vdisk * metrics_per_vdisk`. Telegraf's common VM list has 9 virtual-disk metrics; Datadog's broader VM list has more.
  - VM NIC metrics: `V * vnic * metrics_per_vnic`. Zabbix's VM NIC discovery has at least 5 item prototypes for bytes, packets, and usage.
  - VM per-snapshot details: `V * snap * metrics_or_labels_per_snapshot`. Broadcom supports a maximum of 32 snapshots in a chain and recommends 2 to 3 for performance.
  - Host physical NIC metrics: `H * pnic * metrics_per_pnic`.
  - Host LUN/datastore/device metrics: `H * lun * metrics_per_lun`. Broadcom KB evidence for ESXi 7/8 states a maximum of 1024 Fibre Channel/SCSI LUNs per host.
  - Host storage path metrics: `H * path * metrics_per_path`. Broadcom KB evidence for ESXi 8/9 states a maximum of 4096 physical storage paths per host.
  - Host storage adapter metrics: `H * hba * metrics_per_hba`. Datadog has 12 storage-adapter metrics in the checked source.
  - Hardware sensor metrics: `H * sensor * metrics_per_sensor`; the sensor count is vendor/hardware dependent and has no portable small bound.
  - vSAN metrics: `clusters/hosts * disk_groups * disks/components * metrics`; the count depends on vSAN topology and can become much larger than normal datastore aggregates.
  - Tags/custom attributes do not always add metric dimensions directly, but if used as labels they create unbounded user-defined label-value cardinality and may expose sensitive infrastructure names.

Example fanout, not a requirement:

- A moderate vCenter with `V=1000`, `H=50`, `D=100`, `vdisk=3`, `vnic=2`, `lun=100`, `path=400`, `hba=4`, `sensor=80`:
  - current VM dimensions: about `23,000`;
  - current host dimensions: about `1,250`;
  - current datastore dimensions: up to about `1,400`;
  - safe snapshot aggregate: about `3,000`;
  - VM disk metrics using 9 metrics/disk: about `27,000`;
  - VM NIC metrics using 5 metrics/NIC: about `10,000`;
  - host LUN metrics using 10 metrics/LUN: about `50,000`;
  - host path metrics using 10 metrics/path: about `200,000`;
  - hardware sensor metrics using 2 metrics/sensor: about `8,000`.
- The important point is that host storage paths alone can exceed the entire existing collector surface in a medium environment.

Competitor default-policy evidence:

- Datadog is mostly conservative by default:
  - `collection_level` defaults to `1`; level `4` means every available metric, with warnings about slow and CPU-intensive collection.
  - `collect_per_instance_filters` is optional and warns that per-instance metrics can be very expensive in big environments.
  - `collect_tags`, `collect_attributes`, `collect_property_metrics`, and `collect_vsan_data` default to `false`.
  - `collect_events` defaults to true for realtime collection, but that is an event surface, not metric cardinality.
- Telegraf is mixed:
  - VM and host instances default to true in the documented example.
  - cluster, resource pool, datastore, and datacenter instances default to false.
  - vSAN is disabled by default.
  - custom attributes are disabled by default because they can add a considerable number of tags.
- New Relic is conservative about per-instance metric cardinality:
  - users select performance levels and metric files;
  - the README warns that enabling more performance metrics adds load;
  - per-instance values are averaged into a single sample instead of emitted as separate instance-tagged series.
- Zabbix is discovery/template driven:
  - VM snapshots, VM network interfaces, VM disks, VM storage, and VMware Tools status are template/discovery items.
  - Once the template/discovery is active, per-VM disk and NIC item prototypes create per-instance metrics.
- LogicMonitor public docs show module/DataSource-level coverage rather than a clear per-metric opt-in model:
  - high-fanout areas such as VM snapshots, VM disk capacity, VM interfaces, hardware sensors, topology, datastore status, and datastore throughput are separate LogicModules/DataSources.
  - the docs warn that running old and new modules together creates duplicate data/alerts and extra vCenter load.
  - Public evidence is insufficient to claim that each high-cardinality metric is opt-in inside LogicMonitor; the safe statement is that coverage is package/module/DataSource controlled.

Initial parity target checklist:

- This checklist is the first normalized definition of "parity" for this SOW.
- It is not yet the final metric-by-metric implementation matrix.
- Before code changes, each row below must be expanded into a parity matrix row set that maps external signals to Netdata support status:
  - `existing-default`: already collected by current Netdata vSphere collector.
  - `new-default`: additive object-level metric/property safe to collect for existing users.
  - `opt-in`: high-cardinality, expensive, sensitive, or advanced collection.
  - `covered-elsewhere`: already covered by another Netdata collector or surface.
  - `follow-up`: valid parity requirement, but not a metric-collector change in this SOW.
  - `excluded`: intentionally not supported, with evidence and product reason.
- No final parity matrix row may remain `unknown` before implementation closes.

Parity evidence anchors:

- Datadog current docs state that the vSphere check collects cluster resource usage metrics for CPU, disk, memory, and network, and watches vCenter events.
- Datadog current docs state that collected metrics depend on `collection_level`.
- Datadog current docs state that per-instance metrics are ignored by default and require `collect_per_instance_filters`.
- Datadog current docs state that property metrics, such as host maintenance mode and cluster DRS configuration, require `collect_property_metrics: true`.
- Datadog current docs list event collection through vCenter Event Manager, including alarm-status changes, VM migration/reconfiguration/power events, task events, and VM messages.
- LogicMonitor current docs organize VMware vSphere monitoring into ESXi, vCenter Appliance, and vSphere module categories.
- LogicMonitor current docs list ESXi coverage for CPU, datastore throughput/usage, disks, hardware health sensors, logical processors, memory, network interfaces, network state, power, VM disk capacity, VM performance, VM snapshots, and VM status.
- LogicMonitor current docs list vCenter Appliance coverage for service health, HA status, memory, network interfaces, power, services, and alarms.
- LogicMonitor current docs list vSphere coverage for clusters, datastore clusters, datastore status, datastore throughput, datastore usage, HA/admission control, host status, network state, resource pools, VM disk capacity, VM network interfaces, VM performance, VM snapshots, VM status, and topology.
- Mirrored Datadog, Telegraf, Grafana `vmware_exporter`, Elastic Beats, Zabbix, and New Relic sources confirm the same broad surface: performance counters, property/status metrics, snapshots, storage capacity, per-device/per-interface metrics, custom metadata, events, vSAN, and simulator-backed tests.

Parity target groups:

1. Compatibility baseline
   - Target: preserve every existing Netdata vSphere context, dimension, label, unit, config key, schema key, stock config example, alert, discovery filter, and lifecycle behavior. V1 chart IDs are recorded but intentionally not preserved.
   - SOW status: mandatory `new-default` gate through the compatibility manifest.
   - Notes: this is a parity group because "existing Netdata" is also a competitor to the new implementation; breaking it fails the user requirement.

2. vCenter / VCSA appliance health
   - Target: VCSA system/applmgmt/load/memory/swap/database-storage/storage/software-package health, services, VCHA/HA where available, VCSA network and memory where available.
   - SOW status: mostly `covered-elsewhere`.
   - Evidence: Netdata already has the `vcsa` collector for VCSA health contexts; LogicMonitor exposes these as vCenter Appliance modules.
   - Notes: do not duplicate VCSA service-health metrics in the vSphere collector unless a matrix row proves a vSphere-only gap.

3. Inventory, discovery, and identity
   - Target: vCenter, datacenters, folders, clusters, ESXi hosts, VMs, datastores, datastore clusters where available, resource pools, object status, and stable identity labels.
   - SOW status: mixed `existing-default` and `new-default`.
   - Notes: sensitive or unstable identity data such as inventory paths, tags, custom attributes, guest hostname, and guest IP are `opt-in`.

4. VM aggregate performance
   - Target: VM CPU, memory, network, disk, uptime/heartbeat, and power/availability signals at VM object level.
   - SOW status: current CPU/memory/network/disk/uptime subset is `existing-default`; missing safe object-level counters from Datadog/Telegraf/Zabbix become `new-default` only if they do not require per-child instances or high collection levels.
   - Candidate gaps: CPU demand/ready/readiness/latency/wait/run/usage MHz, memory balloon/compressed/overhead/host/guest/latency signals, heartbeat, power, and aggregate disk/network usage/contention where available.
   - Notes: per-virtual-disk and per-vNIC metrics are separate `opt-in` groups.

5. VM status and properties
   - Target: power state, overall status, connection/guest/tools status, tools version where safe, boot time or time since power-on, fault tolerance status where available, consolidation-needed status, committed/uncommitted/unshared storage.
   - SOW status: mixed `existing-default`, `new-default`, and `opt-in`.
   - Notes: guest hostname/IP and custom attributes are not default labels.

6. VM snapshots
   - Target: per-VM aggregate snapshot count, maximum snapshot age, maximum snapshot chain depth, and health alerts for age/depth thresholds.
   - SOW status: `new-default`.
   - Notes: per-snapshot name, description, ID, state, tree nodes, and create-time labels are `opt-in` or `excluded` from metrics by default because they are high-cardinality and sensitive.

7. VM virtual disks
   - Target: per-virtual-disk capacity, read/write throughput, IOPS, latency, outstanding IO, load, and provisioned/used where available.
   - SOW status: `opt-in`.
   - Notes: required for LogicMonitor/Zabbix/Datadog/Telegraf parity, but it multiplies by `V * vdisk`.

8. VM network interfaces
   - Target: per-vNIC bytes, packets, drops/errors, usage, and interface identity where safe.
   - SOW status: `opt-in`.
   - Notes: required for LogicMonitor/Zabbix parity, but it multiplies by `V * vnic` and can expose network topology labels.

9. ESXi host aggregate performance
   - Target: host CPU, memory, disk, network, uptime, power/energy, connection state, maintenance mode, and overall health/status at host object level.
   - SOW status: current aggregate CPU/memory/disk/network/uptime subset is `existing-default`; safe missing object-level status/property metrics are `new-default`.
   - Candidate gaps: CPU demand/ready/readiness/latency/wait/costop/capacity/contention, memory state/capacity/latency/balloon/compression/VMFS PBC where safe, disk queue/device/kernel latency aggregates, power usage, connection and maintenance mode.

10. ESXi child-instance metrics
    - Target: physical NICs, logical processors, host disks/devices/LUNs, storage adapters, storage paths, and hardware health sensors.
    - SOW status: `opt-in`.
    - Notes: required for Datadog/Telegraf/LogicMonitor/Zabbix parity, but these multiply by host child objects and may exceed the current collector surface in medium environments.

11. Datastores and datastore clusters
    - Target: datastore capacity, free, used, used percentage, uncommitted/provisioned where available, accessible, maintenance mode, overall status, throughput, IOPS, latency, contention/usage where available, and datastore-cluster capacity where available.
    - SOW status: current datastore capacity/status/perf subset is `existing-default`; missing safe aggregate fields are `new-default`; per-host datastore views are `opt-in`.
    - Notes: keep the existing datastore accessibility guard because VMware says capacity/free-space values are valid only when accessible.

12. Clusters, HA, DRS, and resource pools
    - Target: cluster CPU/memory capacity/effective usage, HA/failover/admission-control state, DRS enabled/default behavior/vmotion rate where safe, cluster status, resource-pool CPU/memory usage/allocation/shares/limits/reservations where available.
    - SOW status: mixed `existing-default` and `new-default`.
    - Notes: property metrics with user-defined labels or high collection cost need explicit gating.

13. Network state and topology
    - Target: vCenter/ESXi network state, distributed port group state, and topology relationships among clusters, hosts, VMs, and datastores.
    - SOW status: metric-safe status counters may be `new-default`; topology graph output is `follow-up` unless this SOW explicitly adopts a topology surface.
    - Notes: do not force topology edges into metric labels if that creates high-cardinality or unstable labels.

14. vSAN
    - Target: vSAN cluster, host, disk-group, disk, DOM/client/cache/capacity, health, latency, throughput, IOPS, and component metrics.
    - SOW status: `opt-in` or `follow-up`.
    - Notes: Datadog and Telegraf both treat vSAN as disabled/advanced by default in checked sources; it is too large for the default-safe core.

15. Tags, custom attributes, guest identity, and inventory paths
    - Target: vSphere tags, custom fields/attributes, inventory path, guest hostname, guest IP, and other operator-defined metadata.
    - SOW status: `opt-in`.
    - Notes: these are useful for filtering and correlation, but they are user-defined and may expose sensitive infrastructure names.

16. Events, alarms, and logs
    - Target: vCenter alarms, alarm status changes, VM migrations, power events, reconfiguration events, tasks, VM messages, and other `vim.event` classes.
    - SOW status: `follow-up`.
    - Notes: events are valid parity scope, but they are not the same VMware API path as performance metrics and should use Netdata logs/OTEL/event ingestion design.

17. Collector self-observability and troubleshooting
    - Target: can-connect status, API/query timing, cache refresh timing, discovery timing, skipped-resource counts, and clear diagnostics for permission/statistics-level gaps.
    - SOW status: partly existing through go.d job health/logging; explicit metric parity is `new-default` only if it fits Netdata collector conventions.
    - Notes: LogicMonitor has a troubleshooter module; Datadog exposes query and collection timing metrics.

18. Configuration and control plane
    - Target: backward-compatible old config and dyncfg, plus new optional collection groups, selectors, limits, discovery interval controls, query object/metric limits, concurrency controls, event controls for follow-up, and high-cardinality opt-in switches.
    - SOW status: backward compatibility is mandatory `existing-default`; new controls are `new-default` only where they protect default-safe behavior, otherwise `opt-in`.
    - Notes: no existing config key may be removed, renamed, made required, or changed in type.

Local go.d overlap scan:

- Search scope:
  - go.d collector directories under `src/go/plugin/go.d/collector/`;
  - stock go.d configs under `src/go/plugin/go.d/config/go.d/`;
  - go.d SNMP stock profiles under `src/go/plugin/go.d/config/go.d/snmp.profiles/`;
  - go.d health files under `src/health/health.d/`.
- Direct VMware/vSphere collectors found:
  - `vsphere`: the current target collector.
  - `vcsa`: a separate vCenter Server Appliance health collector.
- `vcsa` overlap:
  - `src/go/plugin/go.d/collector/vcsa/metadata.yaml:8` declares the monitored instance as "vCenter Server Appliance".
  - `src/go/plugin/go.d/collector/vcsa/metadata.yaml:23` says it monitors health statistics of vCenter Server Appliance servers.
  - `src/go/plugin/go.d/collector/vcsa/charts.go:19` through `src/go/plugin/go.d/collector/vcsa/charts.go:129` define VCSA health contexts for system, appliance management, load, memory, swap, database storage, storage, and software packages.
  - Implication: VCSA health rows in the parity matrix should default to `covered-elsewhere` unless a specific LogicMonitor/Datadog row proves a VCSA gap that belongs in `vcsa`, not `vsphere`.
- `snmp` / ESX overlap:
  - `src/go/plugin/go.d/config/go.d/snmp.profiles/default/vmware-esx.yaml:6` selects VMware ESX/ESXi devices by VMware sysObjectID.
  - `src/go/plugin/go.d/config/go.d/snmp.profiles/default/vmware-esx.yaml:11` through `src/go/plugin/go.d/config/go.d/snmp.profiles/default/vmware-esx.yaml:17` marks the device as vendor `VMware`, type `Server`.
  - `src/go/plugin/go.d/config/go.d/snmp.profiles/default/vmware-esx.yaml:27` through `src/go/plugin/go.d/config/go.d/snmp.profiles/default/vmware-esx.yaml:49` collect VMware HBA status from `VMWARE-RESOURCES-MIB`.
  - `src/go/plugin/go.d/config/go.d/snmp.profiles/default/vmware-esx.yaml:51` through `src/go/plugin/go.d/config/go.d/snmp.profiles/default/vmware-esx.yaml:85` collect VMware hardware/component status from `VMWARE-ENV-MIB`, including chassis, power supply, fan, CPU, memory, battery, temperature sensor, RAID controller, and voltage component types.
  - `src/go/plugin/go.d/collector/snmp/metadata.yaml:169` through `src/go/plugin/go.d/collector/snmp/metadata.yaml:217` describe SNMP's profile-based model: auto-detect device, choose profile, and collect exactly the profile OIDs.
  - Implication: ESXi hardware/HBA/environment health is partially `covered-elsewhere` by SNMP when ESXi SNMP is enabled per host. It is not a replacement for vCenter-wide vSphere inventory, VM, datastore, cluster, resource-pool, snapshot, or vCenter-property collection.
- `prometheus` overlap:
  - `src/go/plugin/go.d/collector/prometheus/metadata.yaml:5` through `src/go/plugin/go.d/collector/prometheus/metadata.yaml:27` define only the generic Prometheus endpoint collector.
  - Literal `vmware`, `vsphere`, `vcenter`, `esxi`, and `esx` searches found no VMware-specific Prometheus integration file under the go.d Prometheus collector.
  - Implication: Prometheus can scrape third-party VMware exporters if a user configures them, but it is not an in-tree VMware parity surface for this SOW.
- Non-overlaps from `esx` search:
  - SNMP profile metadata contains unrelated `ESX-*` model names for Enterasys/Marconi devices.
  - These are not VMware ESX/ESXi monitoring surfaces and should not affect the vSphere parity matrix.

Snapshot no-data behavior in checked implementations:

- Zabbix uses explicit zero/null sentinel values when snapshot data is missing:
  - `zabbix/zabbix @ 980ed807b324dbf333260a449cc7430c0a978aaf`
  - `src/libs/zbxvmware/vmware_vm.c:1153` sets a JSON payload with `snapshot:[]`, `count:0`, `latestdate:null`, `latestage:0`, `oldestdate:null`, `oldestage:0`, `size:0`, and `uniquesize:0`.
  - Zabbix's template extracts `$.count` and `$.latestdate` from the snapshot payload in `templates/app/vmware/template_app_vmware.yaml:1498`.
- Elastic Beats omits snapshot fields when there are no snapshots:
  - `elastic/beats @ fd8ae04b57a40892640a4db87823bdf384c873d3`
  - `metricbeat/module/vsphere/virtualmachine/virtualmachine.go:241` only fills snapshots when `vm.Snapshot != nil`.
  - `metricbeat/module/vsphere/virtualmachine/data.go:75` only emits `snapshot.count` and `snapshot.info` when `len(data.Snapshots) > 0`.
- New Relic snapshot collection is opt-in and emits per-snapshot samples only when snapshots exist:
  - `newrelic/nri-vsphere @ 9366fcd3d597ae0712c94882331042d74fe38e22`
  - `internal/config/config.go:48` defaults `EnableVsphereSnapshots` to `false`.
  - `internal/process/vms.go:220` processes snapshots only when `vm.Snapshot != nil`, `vm.LayoutEx != nil`, and snapshot collection is enabled.
  - `internal/process/snapshot.go:245` creates one sample per snapshot tree entry.
- Datadog, Telegraf, and Grafana `vmware_exporter` sources checked for this SOW do not show VM snapshot metrics in their vSphere metric collectors.

Snapshot no-data recommendation from evidence:

- For Netdata charts and alerts, Zabbix's explicit zero/null model maps better than Elastic/New Relic's omit-when-empty model because Netdata needs stable dimensions and simple alert expressions.
- Netdata should emit `snapshot_count=0`, `snapshot_max_age=0`, and `snapshot_max_chain_depth=0` for VMs with no snapshots, and documentation must state that zero age/depth means "no snapshot exists".

vCenter events clarification:

- "Events are not rejected" means they remain part of the broader vSphere parity/product goal, but they are deferred out of this metrics-focused collector SOW.
- It does not mean metrics and events should be fetched through one VMware API call.
- Datadog evidence shows the distinction:
  - `DataDog/integrations-core @ c84ac0c95e3afc8996276977886c651b37c7cdc4`
  - `vsphere/datadog_checks/vsphere/config.py:93` configures `collect_events_only` and `collect_events` separately from metric collection.
  - `vsphere/datadog_checks/vsphere/vsphere.py:1111` collects/submits events as a separate step.
  - `vsphere/datadog_checks/vsphere/api.py:330` fetches events through `content.eventManager` and `EventFilterSpec`, not the performance metric query path.
  - `vsphere/datadog_checks/vsphere/api.py:364` has a fallback using `CreateCollectorForEvents` and `ReadNextEvents`.
- For Netdata, the likely future design is:
  - reuse vCenter connection/config where practical;
  - query vCenter events separately from performance/property metrics;
  - transform them into an OTEL/logs ingestion shape with filtering, cursoring, deduplication, and sensitive-data handling;
  - keep that implementation in a separate SOW because it has different contracts from chart metrics.

Proposed Netdata default policy:

- Do not make every new metric opt-in. That would technically preserve compatibility, but it would violate the user expectation that existing users should automatically collect more useful data after upgrade.
- Add new metrics by default when all of these are true:
  - same object-level cardinality as current charts: per VM, per host, per datastore, per cluster, or per resource pool;
  - no child-instance multiplier such as disk, NIC, LUN, path, HBA, sensor, vSAN disk, or individual snapshot;
  - no user-defined or sensitive label values such as tags, custom attributes, inventory path, guest IP, guest hostname, snapshot name, or snapshot description;
  - low API cost through already collected or cheap property/performance calls;
  - stable NIDL context, dimensions, units, and labels.
- Candidate default additions:
  - per-VM snapshot aggregate `count`, `max_age`, and `max_chain_depth`;
  - VM snapshot health alerts for chain depth and age;
  - datastore aggregate status/capacity fields not already covered, such as `accessible`, `maintenance_mode`, and `uncommitted`, subject to API availability and docs;
  - low-cost VM/host/cluster status/property metrics such as power/connection/maintenance/HA/DRS/tool status only if they are stable aggregate object properties and not per-child objects.
- Make new metrics opt-in when any of these are true:
  - per-child instance metrics: VM disks, VM NICs, host NICs, host disks/LUNs/devices, storage adapters, storage paths, hardware sensors, vSAN disk groups/disks/components, individual snapshots;
  - user metadata as labels: vSphere tags, custom attributes, inventory paths, guest hostnames, guest IPs;
  - expensive collection level, broad property metrics, historical queries with high `maxQueryMetrics`, vSAN, or event collection;
  - metric families whose safe default needs real-world cardinality data first.

## Pre-Implementation Gate

Status: ready-for-implementation

Implementation readiness checklist:

- Preparatory implementation completed:
  - Drafted `.agents/sow/specs/vsphere-v1-compatibility-manifest.md` from the current collector and shipped artifacts.
  - Added executable V1 golden baseline `src/go/plugin/go.d/collector/vsphere/testdata/v1_compat_manifest.json` and `TestCollector_V1CompatibilityManifest`.
  - Drafted `.agents/sow/specs/vsphere-parity-matrix.md` with no `unknown` rows.
  - These two tasks did not change collector behavior.
- Ready to start behavior-changing implementation because:
  - the V1 compatibility manifest is recorded;
  - the parity matrix is recorded with classified rows;
  - the executable compatibility baseline will treat chart IDs as intentionally breaking while preserving contexts, dimensions, labels, config, and dyncfg;
  - the first behavior-changing commit is the framework v2 migration, with no enrichment mixed in.

Success assurance gates:

- Compatibility gate:
  - old config YAML and dyncfg JSON still parse;
  - no existing config key is removed, renamed, made required, or type-changed;
  - every old context, dimension name, algorithm, scale, unit, and label key/value matches the manifest; chart IDs intentionally differ;
  - V2 adds only one new label, `id`, equal to the vSphere managed-object reference used as the old V1 chart-ID prefix.
- Parity gate:
  - every LogicMonitor, Datadog, Telegraf, Zabbix, Elastic, New Relic, Grafana exporter, VCSA, and SNMP `vmware-esx` row is classified;
  - rows classified as `new-default` or `opt-in` have matching tests or explicit follow-up tracking.
- NIDL/cardinality gate:
  - default additions stay object-level;
  - child-instance metrics are opt-in with selectors/limits;
  - sensitive labels are off by default.
- Runtime correctness gate:
  - snapshot traversal tests cover empty snapshots, sibling snapshots, nested chains, and old create times;
  - datastore tests cover accessible/inaccessible semantics and capacity/free/uncommitted behavior;
  - discovery and obsolete-chart behavior remains compatible.
- Real-use gate:
  - `govmomi` / `vcsim` or equivalent synthetic vSphere validation runs against the collector;
  - no production vCenter data is needed or stored.
- Artifact gate:
  - code, metadata, schema, stock config, health alerts, README/docs, specs, and this SOW stay in sync.

Brutal truth:

- We cannot prove success by inspection alone.
- The work becomes low-risk only if the compatibility manifest is the first artifact and the v2 migration is forced to pass it before any enrichment is added.
- The parity matrix is the second protection: it prevents vague "parity" scope from leaking uncontrolled metrics into defaults.

Problem / root-cause model:

- The existing vSphere collector predates framework v2 and emits metrics through v1 APIs.
- The existing collector collects a curated subset of vSphere metrics and properties.
- The requested enrichment is much larger than a mechanical v2 migration because competitor parity includes properties, events, topology, hardware sensors, VM snapshots, VM disk capacity, VM interfaces, host interfaces, storage adapters/paths, tags, custom attributes, vSAN, and optional high collection levels.
- The root compatibility challenge is preserving current public metric meaning and NIDL identity, except chart IDs, while adding a larger, bounded, NIDL-friendly metric surface.

Evidence reviewed:

- `src/go/plugin/go.d/collector/vsphere/collector.go:26` registers vSphere with v1 `Create`.
- `src/go/plugin/go.d/collector/vsphere/collector.go:66` defines current public config fields.
- `src/go/plugin/go.d/collector/vsphere/collector.go:164` returns v1 `map[string]int64`.
- `src/go/plugin/go.d/collector/vsphere/discover/discover.go:99` defines discovery property sets and lacks `snapshot`.
- `src/go/plugin/go.d/collector/vsphere/discover/metric_lists.go:93` defines the current VM metric set.
- `src/go/plugin/go.d/collector/vsphere/discover/metric_lists.go:122` defines the current datastore metric set and notes Level 1 / Level 2 counters.
- `src/go/plugin/go.d/collector/vsphere/collect.go:201` intentionally guards datastore capacity/free-space when inaccessible.
- `src/go/plugin/go.d/collector/vsphere/charts.go:107` and following define existing VM chart contexts and dimensions.
- `src/go/plugin/go.d/collector/vsphere/charts.go:281` and following define existing host chart contexts and dimensions.
- `src/go/plugin/go.d/collector/vsphere/charts.go:434` and following define existing datastore chart contexts and dimensions.
- `src/go/plugin/go.d/collector/powervault/collector.go:26` shows the local v2 `CreateV2` pattern.
- `src/go/plugin/go.d/collector/azure_monitor/collector_runtime.go:50` shows local use of v2 `metrix.HostScope`.
- `.agents/sow/specs/go-v2-host-scope.md:70` defines the v2 host-scope collector contract.
- Broadcom `VirtualMachineSnapshotInfo` documents snapshot hierarchy via `rootSnapshotList`.
- Broadcom `VirtualMachineSnapshotTree` documents `childSnapshotList`, `createTime`, and snapshot identity fields.
- Broadcom `DatastoreSummary` documents `accessible`, `capacity`, `freeSpace`, `uncommitted`, `type`, and `maintenanceMode`, and warns that capacity/free-space validity depends on accessibility.
- Datadog docs state that vSphere collection includes metrics/events, collection levels, property metrics, and event filtering.
- LogicMonitor docs list vSphere modules covering snapshots, datastore status/usage/throughput, VM disk capacity, VM interfaces, VM status, HA, network state, topology, and hardware sensors.
- Mirrored open-source references are listed below.

Affected contracts and surfaces:

- Go collector code under `src/go/plugin/go.d/collector/vsphere/`.
- Stock config at `src/go/plugin/go.d/config/go.d/vsphere.conf`.
- Dynamic configuration schema at `src/go/plugin/go.d/collector/vsphere/config_schema.json`.
- Integration metadata at `src/go/plugin/go.d/collector/vsphere/metadata.yaml`.
- Health alerts at `src/health/health.d/vsphere.conf`.
- Collector tests under `src/go/plugin/go.d/collector/vsphere/`.
- Generated integrations artifacts; hand-editing generated `integrations/*.md` files is not expected.
- Public metric contracts: contexts, dimensions, labels, units, health-template names, and the accepted chart-ID break.
- NIDL-facing context design.

Existing patterns to reuse:

- v2 collector registration and `metrix.CollectorStore` from `powervault`.
- v2 host scope / virtual-node model from the project spec and `azure_monitor`.
- Existing vSphere discovery and matcher model for host, VM, datastore, and cluster include filters.
- Existing datastore accessibility guard.
- Existing two-phase chart creation for property charts and perf charts.
- `govmomi` APIs already used by the collector.
- `vcsim` from the `govmomi` ecosystem for synthetic validation.

Risk and blast radius:

- High blast radius for existing users because contexts/dimensions/labels are consumed by dashboards, alerts, APIs, and Cloud surfaces.
- High operational risk if default collection expands to full vSphere performance levels or per-instance counters without caps.
- Medium implementation risk for v2 migration because chart lifecycle, disappearing resources, and scoped metrics must be carefully tested.
- High product risk around vnodes: per-VM vnode emission could create very large node cardinality and change user mental models.
- High documentation risk because collector code, metadata, config schema, stock config, alerts, and README/integration docs must stay consistent.
- Sensitive-data risk is material because vSphere metadata often embeds names and inventory structure.

Code-size estimate from current tree:

- Current vSphere collector surface checked on 2026-05-07:
  - `charts.go`: 1300 lines.
  - `collect.go`: 566 lines.
  - `collector.go`: 181 lines.
  - `collector_test.go`: 897 lines.
  - discovery/resource/match code and tests: several additional focused files.
  - config schema, metadata, stock config, and health alerts are also affected.
- V2 migration plus compatibility harness is medium-sized, not huge:
  - expect meaningful edits in `collector.go`, `collect.go`, chart-template code/YAML, tests, and fixtures;
  - the golden compatibility manifest may be mechanically large but should be generated/deterministic test data;
  - review risk is high because public metric identity must not change, not because the code volume is necessarily huge.
- Safe default enrichment is also medium-sized:
  - snapshot aggregate collection and alerts;
  - datastore aggregate additions;
  - low-cost object property/status metrics.
- Full parity with all opt-in child-instance groups is large:
  - VM disks, VM NICs, host NICs, LUNs/devices, storage adapters, storage paths, hardware sensors, vSAN, tags/custom attributes, and event/log follow-up each have different cardinality, labels, docs, tests, and operational risks.
- Practical implication:
  - one PR for V2 migration plus safe default enrichment is manageable because the compatibility harness landed first;
  - the user explicitly pulled the remaining non-event parity rows into this SOW/PR on 2026-05-08, so opt-in high-cardinality, label-enrichment, and topology/function work must be implemented as reviewable commits rather than separate SOWs; collector-generated ESXi/VM vnode work was later removed from this PR by user decision on 2026-05-20;
  - events remain out of this PR by user decision.

Sensitive data handling plan:

- No real vCenter credentials, URLs, IP addresses, VM names, hostnames, datastore names, tags, custom attributes, guest hostnames, guest IPs, or snapshot names/descriptions will be written to SOWs, specs, docs, skills, tests, or comments.
- Test fixtures must be synthetic or redacted.
- If real production output is needed later, it must be summarized or sanitized before entering durable artifacts.
- Snapshot details should default to aggregate numeric metrics only; names/descriptions must not become labels by default.
- Parity research from local mirrored repositories is cited by upstream repository and checked HEAD commit, not workstation absolute paths. These mirrors are shallow clones, so evidence is snapshot-only current-source evidence.

Implementation plan:

1. Build a compatibility manifest from the existing vSphere collector.
   - Scope: current contexts, V1 chart IDs for traceability, dims, labels, units, config keys, schema keys, stock config, and health alerts.
   - Likely files/modules: `src/go/plugin/go.d/collector/vsphere/*`, `src/health/health.d/vsphere.conf`, stock config, metadata, schema.
2. Produce the parity matrix.
   - Scope: LogicMonitor, Datadog, Telegraf, Zabbix, Elastic, New Relic, Grafana exporter, and any additional mirrored vSphere references worth including.
   - Output: each row maps external signal to Netdata context/dimension/label/default status/opt-in status/exclusion reason.
3. Decide default-vs-opt-in collection policy and v2 vnode/scope policy.
   - Scope: decisions listed under `## Implications And Decisions`.
   - Blocker: implementation must not start until the user decisions are recorded.
4. Convert the collector to framework v2.
   - Scope: registration, metric store, chart templates, lifecycle, disappeared-resource handling, compatibility tests.
   - Constraint: existing public metric identity must remain unchanged.
5. Add VM snapshot aggregate collection.
   - Scope: retrieve `snapshot`, traverse snapshot tree, compute count, maximum age, and maximum chain depth.
   - Constraint: avoid snapshot names/descriptions as default labels.
6. Enrich datastore signals.
   - Scope: preserve current datastore charts and add safe missing fields from parity matrix.
   - Constraint: keep accessibility semantics correct.
7. Add selected parity/enrichment groups.
   - Scope: VM status/tools/storage, VM disks/NICs, host storage/network/hardware sensors, HA/network state/topology-adjacent metrics, vSAN if selected.
   - Constraint: high-cardinality groups must be opt-in or bounded unless the user explicitly accepts default cardinality.
8. Update all collector artifacts.
   - Scope: metadata, config schema, stock config, health alerts, README/docs, generated-artifact expectations, specs/skills if behavior changes durable workflow.
9. Validate with synthetic and golden tests.
   - Scope: old-vs-new chart compatibility, config compatibility, dyncfg compatibility, snapshot tree tests, datastore inaccessible tests, parity matrix coverage, load/cardinality tests.

Validation plan:

- Run a compatibility test that compares current and v2 outputs for existing fixtures and fails on any changed existing context, dimension name, label key, unit, algorithm, or scale. Chart ID differences are expected and must be explicitly asserted as the accepted V2 migration break.
- Run config tests for old YAML examples and dynamic configuration JSON payloads.
- Run snapshot traversal unit tests covering no snapshots, one snapshot, siblings, nested chains, missing optional fields, and old create times.
- Run datastore tests covering accessible and inaccessible datastores, capacity/free-space/uncommitted, and status values.
- Run discovery filter tests to prove existing include semantics are unchanged.
- Run v2 chart-template tests to prove generated contexts/dimensions match the compatibility manifest.
- Run `go test` for the vSphere collector package and any shared v2 chart/metrix package affected by the migration.
- Run static searches for changed existing context strings and config keys.
- Use `govmomi` / `vcsim` or synthetic fixtures for real-use validation.
- If a real vCenter sample is ever used, sanitize all identifiers before storing evidence.

Artifact impact plan:

- AGENTS.md: likely no update unless this SOW exposes a new general repository workflow rule.
- Runtime project skills: update `.agents/skills/project-writing-collectors/SKILL.md` only if vSphere work exposes a reusable collector-design rule not already covered.
- Specs: added the v1 compatibility manifest and parity matrix as implementation prep contracts; update them when implementation changes public collector behavior or parity classification.
- End-user/operator docs: update vSphere collector README/integration docs, stock config comments, health alert docs, and configuration examples if implementation proceeds.
- End-user/operator skills: likely unaffected unless vSphere operator workflows are added to public skills.
- SOW lifecycle: this SOW is in `current/` after user approval to proceed; if implementation is too large, split follow-up SOWs before behavior-changing code changes.

Open-source reference evidence:

- `DataDog/integrations-core @ c84ac0c95e3afc8996276977886c651b37c7cdc4`
  - `vsphere/metadata.csv:11` begins a broad vSphere metric catalog.
  - `vsphere/datadog_checks/vsphere/data/conf.yaml.example:86` documents collection level/type and collection-cost tradeoffs.
  - `vsphere/datadog_checks/vsphere/data/conf.yaml.example:115` documents event collection.
  - `vsphere/datadog_checks/vsphere/data/conf.yaml.example:175` documents filters by resource/property/name/inventory path/tag/attribute/hostname/guest hostname.
  - `vsphere/datadog_checks/vsphere/data/conf.yaml.example:236` documents metric filters per resource type.
  - `vsphere/datadog_checks/vsphere/config.py:80` lists collection/event/property/tag/custom-attribute/vSAN options.
  - `vsphere/datadog_checks/vsphere/constants.py:79` lists property metrics for VM, host, cluster, datastore, and resource pools.
  - `vsphere/datadog_checks/vsphere/metrics.py:74` lists VM performance metrics beyond current Netdata coverage.
  - `vsphere/datadog_checks/vsphere/metrics.py:211` lists host performance metrics beyond current Netdata coverage.
  - `vsphere/datadog_checks/vsphere/metrics.py:413` lists vSAN metrics.
  - `vsphere/datadog_checks/vsphere/event.py:24` implements event collection and transformation.
- `influxdata/telegraf @ 7906603d75f1e0b0ac8132dbbd62f931d1f487de`
  - `plugins/inputs/vsphere/README.md:1` states coverage for vCenter, clusters, hosts, resource pools, VMs, datastores, and vSAN.
  - `plugins/inputs/vsphere/README.md:43` lists default VM metrics including demand/readiness/latency, memory, network, power, virtual disk, and uptime.
  - `plugins/inputs/vsphere/README.md:86` lists host metrics including storage adapters and power.
  - `plugins/inputs/vsphere/README.md:145` lists cluster/resource pool/datastore/datacenter/vSAN controls.
  - `plugins/inputs/vsphere/README.md:187` documents query limits, concurrency, discovery interval, and timeouts.
  - `plugins/inputs/vsphere/README.md:213` documents custom attributes and metric lookback.
  - `plugins/inputs/vsphere/vsphere.go:23` shows config fields for include/exclude selectors, instances, vSAN, custom attributes, IP addresses, query limits, and concurrency.
  - `plugins/inputs/vsphere/vsphere.go:150` shows safe defaults for metric groups and query limits.
- `grafana/vmware_exporter @ 3edc42190c6709567c0465304525f42ead2ac550`
  - `vsphere/test_metrics.txt:1` shows cluster/datacenter/datastore metrics including capacity/provisioned/used.
  - `vsphere/test_metrics.txt:80` shows host and VM metrics including CPU demand/latency/readiness, memory, network, heartbeat, OS uptime, and virtual disk metrics.
- `elastic/beats @ fd8ae04b57a40892640a4db87823bdf384c873d3`
  - `metricbeat/module/vsphere/virtualmachine/virtualmachine.go:79` defines VM data including host, networks, datastores, custom fields, snapshots, performance data, and triggered alarms.
  - `metricbeat/module/vsphere/virtualmachine/virtualmachine.go:169` retrieves VM properties including `summary`, `datastore`, `triggeredAlarmState`, and `snapshot`.
  - `metricbeat/module/vsphere/virtualmachine/virtualmachine.go:241` collects snapshots from the root snapshot list.
  - `metricbeat/module/vsphere/virtualmachine/data.go:35` maps VM info including snapshot count/info and triggered alarms.
- `zabbix/zabbix @ 980ed807b324dbf333260a449cc7430c0a978aaf`
  - `templates/app/vmware/template_app_vmware.yaml:13` notes hardware sensors and custom performance counters.
  - `templates/app/vmware/template_app_vmware.yaml:53` covers VMware alarms/events.
  - `templates/app/vmware/template_app_vmware.yaml:520` covers datastore discovery and datastore capacity/performance alerts.
  - `templates/app/vmware/template_app_vmware.yaml:1118` covers VM snapshot consolidation needed.
  - `templates/app/vmware/template_app_vmware.yaml:1498` covers VM snapshot count and latest date.
  - `templates/app/vmware/template_app_vmware.yaml:1602` covers VM storage committed/uncommitted/unshared.
  - `templates/app/vmware/template_app_vmware.yaml:1633` covers VMware tools status/version alerts.
  - `templates/app/vmware/template_app_vmware.yaml:1706` covers per-VM network interface and virtual disk discovery/metrics.
- `newrelic/nri-vsphere @ 9366fcd3d597ae0712c94882331042d74fe38e22`
  - `README.md:6` states collection of summary/performance metrics from vCenter/ESXi.
  - `README.md:26` documents performance metrics by performance level and warns that more metrics increase load.
  - `README.md:42` states the integration uses `govmomi` and `vcsim` for integration tests.
  - `README.md:78` documents a local vCenter simulator workflow using `vcsim`.

Open decisions and pre-code gates:

- Product decisions made so far are recorded in `## Implications And Decisions`.
- Framework V2 migration implementation started after the compatibility manifest, parity matrix, and accepted chart-ID break were recorded.

## Implications And Decisions

Implementation started after the decisions, compatibility manifest, and normalized parity matrix were recorded.

1. Parity scope

   A. Curated safe parity by default, heavy parity opt-in.
   - Pros: preserves existing-user safety, avoids accidental vCenter overload, still allows Netdata to become equal/superset through documented opt-in groups.
   - Cons: some competitor-parity metrics are not collected by default.
   - Implications: config/schema must add collection groups or levels; docs must explain default vs opt-in.
   - Risks: users may expect "superset" to mean all-on-by-default unless docs are explicit.

   B. Full competitor parity enabled by default.
   - Pros: strongest literal interpretation of "equal or superset".
   - Cons: high vCenter load, high Netdata series/cardinality growth, higher chance of collection gaps or timeouts.
   - Implications: existing users may suddenly collect many more series and may need stronger vCenter sizing.
   - Risks: violates the critical compatibility spirit if existing deployments become slow or noisy.

   Recommendation: A.

   User decision 2026-05-07: A. Safe aggregate object-level metrics are default-on; heavy/high-cardinality parity is opt-in.

2. Framework v2 host-scope / vnode strategy

   A. Preserve current job-level emission by default; do not create per-VM/per-datastore/per-host vnodes by default.
   - Pros: best compatibility for current charts, dashboards, alerts, and user expectations.
   - Cons: less topology-native than per-resource vnodes.
   - Implications: NIDL friendliness is achieved through contexts and labels, not by changing node identity.
   - Risks: a later topology/vnode feature may need another explicit design pass.

   B. Create v2 HostScopes/vnodes for ESXi hosts by default, but keep VMs/datastores as labels.
   - Pros: better node model for physical/virtual hosts.
   - Cons: may change where charts appear and may surprise current users.
   - Implications: requires stable GUID policy and vnode lifecycle tests.
   - Risks: chart identity and Cloud node cardinality changes.

   C. Create v2 HostScopes/vnodes for ESXi hosts, VMs, and datastores by default.
   - Pros: maximum topology-native decomposition.
   - Cons: very high node cardinality, largest compatibility risk.
   - Implications: major product/UI behavior change.
   - Risks: not compatible with the "existing users just collect more data" rule.

   Recommendation: A for this SOW.

   User decision 2026-05-07: default to A, with opt-in host-scope modes. Do not automatically create a vSphere/vCenter vnode because that would move existing charts for users who did not configure `vnode` and would break backwards compatibility. Preserve the existing optional `vnode` config for the vCenter/job scope. Add `esxi_vnodes: false` so ESXi hosts may become v2 host-scoped vnodes only when explicitly enabled. Add `vm_vnodes: false` only as an explicitly warned advanced option because users are expected to install Netdata Agents inside VMs, and VM vnodes would duplicate those nodes in dashboards. Datastores are not recommended as vnodes in this SOW; keep them as datastore-labeled resource charts under the vCenter job/default scope unless a future topology-specific design creates non-host resource entities. This ESXi/VM vnode portion was superseded by the 2026-05-20 hard-removal decision; only the pre-existing job-level `vnode` remains in scope.

   Datastore vnode rationale:
   - Datastores are shared storage resources, not hosts that can run an Agent.
   - A datastore can be visible to many ESXi hosts and may move across operational ownership boundaries; assigning it a vnode can create ambiguous ownership.
   - Existing datastore charts already have stable labels `datacenter`, `datastore`, and `type` in `src/go/plugin/go.d/collector/vsphere/charts.go:717`.
   - NIDL compatibility can be achieved with datastore-specific contexts and labels without changing node identity.
   - If topology later needs datastore vertices, that should be modeled as topology/resource entities, not automatically as Netdata host/vnode identities.

3. Snapshot metric shape

   A. Aggregate per VM only: `count`, `max_age`, and `max_chain_depth`.
   - Pros: exactly matches user-requested signals, low cardinality, no sensitive snapshot names/descriptions.
   - Cons: no per-snapshot drilldown in metrics.
   - Implications: detailed snapshot names/descriptions stay out of default telemetry.
   - Risks: users cannot identify the exact snapshot from Netdata metrics alone.

   B. Per-snapshot dimensions/labels.
   - Pros: richer troubleshooting directly in metrics.
   - Cons: snapshot names/descriptions are sensitive and high-cardinality.
   - Implications: must add strong redaction/selector policy.
   - Risks: data exposure and unstable series identity.

   Recommendation: A.

   User decision 2026-05-07: A. Snapshot data is aggregate per VM only. Do not emit per-snapshot dimensions or labels by default.

4. No-snapshot value semantics

   A. Emit `count=0`, `max_chain_depth=0`, and `max_age=0` when a VM has no snapshots.
   - Pros: easy alerting and dashboarding.
   - Cons: `age=0` is a sentinel, not a real snapshot age.
   - Implications: docs and health alerts must state that age/depth zero means no snapshots.
   - Risks: consumers may misread zero age as a fresh snapshot.

   B. Emit `count=0` and omit age/depth dimensions when no snapshots exist.
   - Pros: avoids sentinel values.
   - Cons: dynamic dimension lifecycle is more complex and less convenient for alerting.
   - Implications: chart lifecycle tests become more important.
   - Risks: disappearing dimensions may confuse users.

   Recommendation: A, with clear docs.

   User decision 2026-05-07: A. Emit `snapshot_count=0`, `snapshot_max_chain_depth=0`, and `snapshot_max_age=0` when a VM has no snapshots. Documentation and chart help must state that zero age/depth means no snapshot exists.

5. Events and alarms

   A. Keep this SOW metrics-first; support status/alarm property metrics and health alerts, but do not add vCenter event/log ingestion here.
   - Pros: contained scope, matches Netdata collector model, avoids mixing metrics with event/log ingestion.
   - Cons: not a full Datadog event-parity implementation.
   - Implications: parity matrix must mark events as separate-surface or future tracked work.
   - Risks: "equal or superset" remains incomplete if events are considered in scope.

   B. Include vCenter event collection in this SOW.
   - Pros: closer Datadog/LogicMonitor parity.
   - Cons: larger design surface; events need a logs/events pipeline contract, not just metric charts.
   - Implications: likely needs additional specs/docs and a separate validation strategy.
   - Risks: scope creep and delayed v2/metrics compatibility work.

   Recommendation: A unless the user explicitly requires event parity in this collector change.

   User decision 2026-05-07: A. Keep this SOW metrics-first. vCenter events still need to be solved later, likely by pushing them through Netdata's OTEL/logs ingestion path. Superseded by the 2026-05-08 event decision below: events are excluded from this PR, and future event work requires a new user-approved SOW.

6. High-cardinality groups

   A. Add bounded opt-in collection groups for per-instance/high-cardinality data.
   - Pros: gives advanced users parity without forcing cardinality on everyone.
   - Cons: more config and docs.
   - Implications: config schema needs selectors/limits for VM disks, VM NICs, host NICs, storage paths/adapters, hardware sensors, tags/custom attributes, and vSAN.
   - Risks: misconfiguration can still create large series counts.

   B. Enable all discovered per-instance data by default.
   - Pros: maximum data out of the box.
   - Cons: high series cardinality and vCenter load.
   - Implications: existing users may experience data volume and performance changes.
   - Risks: not safe for large vCenters.

   Recommendation: A.

   User decision 2026-05-07: A. Per-child-instance, sensitive-label, expensive, vSAN, and event-like surfaces require explicit opt-in selectors or bounds.

7. Implementation decomposition

   A. Keep one umbrella SOW, but implement in internal phases with hard validation gates.
   - Pros: keeps the compatibility and parity goal unified.
   - Cons: the SOW will be large and long-lived.
   - Implications: progress must be tracked carefully in the execution log.
   - Risks: harder to review as one PR if it becomes too large.

   B. Split after feasibility into multiple SOWs: v2 compatibility migration, snapshot/datastore enrichment, parity matrix, high-cardinality opt-ins, docs/alerts.
   - Pros: smaller reviewable chunks, lower regression risk per PR.
   - Cons: more lifecycle overhead and more cross-SOW dependency tracking.
   - Implications: compatibility manifest must be shared across SOWs.
   - Risks: fragmented implementation may leave parity incomplete longer.

   Recommendation: B if implementation is intended to start immediately after this study; A only if one large PR is acceptable.

   User decision 2026-05-07: use one SOW/PR for the core work, split into focused reviewable commits. Commit boundaries should follow the implementation chunks: compatibility harness/manifest, V2 migration, snapshot aggregate metrics and alerts, datastore aggregate enrichment, safe default property/status metrics, and docs/schema/metadata/health consistency. This was later expanded on 2026-05-08 to pull opt-in high-cardinality parity groups into the same SOW/PR. vCenter event ingestion remains excluded from this PR by user decision.

8. Framework V2 chart-ID compatibility

   A. Extend framework V2 chartengine to support full chart-ID templates for instance charts, then migrate vSphere using `<resource_id>_<chart>` IDs.
   - Pros: preserves the critical compatibility requirement and improves framework V2 for future migrations that have legacy dynamic IDs.
   - Cons: requires framework code changes and tests before the vSphere migration.
   - Implications: chartengine template parsing/rendering must support label placeholders or an equivalent full-ID mode, without breaking existing suffix-based V2 collectors.
   - Risks: framework maintainer review becomes part of this PR; the design must be small and backward compatible.

   B. Keep vSphere on framework V1 for now and implement enrichment there.
   - Pros: avoids chartengine framework work.
   - Cons: fails the explicit requirement to move vSphere to framework V2.
   - Implications: enrichment could ship sooner, but migration remains unsolved.
   - Risks: creates more V1 code that must be migrated later.

   C. Migrate to V2 using normal suffix IDs and accept renamed charts.
   - Pros: simplest V2 implementation.
   - Cons: breaks chart IDs and by-instance time-series continuity for old IDs.
   - Implications: context/dimension/label-based charts and health templates can remain compatible; users grouped by anything except `by instance` should not see a semantic change.
   - Risks: any external consumer that keys directly on old chart IDs will need to adapt.

   D. Generate static V2 chart templates from the initial discovery result.
   - Pros: can preserve initial chart IDs without changing chartengine.
   - Cons: V2 runtime loads `ChartTemplateYAML()` once after `Check()`, so resources discovered later would not get templates.
   - Implications: dynamic vCenter inventory changes would be broken or incomplete.
   - Risks: not compatible with current discovery/lifecycle behavior.

   Original recommendation before maintainer-risk decision: A. Superseded by the user decision below.

   User decision 2026-05-08: C for chart IDs only. Do not extend framework V2 chartengine for vSphere. Migrate with normal V2 instance chart IDs even though old chart IDs and by-instance time-series continuity are lost. Preserve compatibility for contexts, dimensions, labels, configuration, dynamic configuration, and metric meaning.

9. VM and host power-state discovery controls

   A. Add `vm_power_states` and `host_power_states` as optional config arrays. Defaults preserve current behavior by including only `poweredOn`. The stock config and dyncfg schema show every valid state so users can opt in explicitly.
   - Pros: backwards compatible, self-explanatory, lets users collect property/status/snapshot data for non-running objects when they want it.
   - Cons: more config surface.
   - Implications: discovery filtering moves from hardcoded `poweredOn` to configured allow-lists; performance scraping must not query non-powered-on VMs/hosts unless proven valid.
   - Risks: if non-powered-on VMs lack host placement data, hierarchy/filtering may need safe fallback behavior.

   B. Keep hardcoded `poweredOn` filters and only document that non-powered-on objects are excluded.
   - Pros: smallest change.
   - Cons: no user control, does not satisfy the request for self-explanatory options.
   - Implications: status/snapshot/property data for powered-off and suspended VMs remains unavailable.
   - Risks: parity gap remains.

	   Recommendation: A.

	   User decision 2026-05-08: A. Add explicit power-state configuration for VMs and hosts. Defaults must remain `poweredOn` only for backwards compatibility. The stock config must list all valid states so the option is self-explanatory. Non-powered-on VMs/hosts may collect only safe property/status data; performance scraping for non-powered-on VMs/hosts must be skipped unless the implementation proves a valid VMware performance path.

10. Remaining parity work packaging

   User decision 2026-05-08: Keep all remaining vSphere parity work in this SOW and one PR, split by reviewable commits. The pending SOW files created for follow-up tracking are superseded by this decision and should be removed or consolidated before implementation proceeds.

11. Pending follow-up SOW files

   User decision 2026-05-08: Delete the four pending vSphere follow-up SOW files created during the earlier split attempt. Track the remaining vSphere parity work inside this SOW instead.

12. Opt-in configuration style

   User decision 2026-05-08: Use granular explicit opt-in groups, not one opaque collection level. Each high-cardinality group should have a self-explanatory config key.

13. Remaining high-cardinality feasibility

   User decision 2026-05-08: Do feasibility now before deciding whether every remaining opt-in metric group can be implemented in this PR. Feasibility must cover official documentation, SDK support, local mirrored/open-source implementations, fixtures or simulator paths, testability, and operational risk.

14. Cap and selector policy

   User decision 2026-05-08: Every opt-in high-cardinality group needs both a selector or allowlist and a maximum cap.

15. Sensitive enrichment labels

   User concern 2026-05-08: Netdata is a monitoring system; users need labels to filter and group by vSphere metadata. The implementation adds opt-in inventory path, VM guest, vSphere tag, and custom-attribute labels with selectors/caps. More sensitive device identity labels such as MACs, IQNs, WWNs, and datastore paths remain excluded from this PR pending a separate allowlist/cap product decision.

16. vCenter/ESXi events

   User decision 2026-05-08: Do not implement events in this PR. The user needs to sync with developers first. Events remain outside this SOW's implementation scope.

17. Topology and troubleshooting surface

   User decision 2026-05-08: Implement topology/troubleshooting as topology output and/or Functions, not as metric labels.

   Follow-up user decision 2026-05-08: Publish vSphere topology using the existing topology Function naming convention: public `topology:vsphere` with a required job selector. Do not keep the job-method shape `vsphere:<job>:topology`. Keep readiness as a module Function with a required job selector too because the framework does not allow mixing module methods and job methods in one collector.

18. Collector-generated vnodes

   User decision 2026-05-08: Implement optional ESXi and optional VM vnodes, both default off. VM vnodes must carry a clear duplication warning because users are expected to run Netdata Agents inside VMs too. Datastore vnodes remain excluded unless a later explicit product decision changes that.

   Superseding user decision 2026-05-20: hard-remove optional ESXi and VM
   vnode support from this PR. Keep only the existing job-level `vnode`
   configuration. Reintroduce collector-generated ESXi/VM vnodes only in a
   separate PR with stable identity and lifecycle design.

19. Evidence threshold for difficult surfaces

   User decision 2026-05-08: If exact official documentation, open-source implementations, or fixtures are available and can be examined, implement the surface. If not, the surface must be marked blocked with evidence rather than implemented by guesswork.

## Remaining Parity Feasibility - 2026-05-08

TL;DR:

- Technically feasible: P13-P22, P26, P28-P34.
- Feasible but not metrics: P30-P32 must use topology output and/or Functions.
- Not in this PR: P27 events, by explicit user decision.
- Highest-risk item: P26 vSAN, because it uses the separate vSAN API endpoint/performance service and must be isolated behind an opt-in group.

Current Netdata label surface:

- Current vSphere V2 bridge appends `id=<managed-object-id>` and preserves old chart labels in `src/go/plugin/go.d/collector/vsphere/metrics.go:68`.
- VM charts currently label `datacenter`, `cluster`, `host`, and `vm` in `src/go/plugin/go.d/collector/vsphere/charts.go:954`.
- Host charts currently label `datacenter`, `cluster`, and `host` in `src/go/plugin/go.d/collector/vsphere/charts.go:980`.
- Datastore charts currently label `datacenter`, `datastore`, and `type` in `src/go/plugin/go.d/collector/vsphere/charts.go:1018`.
- Cluster charts currently label `datacenter` and `cluster` in `src/go/plugin/go.d/collector/vsphere/charts.go:1621`.
- Resource-pool charts currently label `datacenter`, `cluster`, and `resource_pool` in `src/go/plugin/go.d/collector/vsphere/charts.go:1634`.
- Gap: the plugin does not yet emit vSphere tags, custom attributes, inventory paths, folder lineage, guest hostname/IP, MAC, IQN, WWN, or datastore/device paths as labels.

Feasibility table:

| Rows | Surface | Feasibility | Evidence | Implementation rule |
|---|---|---|---|---|
| P13 | Datastore clusters / storage pods | Feasible | Broadcom `StoragePod` docs; `elastic/beats @ 7bbe8ee...` datastorecluster module | Opt-in `collect_datastore_clusters`, selector, max cap. |
| P14-P16 | VM disks and vNICs | Feasible | Broadcom `VirtualDisk` and `VirtualEthernetCard`; Datadog/Telegraf/OTel/Zabbix/New Relic implementations | Opt-in groups, per-child selector, max cap, no raw datastore path/MAC by default. |
| P17-P21 | Host pNICs, disks/LUNs, HBAs, storage paths, CPU cores | Feasible | Broadcom counter pages plus `HostScsiDisk`, `HostHostBusAdapter`, `HostMultipathInfo`; Datadog/Telegraf/OTel/New Relic implementations | Opt-in groups, selector, max cap. Storage paths are the highest-cardinality host detail. |
| P22 | Host/VM power and energy counters | Feasible | Broadcom power counters; Datadog/Telegraf/New Relic collect power counters | Opt-in until version/API availability is proven in tests. |
| P26 | vSAN cluster/host capacity/performance/health | Feasible, high risk | Broadcom vSAN Management API; Datadog, Telegraf, OTel vSAN metrics and OTel recorded fixtures | Opt-in, cluster selector, vSAN API capability check, performance-service check, isolated commit. vSAN events excluded. |
| P28 | Tags and custom attributes as labels | Feasible | Broadcom Tag Association API and `CustomFieldsManager`; Datadog, Telegraf, Elastic, New Relic | Opt-in only. Category/attribute allowlist, key prefix/sanitization, per-object label cap. |
| P29 | Inventory paths, guest hostname/IP, MAC/IQN/WWN identity | Feasible in subsets | Datadog filters, Telegraf IP option, OTel inventory-path attributes, Broadcom guest/device/storage objects | Opt-in label groups. Sensitive values default off; multi-value values capped. |
| P30 | Topology edges | Implemented, non-metric | LogicMonitor topology sources, Elastic network module, New Relic object fixtures, OTel resource model, Netdata Function topology schema, SNMP topology Function pattern | Cached public `topology:vsphere` Function alias emits inventory actors/links, not metric labels, and requires selecting a vSphere job. |
| P31 | Network/DVPG status | Implemented opt-in, non-metric | Broadcom `NetworkSummary.accessible`, Broadcom `DistributedVirtualPortgroup`, New Relic network collector, Elastic network module | `collect_network_topology` defaults off and discovers Network/DVPG objects only for cached topology actors/status/links. |
| P32 | Troubleshooter/readiness checks | Implemented, non-metric | LogicMonitor troubleshooter, Datadog connection service checks, Netdata Function table schema | Cached `vsphere:readiness` Function reports local readiness/config/discovery/optional-surface/vSAN state without exposing URL/credentials or issuing extra vCenter API calls, and requires selecting a vSphere job. |
| P33-P34 | Optional VM and ESXi vnodes | Feasible but removed from this PR | V2 host-scope spec, local `azure_monitor` scoped collector pattern, and 2026-05-20 review decision | Out of scope for this PR. Requires a future focused identity/lifecycle design. |
| P27 | vCenter/ESXi events | Feasible separately, excluded here | Datadog/New Relic event implementations and Broadcom EventManager | Do not implement in this PR by user decision. |

Testability:

- Existing `govmomi/simulator` tests cover the main SOAP discovery/scrape lifecycle and can be extended with synthetic resources.
- New Relic ships `govc object.save` vCenter fixtures for snapshots and object relationships; OTel ships recorded vCenter 7.0.2 mock XML, including vSAN responses.
- Datadog/Telegraf/OTel metric lists provide exact counter names and grouping evidence.
- Residual risk: vSAN and storage-path behavior cannot be fully proven against every real environment without a live vCenter/vSAN or broader recorded fixtures. The PR must record this as residual validation risk, keep the groups opt-in, and fail gracefully when capability checks fail.

Implementation decisions derived from feasibility:

- Use explicit boolean `collect_*` groups for each non-default surface, plus include selectors and max caps for every child-instance group.
- Preserve default-safe behavior: new child-instance metrics, sensitive labels, vSAN, and network topology discovery default off.
- Implement labels because users need filtering/grouping, but only through opt-in allowlists for user-defined or sensitive metadata.
- Events are excluded from this PR; no metric or Function substitute should be added for events.

## Plan

1. Finish and record feasibility gate.
   - Scope: current code labels, official docs, mirrored implementations, fixtures, risks, and no-event decision.
   - Risk: low; no source behavior changes.
2. Normalize remaining parity matrix and SOW lifecycle.
   - Scope: one SOW/one PR, no deleted follow-up SOW references, rows P13-P34 classified against current decisions.
   - Risk: low.
3. Implement opt-in labels and high-cardinality metric groups.
   - Scope: P13-P22, P26, P28-P29 with granular config, selectors, caps, docs, tests.
   - Risk: high; vCenter load, cardinality, sensitive metadata, and vSAN capability handling.
4. Remove collector-generated ESXi/VM vnodes from this PR.
   - Scope: P33-P34 with config, schema, docs, tests, and HostScope plumbing removed.
   - Risk: low after removal; reintroduction requires a separate identity/lifecycle design.
5. Implement topology/function surfaces where existing Netdata patterns allow it.
   - Scope: P30-P32. Implemented as cached read-only Functions; topology is not faked as metric labels.
   - Risk: medium; product surface must not be faked as metrics.
6. Update artifacts and validate.
   - Scope: code, tests, metadata, schema, stock config, health, README/generated integration docs, specs, SOW.
   - Risk: medium; artifacts must stay synchronized.

## Execution Log

### 2026-05-07

- Created SOW from project template.
- Recorded user requirements.
- Inspected current vSphere collector architecture, config, discovery, metrics, datastore handling, and charts.
- Inspected local v2 collector patterns and v2 host-scope spec.
- Researched official Broadcom vSphere API documentation for snapshots, datastores, and performance manager.
- Researched current Datadog and LogicMonitor vSphere documentation.
- Researched mirrored vSphere monitoring implementations from Datadog, Telegraf, Grafana exporter, Elastic Beats, Zabbix, and New Relic.
- Clarified high-cardinality definition and competitor default-policy evidence after user question.
- Added a vSphere cardinality sizing model with formulas and a moderate-environment example.
- Clarified the vCenter-backed collection topology: one Netdata job can fan out across all included resources behind one vCenter endpoint.
- Added proposed default-vs-opt-in policy: safe aggregate object-level metrics default on; high-cardinality, sensitive, or expensive surfaces opt-in.
- Recorded user decision for safe default metrics and opt-in high-cardinality groups.
- Recorded corrected user vnode decision: no automatic vCenter vnode; preserve optional job-level `vnode`; no datastore vnodes in this SOW. Optional ESXi/VM vnodes were later removed by user decision on 2026-05-20.
- Added evidence for snapshot no-data behavior in Zabbix, Elastic Beats, New Relic, Datadog, Telegraf, and Grafana exporter.
- Recorded user decision for aggregate per-VM snapshot metrics and zero no-snapshot semantics.
- Recorded user decision to exclude vCenter events from this metrics SOW while tracking event ingestion as a later OTEL/logs-path problem.
- Clarified that deferred vCenter events are separate API calls/surface, not the same VMware metric query, using Datadog as evidence.
- Clarified that framework v2 is not inherently user-facing incompatible with v1; compatibility depends on preserving config/schema/chart/metric/label/scope contracts during migration.
- Added concrete compatibility harness design: V1 golden manifest, config/schema tests, V2 chart-template tests, V2 metric-store tests, and lifecycle tests.
- Added code-size estimate: V2 plus safe default enrichment is medium-sized; full opt-in parity is large.
- Recorded implementation decomposition decision: one core SOW/PR split into focused reviewable commits; this was later expanded to include opt-in high-cardinality parity, while events remain excluded by user decision.
- Added the initial parity target checklist defining the required coverage groups and classifying them as existing-default, new-default, opt-in, covered-elsewhere, follow-up, or excluded.
- Searched go.d collector, config, health, and SNMP profile surfaces for VMware/vSphere/vCenter/ESXi/ESX overlap.
- Recorded overlap findings: `vcsa` covers VCSA appliance health; SNMP `vmware-esx` covers ESXi HBA and hardware/environment status per host; generic Prometheus has no VMware-specific in-tree integration.
- Added implementation readiness and success assurance gates: compatibility manifest first, normalized parity matrix second, then behavior-changing v2 migration only after both gates are satisfied.
- Spawned a read-only agent to review framework V2 go.d collector style and maintainer-preferred patterns.
- Added `.agents/skills/project-writing-go-modules-framework-v2/SKILL.md` to capture framework V2 collector style, chart-template, metric-store, host-scope, config, logging, and test conventions.
- Added the framework V2 skill as a repo-local runtime skill. `AGENTS.md` registration was intentionally removed from the final PR diff to avoid broad repository-instruction churn.
- Reworded a pre-existing GitHub SSH example in `.agents/skills/mirror-netdata-repos/SKILL.md` that triggered the SOW audit email-pattern guardrail.
- Compressed `.agents/skills/project-writing-go-modules-framework-v2/SKILL.md` after user feedback; kept the operational rules and read-first source list, removed citation-heavy evidence sections.
- No collector source implementation started.

### 2026-05-08

- Moved this SOW to `.agents/sow/current/` and marked it `in-progress` after the user approved proceeding.
- Drafted `.agents/sow/specs/vsphere-v1-compatibility-manifest.md` as the current v1 public-contract baseline for chart IDs, contexts, dimensions, labels, config keys, schema keys, stock config, health alerts, metric source lists, lifecycle behavior, and existing artifact drift.
- Added `src/go/plugin/go.d/collector/vsphere/compat_manifest_test.go` and `src/go/plugin/go.d/collector/vsphere/testdata/v1_compat_manifest.json` to make the current V1 chart/metric contract executable in tests.
- Normalized the executable golden to pin label keys and sources, not simulator-specific label values, because `vcsim` can assign VM host labels differently between runs.
- Drafted `.agents/sow/specs/vsphere-parity-matrix.md` as the normalized parity matrix with all rows classified as `existing-default`, `new-default`, `opt-in`, `covered-elsewhere`, `follow-up`, or `excluded`.
- Recorded that mirrored repository evidence is from shallow clone HEAD snapshots only; it supports current-source parity comparisons, not history/blame/timeline conclusions.
- Added the framework V2 chart-ID compatibility blocker with file/line evidence: current V2 literal templates and suffix-only instance IDs cannot preserve vSphere V1 resource-ID-prefix chart IDs.
- Recorded user decision to accept chart-ID break and proceed with normal V2 instance chart IDs, while preserving contexts, dimensions, labels, configuration, dynamic configuration, and metric meaning.
- Updated the SOW and compatibility-manifest spec so chart IDs are traceability evidence, not a V2 preservation requirement.
- Converted vSphere registration from `Create` / `CollectorV1` to `CreateV2` / `CollectorV2`.
- Added `metrix.NewCollectorStore()`, `MetricStore()`, embedded `charts.yaml`, and `ChartTemplateYAML()`.
- Kept the existing V1 `collect()` map path as an intentional compatibility bridge for the first migration commit, while public `Collect(context.Context)` now writes to the V2 metric store and returns `error`.
- Added a V2 metric bridge that maps every existing V1 chart context and dimension name into a pre-registered V2 gauge.
- Added generated `charts.yaml`, derived from existing V1 chart templates by `TestCollector_ChartTemplateYAML`, to avoid hand-copy drift.
- Added `TestCollector_V2CompatibilitySurface` to prove:
  - V2 chart IDs are the expected accepted break (`<context_suffix>_<resource-id>` instead of `<resource-id>_<chart>`);
  - old contexts, titles, families, units, chart types, priorities, dimensions, algorithms, multipliers, divisors, and old label key/value pairs are preserved;
  - the only new V2 chart label is `id=<managed-object-id>`, required for stable instance chart materialization.
- Added V2 chart coverage assertion in `TestCollector_Collect` so collected metric-store samples materialize the expected chart-template contexts and dimensions.
- Added default per-VM snapshot aggregate metrics:
  - `vsphere.vm_snapshot_count` / `count` in snapshots;
  - `vsphere.vm_snapshot_max_age` / `age` in seconds;
  - `vsphere.vm_snapshot_max_chain_depth` / `depth` in snapshots.
- Added VM discovery of the `snapshot` property and snapshot-tree traversal that stores only aggregate count, maximum chain depth, and oldest create time. Snapshot names, descriptions, and snapshot IDs are not emitted as labels or metrics.
- Added zero no-snapshot semantics: count, age, and chain depth are all `0` when a VM has no snapshots.
- Added health templates:
  - `vsphere_vm_snapshot_chain_depth` warns when `depth > 3`;
  - `vsphere_vm_snapshot_age` is critical when `age > 86400` seconds.
- Updated `metadata.yaml` and `README.md` for the new snapshot metrics, alerts, and the V2 `id` instance label.
- Updated `.agents/sow/specs/vsphere-parity-matrix.md` rows P06/P07 from proposed snapshot contexts to implemented contexts and alerts.
- Added default datastore aggregate enrichment:
  - `vsphere.datastore_space_usage` now includes `uncommitted`;
  - `vsphere.datastore_accessibility_status` reports accessible/inaccessible;
  - `vsphere.datastore_maintenance_status` reports normal, entering maintenance, in maintenance, or unknown;
  - `vsphere.datastore_multiple_host_access` reports enabled, disabled, or unknown.
- Preserved the VMware datastore capacity guard: capacity, free, used, used percent, and uncommitted are emitted as zero when `DatastoreSummary.accessible` is false.
- Kept initially inaccessible datastores in discovery as status-only resources; datastore performance scraping skips inaccessible datastores so historical perf queries do not target them.
- Updated `metadata.yaml`, generated `README.md`, generated `charts.yaml`, and the executable compatibility manifest for datastore enrichment.
- Updated `.agents/sow/specs/vsphere-parity-matrix.md` row P08 from proposed datastore contexts to implemented contexts and semantics.
- Expanded `src/go/plugin/go.d/config/go.d/vsphere.conf` comments so the stock config explains one-job-per-vCenter behavior, selector formats/defaults, resource-pool selector behavior, secret resolver usage, optional `vnode`, discovery interval load considerations, and common HTTP/TLS settings.
- Recorded user decision to add explicit VM/host power-state configuration while preserving default `poweredOn` discovery.
- Added optional config keys:
  - `host_power_states`, default `poweredOn`, valid values `poweredOn`, `poweredOff`, `standBy`, `unknown`;
  - `vm_power_states`, default `poweredOn`, valid values `poweredOn`, `poweredOff`, `suspended`.
- Added default object-level power-state metrics:
  - `vsphere.host_power_state` with dimensions `powered_on`, `powered_off`, `standby`, `unknown`;
  - `vsphere.vm_power_state` with dimensions `powered_on`, `powered_off`, `suspended`.
- Changed discovery filtering from hardcoded `poweredOn` to the configured power-state allow-lists. Defaults preserve previous behavior.
- Added a safe hierarchy fallback for opted-in non-running VMs whose `runtime.host` is absent: use the VM inventory folder parent to recover the datacenter label where possible; host and cluster labels remain empty when vCenter does not provide placement.
- Skipped real-time performance query specs for non-powered-on hosts and VMs. Such objects collect property/status data only.
- Updated `config_schema.json`, stock `go.d/vsphere.conf`, `metadata.yaml`, generated README/integration docs, generated `charts.yaml`, the compatibility manifest fixture, and the parity/compatibility specs for the power-state controls and metrics.
- Added default object-level VM property/status metrics:
  - `vsphere.vm_connection_state` for connected/disconnected/orphaned/inaccessible/invalid;
  - `vsphere.vm_tools_running_status` for running/not-running/executing-scripts/unknown;
  - `vsphere.vm_tools_version_status` for bounded VMware Tools version states plus unknown;
  - `vsphere.vm_consolidation_needed` for disk consolidation-needed state;
  - `vsphere.vm_config_cpu`, `vsphere.vm_config_memory`, `vsphere.vm_config_devices`, and `vsphere.vm_storage_usage` for bounded numeric summary data.
- Added default object-level host property/status metrics:
  - `vsphere.host_connection_state` for connected/not-responding/disconnected;
  - `vsphere.host_maintenance_status` for normal/in-maintenance.
- Kept guest hostname, guest IP, guest OS name, tags, custom attributes, and inventory paths out of default labels/metrics because they are free-form or sensitive identity surfaces.
- Updated `metadata.yaml`, generated README/integration docs, generated `charts.yaml`, the compatibility manifest fixture, and the parity/compatibility specs for the VM/host property-status slice.
- Fixed `integrations/gen_docs_integrations.py` path normalization so scoped docs generation works with current `blob/master` metadata links as well as older `edit/master` links.
- Added default object-level cluster DRS/HA property-status metrics:
  - `vsphere.cluster_drs_mode` for manual/partially-automated/fully-automated/unknown;
  - `vsphere.cluster_drs_vmotion_rate` for the numeric DRS vMotion recommendation threshold;
  - `vsphere.cluster_ha_host_monitoring` for enabled/disabled/unknown;
  - `vsphere.cluster_ha_vm_monitoring` for disabled/VM-monitoring-only/VM-and-application-monitoring/unknown;
  - `vsphere.cluster_ha_vm_component_protection` for enabled/disabled/unknown.
- Kept cluster DRS/HA enrichment bounded to state-set dimensions and one numeric threshold. No free-form cluster config strings are emitted as labels.
- Updated `metadata.yaml`, generated README/integration docs, generated `charts.yaml`, the compatibility manifest fixture, and the parity/compatibility specs for the cluster property-status slice.
- Added default job-level inventory count metrics:
  - `vsphere.inventory_objects` with dimensions `datacenters`, `folders`, `clusters`, `hosts`, `vms`, `datastores`, and `resource_pools`.
- Inventory counts reflect the resources discovered after include filters are applied. They do not create a mandatory vCenter vnode; V2 uses only the static `id=inventory` instance label.
- Updated `metadata.yaml`, generated README/integration docs, generated `charts.yaml`, the compatibility manifest fixture, and the parity/compatibility specs for the inventory-count slice.
- Recorded user decisions to keep all remaining non-event parity work in this SOW/PR, use granular opt-in groups, require selectors and caps for high-cardinality groups, implement labels through opt-in allowlists, exclude events from this PR, implement topology/troubleshooting through topology output and/or Functions, and hard-remove optional ESXi/VM vnodes before merge.
- Deleted the four pending vSphere follow-up SOW files that were superseded by the one-SOW/one-PR decision.
- Completed feasibility for remaining parity rows P13-P34. All non-event rows are technically feasible with opt-in controls and tests/fixtures; P27 events are feasible separately but out of this PR by user decision; P26 vSAN is feasible but highest risk.
- Updated `.agents/sow/specs/vsphere-parity-matrix.md` to remove stale follow-up-SOW language, update mirrored repository commit evidence, classify topology/function rows as non-metric surfaces, classify events as out-of-scope for this PR, and classify VM/ESXi vnodes as out of scope for this PR.
- Removed optional V2 host scopes for ESXi hosts and VMs:
  - deleted `esxi_vnodes` and `vm_vnodes` config/schema/metadata/stock-config/test-fixture surfaces;
  - removed HostScope routing from VM-owned and host-owned metric writers;
  - deleted vnode implementation and tests.
- Updated `config_schema.json`, stock `go.d/vsphere.conf`, `metadata.yaml`, generated integration docs, and config serialization fixtures after removing the vnode options.
- Added opt-in label enrichment:
  - `collect_inventory_path_label` defaults to `false` and adds `inventory_path` only when the collector can derive a bounded inventory path for VM, host, datastore, cluster, or resource-pool resources.
  - `vm_guest_labels` defaults to an empty allowlist and only accepts `guest_hostname`, `guest_ip`, and `guest_os`.
  - Default labels remain unchanged except for the already accepted V2 `id` instance label.
- Added optional VM virtual disk capacity collection:
  - `collect_vm_disks` defaults to `false`;
  - `vm_disk_include` defaults to `*` and matches disk display label, numeric disk key, or `key:<disk_key>`;
  - `max_vm_disks` defaults to `1024`;
  - context `vsphere.vm_disk_capacity` emits one `capacity` dimension in bytes with labels `id`, `datacenter`, `cluster`, `host`, `vm`, `disk`, and `disk_key`;
- Added optional VM virtual disk performance collection:
  - `collect_vm_disk_performance` defaults to `false`;
  - the group reuses `vm_disk_include` and `max_vm_disks`;
  - vSphere `virtualDisk.*` counters are requested with wildcard instance `*` only when the option is enabled;
  - returned performance instances such as `scsi0:0` are selected by raw instance or `instance:<disk_instance>`;
  - contexts `vsphere.vm_disk_device_io`, `vsphere.vm_disk_device_iops`, `vsphere.vm_disk_device_latency`, and `vsphere.vm_disk_device_outstanding_io` emit read/write throughput, IOPS, latency, and outstanding I/O;
  - labels are `id`, `datacenter`, `cluster`, `host`, `vm`, `disk`, and `disk_instance`, plus opt-in enrichment labels;
- Added optional VM network-interface performance collection:
  - `collect_vm_nic_performance` defaults to `false`;
  - `vm_nic_include` defaults to `*` and matches the raw vSphere performance instance or `interface:<interface_instance>`;
  - `max_vm_nics` defaults to `1024`;
  - selected VM `net.*` counters are requested with wildcard instance `*` only when the option is enabled;
  - existing aggregate VM network metrics remain default-on and are not suppressed by the opt-in per-interface surface;
  - contexts `vsphere.vm_net_interface_traffic`, `vsphere.vm_net_interface_packets`, `vsphere.vm_net_interface_drops`, `vsphere.vm_net_interface_broadcast_packets`, and `vsphere.vm_net_interface_multicast_packets` emit per-interface throughput, packets, drops, broadcast packets, and multicast packets;
  - labels are `id`, `datacenter`, `cluster`, `host`, `vm`, `interface`, and `interface_instance`, plus opt-in enrichment labels;
  - VM packet error counters are not emitted because the current Broadcom network counter table documents `errorsRx` and `errorsTx` for `HostSystem`, not VM.
- Added optional host physical network-interface performance collection:
  - `collect_host_nic_performance` defaults to `false`;
  - `host_nic_include` defaults to `*` and matches the raw vSphere performance instance or `interface:<interface_instance>`;
  - `max_host_nics` defaults to `1024`;
  - selected host `net.*` counters are requested with wildcard instance `*` only when the option is enabled;
  - existing aggregate host network metrics remain default-on and are not suppressed by the opt-in per-interface surface;
  - contexts `vsphere.host_net_interface_traffic`, `vsphere.host_net_interface_packets`, `vsphere.host_net_interface_drops`, `vsphere.host_net_interface_errors`, `vsphere.host_net_interface_broadcast_packets`, `vsphere.host_net_interface_multicast_packets`, `vsphere.host_net_interface_unknown_protocol_frames`, and `vsphere.host_net_interface_usage` emit per-interface throughput, packets, drops, errors, broadcast packets, multicast packets, unknown protocol frames, and combined usage;
  - labels are `id`, `datacenter`, `cluster`, `host`, `interface`, and `interface_instance`, plus opt-in enrichment labels;
- Added optional datastore cluster / StoragePod collection:
  - `collect_datastore_clusters` defaults to `false`;
  - `datastore_cluster_include` defaults to `/*` and matches `/Datacenter/DatastoreCluster`, datastore-cluster name, or managed object ID;
  - `max_datastore_clusters` defaults to `256`;
  - contexts `vsphere.datastore_cluster_space_utilization`, `vsphere.datastore_cluster_space_usage`, and `vsphere.datastore_cluster_storage_drs_status` emit capacity/free/used/utilization and Storage DRS enabled/disabled state.
- Added optional host disk/LUN/device performance collection:
  - `collect_host_disk_performance` defaults to `false`;
  - `host_disk_include` defaults to `*` and matches the raw vSphere performance instance or `instance:<disk_instance>`;
  - `max_host_disks` defaults to `1024`;
  - selected host `disk.*` counters are requested with wildcard instance `*` only when the option is enabled;
  - existing aggregate host disk metrics remain default-on and are not suppressed by the opt-in per-device surface;
  - contexts `vsphere.host_disk_device_io`, `vsphere.host_disk_device_iops`, `vsphere.host_disk_device_requests`, `vsphere.host_disk_device_latency`, `vsphere.host_disk_device_latency_breakdown`, `vsphere.host_disk_device_read_latency_breakdown`, `vsphere.host_disk_device_write_latency_breakdown`, `vsphere.host_disk_device_commands`, `vsphere.host_disk_device_command_events`, `vsphere.host_disk_device_queue_depth`, `vsphere.host_disk_device_scsi_reservation_conflicts`, and `vsphere.host_disk_device_scsi_reservation_conflicts_percentage` emit per-device throughput, IOPS, requests, latency, latency breakdowns, commands, command events, queue depth, and SCSI reservation conflicts;
  - labels are `id`, `datacenter`, `cluster`, `host`, `disk`, and `disk_instance`, plus opt-in enrichment labels;
- Added optional host storage-adapter performance collection:
  - `collect_host_storage_adapter_performance` defaults to `false`;
  - `host_storage_adapter_include` defaults to `*` and matches the raw vSphere performance instance, `adapter:<adapter_instance>`, or `instance:<adapter_instance>`;
  - `max_host_storage_adapters` defaults to `1024`;
  - selected host `storageAdapter.*` counters are requested with wildcard instance `*` only when the option is enabled;
  - aggregate-only `storageAdapter.maxTotalLatency.latest` is requested with an empty instance and emitted as a host-level storage-adapter aggregate metric;
  - contexts `vsphere.host_storage_adapter_io`, `vsphere.host_storage_adapter_commands`, `vsphere.host_storage_adapter_latency`, `vsphere.host_storage_adapter_queue`, `vsphere.host_storage_adapter_outstanding_io_percentage`, `vsphere.host_storage_adapter_throughput`, `vsphere.host_storage_adapter_throughput_contention`, and `vsphere.host_storage_adapter_max_latency` emit per-adapter throughput, commands, latency, queue, outstanding percentage, throughput usage/contention, and aggregate maximum latency;
  - per-adapter labels are `id`, `datacenter`, `cluster`, `host`, `adapter`, and `adapter_instance`, plus opt-in enrichment labels;
  - aggregate maximum-latency labels are `id`, `datacenter`, `cluster`, and `host`, plus opt-in enrichment labels;
- Added optional host storage-path performance collection:
  - `collect_host_storage_path_performance` defaults to `false`;
  - `host_storage_path_include` defaults to `*` and matches the raw vSphere performance instance, `path:<path_instance>`, or `instance:<path_instance>`;
  - `max_host_storage_paths` defaults to `1024`;
  - selected host `storagePath.*` counters are requested with wildcard instance `*` only when the option is enabled;
  - aggregate-only `storagePath.maxTotalLatency.latest` is requested with an empty instance and emitted as a host-level storage-path aggregate metric;
  - contexts `vsphere.host_storage_path_io`, `vsphere.host_storage_path_commands`, `vsphere.host_storage_path_latency`, `vsphere.host_storage_path_command_events`, `vsphere.host_storage_path_throughput`, `vsphere.host_storage_path_throughput_contention`, and `vsphere.host_storage_path_max_latency` emit per-path throughput, commands, latency, command events, throughput usage/contention, and aggregate maximum latency;
  - per-path labels are `id`, `datacenter`, `cluster`, `host`, `path`, and `path_instance`, plus opt-in enrichment labels;
  - aggregate maximum-latency labels are `id`, `datacenter`, `cluster`, and `host`, plus opt-in enrichment labels;
  - IQN/WWN labels are not emitted by default.
- Added optional host CPU-instance performance collection:
  - `collect_host_cpu_instance_performance` defaults to `false`;
  - `host_cpu_instance_include` defaults to `*` and matches the raw vSphere performance instance, `cpu:<cpu_instance>`, or `instance:<cpu_instance>`;
  - `max_host_cpu_instances` defaults to `1024`;
  - selected host `cpu.*` counters are requested with wildcard instance `*` only when the option is enabled;
  - contexts `vsphere.host_cpu_instance_utilization` and `vsphere.host_cpu_instance_time` emit per-instance usage, utilization, core utilization, used time, and idle time;
  - labels are `id`, `datacenter`, `cluster`, `host`, `cpu`, and `cpu_instance`, plus opt-in enrichment labels;
- Added optional aggregate host/VM power metric collection:
  - `collect_power_metrics` defaults to `false`;
  - host and VM power counters are requested only when the option is enabled;
  - this is not a per-child high-cardinality group, so it has no selector or max cap beyond the existing host/VM include selectors;
  - host contexts `vsphere.host_power_usage`, `vsphere.host_power_capacity_usage`, `vsphere.host_power_capacity_utilization`, and `vsphere.host_energy_usage` emit current power, power cap, capacity used/usable/idle/system/VM breakdown, capacity utilization, and energy;
  - VM contexts `vsphere.vm_power_usage` and `vsphere.vm_energy_usage` emit current power and energy;
  - host labels are `id`, `datacenter`, `cluster`, and `host`, plus opt-in enrichment labels;
  - VM labels are `id`, `datacenter`, `cluster`, `host`, and `vm`, plus opt-in enrichment labels;
  - non-powered-on hosts/VMs remain property/status-only because real-time performance query specs are skipped for them.
- Added opt-in vSphere user metadata labels:
  - `vsphere_tag_categories` defaults to an empty allowlist and matches vSphere tag category names with one glob pattern per YAML list item;
  - `custom_attributes` defaults to an empty allowlist and matches vSphere custom attribute names with one glob pattern per YAML list item;
  - `max_user_metadata_labels` defaults to `64` and caps vSphere tag/custom-attribute labels per discovered resource;
  - tag labels use `vsphere_tag_<sanitized_category>` keys and sort/join multiple tags in one category with the pipe character;
  - custom attribute labels use `vsphere_custom_attribute_<sanitized_name>` keys;
  - discovery reads SOAP `customValue` only when custom attributes are configured, and reads REST/vAPI tag associations only when tag categories are configured;
  - tag/custom-attribute API failures are warning-only so normal vSphere metric discovery continues.
- Evidence for P20 used Broadcom's current Storage Path Counters page, which documents `storagePath.*` counters as HostSystem instance metrics plus aggregate `maxTotalLatency`, and mirrored repository snapshots:
  - `datadog/integrations-core @ 1befb9012c44152b0aedfb17142041bcc9c1dc61`, `esxi/metadata.csv:167`, `vsphere/metadata.csv:244`, `vsphere/datadog_checks/vsphere/metrics.py:377`.
  - `newrelic/nri-vsphere @ 9366fcd3d597ae0712c94882331042d74fe38e22`, `vsphere-performance.metrics:140`, `test-data/SDDC-Datacenter/0027-PerformanceManager-PerfMgr.xml:4452`, `test-data/SDDC-Datacenter/0027-PerformanceManager-PerfMgr.xml:4716`.
- Evidence for P21 used Broadcom's current CPU Counters page, which documents CPU HostSystem instance counters, and mirrored repository snapshots:
  - `influxdata/telegraf @ 5a1147f1bb725ff8fd483ea55045506aa70db191`, `plugins/inputs/vsphere/sample.conf:57`, `plugins/inputs/vsphere/sample.conf:60`, `plugins/inputs/vsphere/sample.conf:65`, `plugins/inputs/vsphere/sample.conf:67`, `plugins/inputs/vsphere/sample.conf:68`.
  - `newrelic/nri-vsphere @ 9366fcd3d597ae0712c94882331042d74fe38e22`, `vsphere-performance.metrics:18`, `vsphere-performance.metrics:55`, `vsphere-performance.metrics:58`, `vsphere-performance.metrics:62`, `vsphere-performance.metrics:131`.
  - `open-telemetry/opentelemetry-collector-contrib @ 34ed18e037dc63e41c4b4a8356d2a13d55c768f4`, `receiver/vcenterreceiver/metrics.go:419`, `receiver/vcenterreceiver/metrics.go:474`.
- Evidence for P22 used Broadcom's current Power Counters page, which documents host and VM power/energy counters and host power-capacity counters, and mirrored repository snapshots:
  - `DataDog/integrations-core @ 1befb9012c44152b0aedfb17142041bcc9c1dc61`, `datadog_checks_base/datadog_checks/base/checks/libs/vmware/all_metrics.py:1333`, `datadog_checks_base/datadog_checks/base/checks/libs/vmware/all_metrics.py:1336`, `datadog_checks_base/datadog_checks/base/checks/libs/vmware/all_metrics.py:1339`, `vsphere/datadog_checks/vsphere/metrics.py:35`.
  - `influxdata/telegraf @ 5a1147f1bb725ff8fd483ea55045506aa70db191`, `plugins/inputs/vsphere/sample.conf:37`, `plugins/inputs/vsphere/sample.conf:97`, `plugins/inputs/vsphere/README.md:659`, `plugins/inputs/vsphere/README.md:660`, `plugins/inputs/vsphere/README.md:744`, `plugins/inputs/vsphere/README.md:745`, `plugins/inputs/vsphere/README.md:746`.
  - `newrelic/nri-vsphere @ 9366fcd3d597ae0712c94882331042d74fe38e22`, `vsphere-performance.metrics:121`, `vsphere-performance.metrics:137`, `vsphere-performance.metrics:138`, `vsphere-performance.metrics:230`, `vsphere-performance.metrics:240`.
- Evidence for P28 used Broadcom's current vSphere Automation API tag association endpoint (`list-attached-tags-on-objects`) and Web Services `CustomFieldsManager`/`customValue` model, plus mirrored repository snapshots:
  - `DataDog/integrations-core @ 1befb9012c44152b0aedfb17142041bcc9c1dc61`, `vsphere/datadog_checks/vsphere/data/conf.yaml.example:175`, `vsphere/datadog_checks/vsphere/config.py:80`.
  - `influxdata/telegraf @ 5a1147f1bb725ff8fd483ea55045506aa70db191`, `plugins/inputs/vsphere/README.md:213`, `plugins/inputs/vsphere/vsphere.go:23`.
  - `newrelic/nri-vsphere @ 9366fcd3d597ae0712c94882331042d74fe38e22`, `internal/collect/vms.go:21`, `internal/collect/vms.go:22`.
  - `elastic/beats @ 7bbe8ee6dcfbf416c53ceb7725909d37a499846c`, `metricbeat/module/vsphere/virtualmachine/virtualmachine.go:79`, `metricbeat/module/vsphere/virtualmachine/data.go:75`.
- Updated `config_schema.json`, stock `go.d/vsphere.conf`, `metadata.yaml`, generated integration docs, generated `charts.yaml`, config serialization fixtures, and the parity/compatibility specs for opt-in labels, VM disk capacity/performance, VM/host network-interface performance, host disk/LUN/device performance, host storage-adapter performance, host storage-path performance, host CPU-instance performance, host/VM power metrics, and datastore cluster metrics.
- Added optional vSAN collection:
  - `collect_vsan` defaults to `false`;
  - discovery requests cluster vSAN config, host vSAN node UUID, and VM instance UUID only when this option is enabled;
  - vSAN API calls run only for clusters whose vSAN config is enabled;
  - vSAN space usage and health use vSAN Management API managed objects;
  - vSAN performance uses the OTel/Datadog common entity families, but queries concrete discovered refs (`cluster-domclient:<uuid>`, `host-domclient:<uuid>`, and `virtual-machine:<uuid>`) instead of wildcard refs;
  - dedicated `vsan_cluster_include`, `vsan_host_include`, and `vsan_vm_include` selectors plus `max_vsan_clusters`, `max_vsan_hosts`, and `max_vsan_vms` caps bound both emitted series and vSAN API query scope;
  - contexts `vsphere.vsan_cluster_space_usage`, `vsphere.vsan_cluster_space_utilization`, `vsphere.vsan_cluster_health_status`, `vsphere.vsan_cluster_operations`, `vsphere.vsan_cluster_throughput`, `vsphere.vsan_cluster_latency`, `vsphere.vsan_cluster_congestions`, `vsphere.vsan_host_operations`, `vsphere.vsan_host_throughput`, `vsphere.vsan_host_latency`, `vsphere.vsan_host_congestions`, `vsphere.vsan_host_cache_hit_rate`, `vsphere.vsan_vm_operations`, `vsphere.vsan_vm_throughput`, and `vsphere.vsan_vm_latency` are emitted only when vSAN returns matching data;
  - cluster, host, and VM vSAN metrics remain default/job scoped;
  - vSAN API/performance failures log one warning per failure class and otherwise emit no vSAN series for that query.
- Evidence for P26 used govmomi vSAN client/types and mirrored repository snapshots:
  - `open-telemetry/opentelemetry-collector-contrib @ 34ed18e037dc63e41c4b4a8356d2a13d55c768f4`, `receiver/vcenterreceiver/client.go:377`, `receiver/vcenterreceiver/client.go:472`, `receiver/vcenterreceiver/metrics.go:575`, `receiver/vcenterreceiver/internal/mockserver/responses/cluster-vsan.xml:6`.
  - `influxdata/telegraf @ 5a1147f1bb725ff8fd483ea55045506aa70db191`, `plugins/inputs/vsphere/vsan.go:116`, `plugins/inputs/vsphere/vsan.go:221`, `plugins/inputs/vsphere/vsan.go:224`, `plugins/inputs/vsphere/vsan.go:248`.
  - `DataDog/integrations-core @ 1befb9012c44152b0aedfb17142041bcc9c1dc61`, `vsphere/datadog_checks/vsphere/config.py:100`, `vsphere/datadog_checks/vsphere/vsphere.py:600`, `vsphere/datadog_checks/vsphere/api.py:442`, `vsphere/datadog_checks/vsphere/metrics.py:491`.
- Updated `config_schema.json`, stock `go.d/vsphere.conf`, `metadata.yaml`, generated integration docs, config serialization fixtures, generated `charts.yaml`, and the parity/compatibility specs for opt-in vSAN metrics and dedicated vSAN selectors/caps.
- Residual P26 gap recorded: vSAN events are out of this PR by user decision, and deeper vSAN disk-group, disk, component, CMMDS, and all Telegraf entity-type metrics are not emitted by `collect_vsan` in this slice because they need an explicit Netdata NIDL/context mapping and bounded config policy before implementation.
- Added read-only vSphere Functions:
  - `vsphere:readiness` reports cached target/credential presence, initialized client/discovery/performance-counter state, inventory counts, optional metric/label gates, network topology gate, and cached vSAN counts for the selected vSphere job;
  - public `topology:vsphere` reports cached inventory topology actors/links for datacenters, clusters, hosts, VMs, datastores, datastore clusters, and resource pools for the selected vSphere job;
  - both Functions are job-scoped, require Cloud, expose no configured vCenter URL or credentials, and do not issue extra vCenter API calls when invoked.
- Added opt-in Network/DVPG topology discovery:
  - `collect_network_topology` defaults to `false`;
  - when enabled, discovery retrieves vSphere `Network` objects with `name`, `parent`, `summary`, `host`, and `vm`;
  - cached topology includes network actors with type, accessibility/status, host count, VM count, and host/VM network links;
  - no charts, metrics, or network path labels are created.
- Evidence for P30-P32 used local framework and topology patterns plus official/mirrored vSphere references:
  - `src/go/plugin/agent/jobmgr/funcctl/controller.go:128` shows module methods add the framework job selector unless `AgentWide` is set.
  - `src/go/plugin/agent/jobmgr/funcctl/controller.go:181` shows module methods can publish raw aliases, matching the SNMP topology pattern.
  - `src/plugins.d/FUNCTION_UI_SCHEMA.json:537` defines the topology response schema with actors and links.
  - `src/go/plugin/go.d/collector/snmp_topology/func_topology.go:12` shows the production topology method ID `topology:snmp`; `src/go/plugin/go.d/collector/snmp_topology/func_topology_presentation.go:33` shows its presentation metadata.
  - Broadcom current vSphere Web Services API documents `NetworkSummary.accessible`, `Network.host`, `Network.vm`, and `DistributedVirtualPortgroup`.
  - `newrelic/nri-vsphere @ 9366fcd3d597ae0712c94882331042d74fe38e22`, `internal/collect/networks.go:14`, `internal/collect/networks.go:37`, `internal/process/datacenter.go:104`, `internal/process/hosts.go:95`, and `internal/process/vms.go:113`.
- Post-review hardening fixed reviewer-identified production-readiness risks:
  - optional datastore-cluster discovery now fails soft with an operator warning instead of aborting the whole collection when StoragePod permissions/API calls fail;
  - optional-surface selectors and vSphere tag/custom-attribute label allowlists now reject empty negative patterns and pure-negative lists during `Init()`;
  - `metadata.yaml` now declares the public topology Function ID as `topology:vsphere`, matching the registered Function alias;
  - stock config, DYNCFG schema, metadata, and generated docs now explain that `collect_vsan` queries concrete discovered vSAN entity refs bounded by dedicated vSAN selectors and caps;
  - vSAN performance scraping now queries concrete discovered cluster/host/VM entity refs instead of wildcard refs.
- Round 3 reviewer hardening fixed the remaining actionable production-readiness risks:
  - `Init()` is now re-entrant for DynCfg/reload-style paths: it stops the previous discovery task, clears cached resources, chart runtime maps, matchers, samples, and Function-visible runtime state, then rebuilds the client/discoverer/scraper from the current config;
  - optional Network/DVPG topology discovery now fails soft with an operator warning instead of aborting the whole metric collection when network permissions/API calls fail;
  - tests now cover re-entrant `Init()` after a selector change, optional network discovery fail-soft behavior, tag/custom-attribute enrichment API failure fail-soft behavior, and partial-init readiness/topology Function calls.
- Round 4 reviewer hardening fixed verified production-readiness issues and cheap low-risk review notes:
  - collector cleanup and re-init now close the vSphere client/container view and wait for the discovery task to stop;
  - collector-generated vnode GUID generation was removed from this PR by the 2026-05-20 hard-removal decision;
  - datastore-cluster V2 metric labels now include opt-in vSphere tag/custom-attribute enrichment labels;
  - resource builders skip malformed parentless folders/clusters/hosts/datastores/datastore clusters instead of panicking;
  - folder and snapshot traversal now have defensive cycle/depth guards;
  - VM/host NIC selectors now accept `instance:` as a universal prefix in addition to `interface:`;
  - `autodetection_retry` docs now match the schema default of 60 seconds;
  - dead datastore-cluster discovery state was removed, topology link preallocation now accounts for network host/VM links, and malformed vSAN performance entity refs are skipped instead of dropping the whole batch.
- Round 5 reviewer hardening fixed additional verified production-readiness risks:
  - vSphere client cleanup now logs out the REST/vAPI tag session, clears cached REST/tag/vSAN clients, and logs out the SOAP session if container-view creation fails after login;
  - `collect_vsan` now has dedicated vSAN selectors and max caps for clusters, hosts, and VMs, and tests verify those controls bound the resources passed to the vSAN scraper;
  - `metadata.yaml` now uses the actual health template name `vsphere_vm_mem_utilization`;
  - missing vCenter performance counters now produce one operator warning per counter name instead of silently reducing the metric list;
  - stock config, DYNCFG schema, metadata, generated integration docs, and config serialization fixtures were updated for the vSAN selector/cap options.
- Round 6 reviewer hardening fixed additional verified production-readiness risks:
  - vCenter 7.0/8.0/9.0 performance query batching now uses the intended 256-query cap instead of the legacy 64-query cap reserved for pre-6.5 endpoints;
  - host and VM performance scrape empty-result cycles now warn and continue with property/status, datastore, cluster, resource-pool, and vSAN collection instead of aborting the full cycle after property metrics were already collected;
  - DynCfg schema required fields now match the actual required connection fields and no longer require selector keys that have defaults;
  - lazy REST tag-manager and vSAN client initialization is mutex-guarded and `Close()` clears cached credentials after logout;
  - VMware's official storage-adapter counter table confirms `storageAdapter.throughput.usag.average` is the correct API counter spelling, so the Round 6 typo finding was rejected as a false positive.
- Round 7 reviewer hardening handled additional low-risk review notes:
  - `Client.Close()` now holds the lazy-client mutex while clearing REST/tag/vSAN cached clients and cached credentials;
  - resource-pool property refresh documents and tests govmomi's `mo.ResourcePool.Runtime`/`Config` zero-value behavior, rejecting the nil-panic finding with type evidence;
  - the ESXi memory health alert summary now uses "memory" instead of "Ram";
  - an empty scraper package test was removed.

## Validation

Acceptance criteria evidence:

- Feasibility and decision evidence is recorded in this SOW.
- The compatibility-manifest prep gate is drafted in `.agents/sow/specs/vsphere-v1-compatibility-manifest.md`.
- The executable V1 golden baseline is added at `src/go/plugin/go.d/collector/vsphere/testdata/v1_compat_manifest.json` and checked by `TestCollector_V1CompatibilityManifest`.
- The parity-matrix prep gate is drafted in `.agents/sow/specs/vsphere-parity-matrix.md`.
- Framework V2 migration acceptance is satisfied by `TestCollector_ChartTemplateYAML`, `TestCollector_V2CompatibilitySurface`, and the full vSphere package test. Snapshot, datastore aggregate enrichment, VM/host power-state controls, VM/host property-status, cluster property-status, inventory-count, opt-in label enrichment, opt-in vSphere tag/custom-attribute labels, opt-in VM disk capacity/performance, opt-in VM network-interface performance, opt-in host physical network-interface performance, opt-in host disk/LUN/device performance, opt-in host storage-adapter performance, opt-in host storage-path performance, opt-in host CPU-instance performance, opt-in host/VM power metrics, opt-in datastore-cluster metrics, opt-in vSAN metrics, read-only readiness Function, cached topology Function, and opt-in Network/DVPG topology discovery criteria are satisfied for the currently implemented metrics, controls, and non-metric surfaces.

Tests or equivalent validation:

- `UPDATE_VSPHERE_COMPAT=1 go test ./plugin/go.d/collector/vsphere -run TestCollector_V1CompatibilityManifest -count=1` passed from `src/go/` while generating the golden fixture.
- `UPDATE_VSPHERE_CHARTS=1 go test ./plugin/go.d/collector/vsphere -run TestCollector_ChartTemplateYAML -count=1` passed from `src/go/` while generating `charts.yaml`.
- `go test ./plugin/go.d/collector/vsphere -run 'TestCollector_(V2CompatibilitySurface|ChartTemplateYAML|Collect)$' -count=1` passed from `src/go/`.
- `go test ./plugin/go.d/collector/vsphere -count=1` passed from `src/go/`.
- `go test ./plugin/go.d/collector/vsphere ./plugin/framework/charttpl ./plugin/framework/chartengine ./plugin/go.d/pkg/collecttest -count=1` passed from `src/go/`.
- `go test ./plugin/go.d/collector/vsphere ./plugin/go.d/collector/vsphere/discover ./plugin/framework/charttpl ./plugin/framework/chartengine ./plugin/go.d/pkg/collecttest -count=1` passed from `src/go/` after snapshot metrics were added.
- `go test ./plugin/go.d/collector/vsphere ./plugin/go.d/collector/vsphere/discover ./plugin/go.d/collector/vsphere/scrape -count=1` passed from `src/go/` after datastore enrichment was added.
- `go test ./plugin/go.d/collector/vsphere -run 'TestCollector_(ConfigurationSerialize|DefaultPowerStates|Init_ReturnsFalseIfInvalidPowerState|Collect_NonPowered|Collect)$|TestSnapshotMaxAgeSeconds' -count=1` passed from `src/go/` after power-state config and property-only collection were added.
- `go test ./plugin/go.d/collector/vsphere/discover -run 'TestDiscoverer_(buildHostsPowerStateFilter|buildVMsPowerStateFilterAndNilHost|setVMHierarchyUsesFolderDatacenterWhenHostMissing|Discover|build|setHierarchy|collectMetricLists)$' -count=1` passed from `src/go/` after configurable power-state discovery was added.
- `go test ./plugin/go.d/collector/vsphere/scrape -run 'Test_new.*Power|TestScraper_Scrape' -count=1` passed from `src/go/` after non-powered-on perf query skipping was added.
- `UPDATE_VSPHERE_COMPAT=1 go test ./plugin/go.d/collector/vsphere -run TestCollector_V1CompatibilityManifest -count=1` passed from `src/go/` after adding power-state metrics and regenerated the compatibility fixture.
- `UPDATE_VSPHERE_CHARTS=1 go test ./plugin/go.d/collector/vsphere -run TestCollector_ChartTemplateYAML -count=1` passed from `src/go/` after adding power-state metrics and regenerated `charts.yaml`.
- `go test ./plugin/go.d/collector/vsphere ./plugin/go.d/collector/vsphere/discover ./plugin/go.d/collector/vsphere/scrape ./plugin/framework/charttpl ./plugin/framework/chartengine ./plugin/go.d/pkg/collecttest -count=1` passed from `src/go/` after adding power-state config and metrics.
- `go test ./plugin/go.d/collector/vsphere -run 'TestCollector_(ChartTemplateYAML|Collect)$|TestWrite.*' -count=1` passed from `src/go/` after adding VM/host property-status metrics.
- `go test ./plugin/go.d/collector/vsphere/discover -run 'TestDiscoverer_(buildHostsPowerStateFilter|buildVMsPowerStateFilterAndNilHost|Discover|build|setHierarchy|collectMetricLists)$' -count=1` passed from `src/go/` after adding VM/host property-status discovery fields.
- `UPDATE_VSPHERE_COMPAT=1 go test ./plugin/go.d/collector/vsphere -run TestCollector_V1CompatibilityManifest -count=1` passed from `src/go/` after adding VM/host property-status metrics and regenerated the compatibility fixture.
- `UPDATE_VSPHERE_CHARTS=1 go test ./plugin/go.d/collector/vsphere -run TestCollector_ChartTemplateYAML -count=1` passed from `src/go/` after adding VM/host property-status metrics and regenerated `charts.yaml`.
- `go test ./plugin/go.d/collector/vsphere ./plugin/go.d/collector/vsphere/discover ./plugin/go.d/collector/vsphere/scrape ./plugin/framework/charttpl ./plugin/framework/chartengine ./plugin/go.d/pkg/collecttest -count=1` passed from `src/go/` after adding VM/host property-status metrics.
- `go test ./plugin/go.d/collector/vsphere -run 'TestCollector_Collect|TestWriteClusterPropertyMetrics_ConfigStates|TestCollector_ChartTemplateYAML' -count=1` passed from `src/go/` after adding cluster DRS/HA property-status metrics.
- `UPDATE_VSPHERE_COMPAT=1 go test ./plugin/go.d/collector/vsphere -run TestCollector_V1CompatibilityManifest -count=1` passed from `src/go/` after adding cluster DRS/HA property-status metrics and regenerated the compatibility fixture.
- `UPDATE_VSPHERE_CHARTS=1 go test ./plugin/go.d/collector/vsphere -run TestCollector_ChartTemplateYAML -count=1` passed from `src/go/` after adding cluster DRS/HA property-status metrics and regenerated `charts.yaml`.
- `go test ./plugin/go.d/collector/vsphere ./plugin/go.d/collector/vsphere/discover ./plugin/go.d/collector/vsphere/scrape ./plugin/framework/charttpl ./plugin/framework/chartengine ./plugin/go.d/pkg/collecttest -count=1` passed from `src/go/` after adding cluster DRS/HA property-status metrics.
- `go test ./plugin/go.d/collector/vsphere -run 'TestCollector_(Collect|ChartTemplateYAML|V2CompatibilitySurface)$|TestCollectInventory' -count=1` passed from `src/go/` after adding inventory-count metrics.
- `UPDATE_VSPHERE_COMPAT=1 go test ./plugin/go.d/collector/vsphere -run TestCollector_V1CompatibilityManifest -count=1` passed from `src/go/` after adding inventory-count metrics and regenerated the compatibility fixture.
- `UPDATE_VSPHERE_CHARTS=1 go test ./plugin/go.d/collector/vsphere -run TestCollector_ChartTemplateYAML -count=1` passed from `src/go/` after adding inventory-count metrics and regenerated `charts.yaml`.
- `go test ./plugin/go.d/collector/vsphere ./plugin/go.d/collector/vsphere/discover ./plugin/go.d/collector/vsphere/scrape ./plugin/framework/charttpl ./plugin/framework/chartengine ./plugin/go.d/pkg/collecttest -count=1` passed from `src/go/` after adding inventory-count metrics.
- `go test ./plugin/go.d/collector/vsphere -run 'TestCollector_(OptionalVnodesDefaultOff|ESXIVnodesScopeOnlyHostMetrics|VMVnodesScopeOnlyVMMetrics)'` passed from `src/go/` after adding optional ESXi/VM vnodes. These tests were deleted with the feature by the 2026-05-20 hard-removal decision.
- `go test ./plugin/go.d/collector/vsphere/...` passed from `src/go/` after adding optional ESXi/VM vnodes and config/docs updates. The feature and its tests were deleted by the 2026-05-20 hard-removal decision.
- `go test ./plugin/go.d/collector/vsphere -run 'TestCollector_(AddsInventoryPathLabel|AddsVMGuestLabels|Init_ReturnsFalseIfInvalidVMGuestLabel|V2CompatibilitySurface)$' -count=1` passed from `src/go/` after adding opt-in inventory-path and VM guest labels.
- `go test ./plugin/go.d/collector/vsphere -run 'TestCollector_(Init_ReturnsFalseIfInvalidVMDisksConfig|CollectsVMDiskCapacity|VMDiskCapacityHonorsSelectorAndCap|VMDiskCapacityHonorsVMVnodes|ChartTemplateYAML)$' -count=1` passed from `src/go/` after adding opt-in VM disk capacity metrics. The vnode-specific test named here was deleted with the feature by the 2026-05-20 hard-removal decision.
- `go test ./plugin/go.d/collector/vsphere -run 'TestCollector_(Init_ReturnsFalseIfInvalidDatastoreClusterConfig|CollectsDatastoreClusterMetrics|DatastoreClusterMetricsHonorSelectorAndCap|ChartTemplateYAML)$' -count=1` passed from `src/go/` after adding opt-in datastore cluster metrics.
- `go test ./plugin/go.d/collector/vsphere/... -count=1` passed from `src/go/` after adding opt-in label enrichment, VM disk capacity, and datastore cluster metrics.
- `UPDATE_VSPHERE_CHARTS=1 go test ./plugin/go.d/collector/vsphere -run TestCollector_ChartTemplateYAML -count=1` passed from `src/go/` and regenerated `charts.yaml` after adding opt-in VM disk performance metrics.
- `go test ./plugin/go.d/collector/vsphere -run 'TestCollector_VMDiskPerformance|TestCollector_Init_ReturnsFalseIfInvalidVMDiskConfig|TestCollector_ChartTemplateYAML' -count=1` passed from `src/go/` after adding opt-in VM disk performance metrics.
- `go test ./plugin/go.d/collector/vsphere/discover -run 'TestSimpleVMMetricListAddsVirtualDiskInstanceMetricsWhenEnabled|TestDiscoverer_collectMetricLists|TestDiscoverer_Discover' -count=1` passed from `src/go/` after adding wildcard `virtualDisk.*` metric discovery.
- `go test ./plugin/go.d/collector/vsphere/... -count=1` passed from `src/go/` after adding opt-in VM disk performance metrics and docs/artifact updates.
- `UPDATE_VSPHERE_CHARTS=1 go test ./plugin/go.d/collector/vsphere -run TestCollector_ChartTemplateYAML -count=1` passed from `src/go/` and regenerated `charts.yaml` after adding opt-in VM network-interface performance metrics.
- `go test ./plugin/go.d/collector/vsphere -run 'TestCollector_VMNICPerformance|TestCollector_Init_ReturnsFalseIfInvalidVMNICConfig|TestCollector_ChartTemplateYAML' -count=1` passed from `src/go/` after adding opt-in VM network-interface performance metrics.
- `go test ./plugin/go.d/collector/vsphere/discover -run 'TestSimpleVMMetricListAddsNetworkInstanceMetricsWhenEnabled|TestSimpleVMMetricListAddsVirtualDiskInstanceMetricsWhenEnabled|TestDiscoverer_collectMetricLists|TestDiscoverer_Discover' -count=1` passed from `src/go/` after adding wildcard VM `net.*` metric discovery while preserving aggregate VM network metrics.
- `go test ./plugin/go.d/collector/vsphere -run 'TestCollector_Collect|TestCollector_VMNICPerformance|TestCollector_Init_ReturnsFalseIfInvalidVMNICConfig|TestCollector_ChartTemplateYAML' -count=1` passed from `src/go/` after fixing the aggregate VM network compatibility regression caught by `TestCollector_Collect`.
- `go test ./plugin/go.d/collector/vsphere/... -count=1` passed from `src/go/` after adding opt-in VM network-interface performance metrics and docs/artifact updates.
- `UPDATE_VSPHERE_CHARTS=1 go test ./plugin/go.d/collector/vsphere -run TestCollector_ChartTemplateYAML -count=1` passed from `src/go/` and regenerated `charts.yaml` after adding opt-in host physical network-interface performance metrics.
- `go test ./plugin/go.d/collector/vsphere -run 'TestCollector_HostNICPerformance|TestCollector_Init_ReturnsFalseIfInvalidHostNICConfig|TestCollector_Collect|TestCollector_ChartTemplateYAML' -count=1` passed from `src/go/` after adding opt-in host physical network-interface performance metrics.
- `go test ./plugin/go.d/collector/vsphere/discover -run 'TestSimpleHostMetricListAddsNetworkInstanceMetricsWhenEnabled|TestSimpleVMMetricListAddsNetworkInstanceMetricsWhenEnabled|TestDiscoverer_collectMetricLists|TestDiscoverer_Discover' -count=1` passed from `src/go/` after adding wildcard host `net.*` metric discovery while preserving aggregate host network metrics.
- `go test ./plugin/go.d/collector/vsphere/... -count=1` passed from `src/go/` after adding opt-in host physical network-interface performance metrics and docs/artifact updates.
- `UPDATE_VSPHERE_CHARTS=1 go test ./plugin/go.d/collector/vsphere -run TestCollector_ChartTemplateYAML -count=1` passed from `src/go/` and regenerated `charts.yaml` after adding opt-in host disk/LUN/device performance metrics.
- `go test ./plugin/go.d/collector/vsphere -run 'TestCollector_(ConfigurationSerialize|HostDiskPerformance|Init_ReturnsFalseIfInvalidHostDiskConfig|Collect|ChartTemplateYAML)' -count=1` passed from `src/go/` after adding opt-in host disk/LUN/device performance metrics.
- `go test ./plugin/go.d/collector/vsphere/discover -run 'TestSimpleHostMetricListAddsDiskInstanceMetricsWhenEnabled|TestSimpleHostMetricListAddsNetworkInstanceMetricsWhenEnabled|TestDiscoverer_collectMetricLists|TestDiscoverer_Discover' -count=1` passed from `src/go/` after adding wildcard host `disk.*` metric discovery while preserving aggregate host disk metrics.
- `go test ./plugin/go.d/collector/vsphere/... -count=1` passed from `src/go/` after adding opt-in host disk/LUN/device performance metrics and docs/artifact updates.
- `UPDATE_VSPHERE_CHARTS=1 go test ./plugin/go.d/collector/vsphere -run TestCollector_ChartTemplateYAML -count=1` passed from `src/go/` and regenerated `charts.yaml` after adding opt-in host storage-adapter performance metrics.
- `go test ./plugin/go.d/collector/vsphere -run 'TestCollector_(ConfigurationSerialize|HostStorageAdapterPerformance|Init_ReturnsFalseIfInvalidHostStorageAdapterConfig|ChartTemplateYAML)' -count=1` passed from `src/go/` after adding opt-in host storage-adapter performance metrics.
- `go test ./plugin/go.d/collector/vsphere/discover -run 'TestSimpleHostMetricListAddsStorageAdapterInstanceMetricsWhenEnabled|TestSimpleHostMetricListAddsDiskInstanceMetricsWhenEnabled|TestSimpleHostMetricListAddsNetworkInstanceMetricsWhenEnabled|TestDiscoverer_collectMetricLists|TestDiscoverer_Discover' -count=1` passed from `src/go/` after adding wildcard host `storageAdapter.*` metric discovery and the aggregate storage-adapter maximum-latency counter.
- `go test ./plugin/go.d/collector/vsphere/... -count=1` passed from `src/go/` after adding opt-in host storage-adapter performance metrics and docs/artifact updates.
- `UPDATE_VSPHERE_CHARTS=1 go test ./plugin/go.d/collector/vsphere -run TestCollector_ChartTemplateYAML -count=1` passed from `src/go/` and regenerated `charts.yaml` after adding opt-in host storage-path performance metrics.
- `go test ./plugin/go.d/collector/vsphere -run 'TestCollector_(HostStoragePathPerformance|Init_ReturnsFalseIfInvalidHostStoragePathConfig|ChartTemplateYAML)' -count=1` passed from `src/go/` after wiring the host storage-path code and chart templates.
- `go test ./plugin/go.d/collector/vsphere -run 'TestCollector_(ConfigurationSerialize|HostStoragePathPerformance|Init_ReturnsFalseIfInvalidHostStoragePathConfig|ChartTemplateYAML)' -count=1` passed from `src/go/` after adding host storage-path config serialization fixtures and schema entries.
- `go test ./plugin/go.d/collector/vsphere/discover -run 'TestSimpleHostMetricListAddsStoragePathInstanceMetricsWhenEnabled|TestSimpleHostMetricListAddsStorageAdapterInstanceMetricsWhenEnabled|TestDiscoverer_collectMetricLists|TestDiscoverer_Discover' -count=1` passed from `src/go/` after adding wildcard host `storagePath.*` metric discovery and the aggregate storage-path maximum-latency counter.
- `go test ./plugin/go.d/collector/vsphere -count=1 -failfast` passed from `src/go/` after adding opt-in host storage-path performance metrics.
- `go test ./plugin/go.d/collector/vsphere/... -count=1` passed from `src/go/` after adding opt-in host storage-path performance metrics and docs/artifact updates.
- `UPDATE_VSPHERE_CHARTS=1 go test ./plugin/go.d/collector/vsphere -run TestCollector_ChartTemplateYAML -count=1` passed from `src/go/` and regenerated `charts.yaml` after adding opt-in host CPU-instance performance metrics.
- `go test ./plugin/go.d/collector/vsphere -run 'TestCollector_(ConfigurationSerialize|HostCPUInstancePerformance|Init_ReturnsFalseIfInvalidHostCPUInstanceConfig|ChartTemplateYAML)' -count=1` passed from `src/go/` after adding opt-in host CPU-instance performance metrics and config artifacts.
- `go test ./plugin/go.d/collector/vsphere/discover -run 'TestSimpleHostMetricListAddsCPUInstanceMetricsWhenEnabled|TestSimpleHostMetricListAddsStoragePathInstanceMetricsWhenEnabled|TestDiscoverer_collectMetricLists|TestDiscoverer_Discover' -count=1` passed from `src/go/` after adding wildcard host `cpu.*` metric discovery while preserving aggregate host CPU metrics.
- `go test ./plugin/go.d/collector/vsphere/... -count=1` passed from `src/go/` after adding opt-in host CPU-instance performance metrics and docs/artifact updates.
- `UPDATE_VSPHERE_CHARTS=1 go test ./plugin/go.d/collector/vsphere -run TestCollector_ChartTemplateYAML -count=1` passed from `src/go/` and regenerated `charts.yaml` after adding opt-in host/VM power metrics.
- `go test ./plugin/go.d/collector/vsphere -run 'TestCollector_(PowerMetrics|ConfigurationSerialize|ChartTemplateYAML)' -count=1` passed from `src/go/` after adding opt-in host/VM power metrics and config artifacts.
- `go test ./plugin/go.d/collector/vsphere/discover -run 'TestSimple.*Power|TestDiscoverer_collectMetricLists|TestDiscoverer_Discover' -count=1` passed from `src/go/` after adding optional host/VM `power.*` metric discovery while preserving default aggregate host/VM metrics.
- `go test ./plugin/go.d/collector/vsphere -count=1` passed from `src/go/` after adding opt-in host/VM power metrics.
- `go test ./plugin/go.d/collector/vsphere/... -count=1` passed from `src/go/` after adding opt-in host/VM power metrics and docs/artifact updates.
- `go test ./plugin/go.d/collector/vsphere/discover -run 'Test(Add|DiscovererPath|ResourceLabels|Discoverer_buildVMsPower)' -count=1` passed from `src/go/` after adding opt-in vSphere tag/custom-attribute labels.
- `go test ./plugin/go.d/collector/vsphere -run 'Test(UserMetadataPatternMatcherPreservesListItems|Collector_(OptInInventoryPathAndVMGuestLabels|Init_ReturnsFalseIfInvalidUserMetadataLabelConfig|ConfigurationSerialize))' -count=1` passed from `src/go/` after adding opt-in vSphere tag/custom-attribute labels and list-item-preserving glob matching for names with spaces.
- `go test ./plugin/go.d/collector/vsphere -count=1` passed from `src/go/` after adding opt-in vSphere tag/custom-attribute labels and docs/artifact updates.
- `go test ./plugin/go.d/collector/vsphere/... -count=1` passed from `src/go/` after adding opt-in vSphere tag/custom-attribute labels and docs/artifact updates.
- `ruby -e 'require "json"; JSON.parse(File.read("src/go/plugin/go.d/collector/vsphere/config_schema.json")); require "yaml"; YAML.load_file("src/go/plugin/go.d/collector/vsphere/metadata.yaml"); YAML.load_file("src/go/plugin/go.d/collector/vsphere/charts.yaml"); YAML.load_file("src/go/plugin/go.d/config/go.d/vsphere.conf")'` passed from the repo root after adding power-state config and docs.
- `ruby -e 'require "yaml"; YAML.load_file("src/go/plugin/go.d/collector/vsphere/metadata.yaml"); YAML.load_file("src/go/plugin/go.d/collector/vsphere/charts.yaml")'` passed from the repo root.
- `ruby -e 'require "yaml"; YAML.load_file("src/go/plugin/go.d/config/go.d/vsphere.conf")'` passed from the repo root after the stock config comment expansion.
- `git diff --check -- src/go/plugin/go.d/config/go.d/vsphere.conf` passed from the repo root after the stock config comment expansion.
- `git diff --check` passed from the repo root.
- `python3 integrations/gen_integrations.py` passed from the repo root to create the local integrations index required by the docs generator.
- `python3 integrations/gen_docs_integrations.py -c go.d.plugin/vsphere` passed from the repo root and regenerated `src/go/plugin/go.d/collector/vsphere/integrations/vmware_vcenter_server.md` from `metadata.yaml`.
- `python3 integrations/gen_integrations.py && python3 integrations/gen_docs_integrations.py -c go.d.plugin/vsphere` passed from the repo root after adding opt-in label enrichment, VM disk capacity, and datastore cluster metrics.
- `python3 integrations/gen_integrations.py && python3 integrations/gen_docs_integrations.py -c go.d.plugin/vsphere` passed from the repo root after adding opt-in VM disk performance metrics.
- `python3 integrations/gen_integrations.py && python3 integrations/gen_docs_integrations.py -c go.d.plugin/vsphere` passed from the repo root after adding opt-in VM network-interface performance metrics.
- `python3 integrations/gen_integrations.py && python3 integrations/gen_docs_integrations.py -c go.d.plugin/vsphere` passed from the repo root after adding opt-in host physical network-interface performance metrics.
- `python3 integrations/gen_integrations.py && python3 integrations/gen_docs_integrations.py -c go.d.plugin/vsphere` passed from the repo root after adding opt-in host disk/LUN/device performance metrics.
- `python3 integrations/gen_integrations.py && python3 integrations/gen_docs_integrations.py -c go.d.plugin/vsphere` passed from the repo root after adding opt-in host storage-adapter performance metrics.
- `python3 integrations/gen_integrations.py && python3 integrations/gen_docs_integrations.py -c go.d.plugin/vsphere` passed from the repo root after adding opt-in host storage-path performance metrics.
- `python3 integrations/gen_integrations.py && python3 integrations/gen_docs_integrations.py -c go.d.plugin/vsphere` passed from the repo root after adding opt-in host CPU-instance performance metrics.
- `python3 integrations/gen_integrations.py` passed from the repo root after adding opt-in host/VM power metrics.
- `python3 integrations/gen_docs_integrations.py -c go.d.plugin/vsphere` passed from the repo root after adding opt-in host/VM power metrics.
- `python3 integrations/gen_integrations.py` passed from the repo root after adding opt-in vSphere tag/custom-attribute labels.
- `python3 integrations/gen_docs_integrations.py -c go.d.plugin/vsphere` passed from the repo root after adding opt-in vSphere tag/custom-attribute labels.
- `ruby -e 'require "json"; JSON.parse(File.read("src/go/plugin/go.d/collector/vsphere/config_schema.json")); require "yaml"; YAML.load_file("src/go/plugin/go.d/collector/vsphere/metadata.yaml"); YAML.load_file("src/go/plugin/go.d/collector/vsphere/charts.yaml"); YAML.load_file("src/go/plugin/go.d/config/go.d/vsphere.conf")'` passed from the repo root after adding opt-in VM disk performance metrics and regenerating docs.
- `ruby -e 'require "json"; JSON.parse(File.read("src/go/plugin/go.d/collector/vsphere/config_schema.json")); require "yaml"; YAML.load_file("src/go/plugin/go.d/collector/vsphere/metadata.yaml"); YAML.load_file("src/go/plugin/go.d/collector/vsphere/charts.yaml"); YAML.load_file("src/go/plugin/go.d/config/go.d/vsphere.conf")'` passed from the repo root after adding opt-in host disk/LUN/device performance metrics and regenerating docs.
- `ruby -e 'require "json"; JSON.parse(File.read("src/go/plugin/go.d/collector/vsphere/config_schema.json")); require "yaml"; YAML.load_file("src/go/plugin/go.d/collector/vsphere/metadata.yaml"); YAML.load_file("src/go/plugin/go.d/collector/vsphere/charts.yaml"); YAML.load_file("src/go/plugin/go.d/config/go.d/vsphere.conf")'` passed from the repo root after adding opt-in host storage-adapter performance metrics and regenerating docs.
- `ruby -e 'require "json"; require "yaml"; JSON.parse(File.read("src/go/plugin/go.d/collector/vsphere/config_schema.json")); JSON.parse(File.read("src/go/plugin/go.d/collector/vsphere/testdata/config.json")); YAML.load_file("src/go/plugin/go.d/collector/vsphere/testdata/config.yaml"); YAML.load_file("src/go/plugin/go.d/collector/vsphere/metadata.yaml"); YAML.load_file("src/go/plugin/go.d/config/go.d/vsphere.conf")'` passed from the repo root after adding opt-in host storage-path performance metrics and regenerating docs.
- `ruby -e 'require "json"; require "yaml"; JSON.parse(File.read("src/go/plugin/go.d/collector/vsphere/config_schema.json")); JSON.parse(File.read("src/go/plugin/go.d/collector/vsphere/testdata/config.json")); YAML.load_file("src/go/plugin/go.d/collector/vsphere/testdata/config.yaml"); YAML.load_file("src/go/plugin/go.d/collector/vsphere/metadata.yaml"); YAML.load_file("src/go/plugin/go.d/collector/vsphere/charts.yaml"); YAML.load_file("src/go/plugin/go.d/config/go.d/vsphere.conf")'` passed from the repo root after adding opt-in host CPU-instance performance metrics and regenerating docs.
- `ruby -e 'require "json"; require "yaml"; JSON.parse(File.read("src/go/plugin/go.d/collector/vsphere/config_schema.json")); JSON.parse(File.read("src/go/plugin/go.d/collector/vsphere/testdata/config.json")); YAML.safe_load(File.read("src/go/plugin/go.d/collector/vsphere/testdata/config.yaml"), permitted_classes: [], aliases: true); YAML.safe_load(File.read("src/go/plugin/go.d/collector/vsphere/metadata.yaml"), permitted_classes: [], aliases: true); YAML.safe_load(File.read("src/go/plugin/go.d/collector/vsphere/charts.yaml"), permitted_classes: [], aliases: true)'` passed from the repo root after adding opt-in host/VM power metrics and regenerating docs.
- `ruby -e 'require "json"; require "yaml"; JSON.parse(File.read("src/go/plugin/go.d/collector/vsphere/config_schema.json")); JSON.parse(File.read("src/go/plugin/go.d/collector/vsphere/testdata/config.json")); YAML.safe_load(File.read("src/go/plugin/go.d/collector/vsphere/testdata/config.yaml"), permitted_classes: [], aliases: true); YAML.safe_load(File.read("src/go/plugin/go.d/collector/vsphere/metadata.yaml"), permitted_classes: [], aliases: true); YAML.safe_load(File.read("src/go/plugin/go.d/collector/vsphere/charts.yaml"), permitted_classes: [], aliases: true)'` passed from the repo root after adding opt-in vSphere tag/custom-attribute labels and regenerating docs.
- `git diff --check` passed from the repo root after adding opt-in VM disk performance metrics.
- `git diff --check` passed from the repo root after adding opt-in host disk/LUN/device performance metrics.
- `git diff --check` passed from the repo root after adding opt-in host storage-adapter performance metrics.
- `git diff --check` passed from the repo root after adding opt-in host storage-path performance metrics.
- `git diff --check` passed from the repo root after adding opt-in host CPU-instance performance metrics.
- `git diff --check` passed from the repo root after adding opt-in vSphere tag/custom-attribute labels.
- `.agents/sow/audit.sh .agents/sow/current/SOW-0015-20260507-vsphere-v2-parity-enrichment.md` passed for this SOW after adding opt-in VM disk performance metrics; the audit still reports unrelated pre-existing repo-wide partial-state warnings for `SOW-0012` and legacy non-project skill classification.
- `python3 - <<'PY' ... build_path(...) ... PY` passed from the repo root and verified the integration docs generator resolves both `blob/master` and `edit/master` metadata links to the local collector path.
- `.agents/sow/audit.sh .agents/sow/current/SOW-0015-20260507-vsphere-v2-parity-enrichment.md` passed for this SOW; the audit still reports pre-existing repo-wide partial-state warnings for `SOW-0012` and legacy non-project skill classification.
- `go test ./plugin/go.d/collector/vsphere -run 'TestVSphereMethods|TestFuncReadiness|TestFuncTopology' -count=1` passed from `src/go/` after adding read-only readiness and cached topology Functions.
- `go test ./plugin/go.d/collector/vsphere/discover -run 'TestDiscoverer_DiscoverNetworkTopologyOptIn|TestNewNetwork|TestDiscoverer_setHierarchy' -count=1` passed from `src/go/` after adding opt-in Network/DVPG topology discovery.
- `go test ./plugin/go.d/collector/vsphere -run 'TestCollector_ConfigurationSerialize|TestFuncTopology|TestFuncReadiness|TestVSphereMethods' -count=1` passed from `src/go/` after adding `collect_network_topology` and Function metadata.
- `go test ./plugin/go.d/collector/vsphere -run 'TestCollector_(HostNICPerformanceOptInEmitsCharts|VMNICPerformanceOptInEmitsCharts|VMDiskPerformanceOptInEmitsCharts|CollectsVMDiskCapacity)' -count=1` passed from `src/go/` after fixing the high-cardinality chart lookup tests to match by context and labels.
- `go test ./plugin/go.d/collector/vsphere/... -count=1` passed from `src/go/` after the final topology/readiness/network discovery updates.
- `go test ./plugin/go.d/collector/vsphere ./plugin/framework/charttpl ./plugin/framework/chartengine ./plugin/go.d/pkg/collecttest -count=1` passed from `src/go/` after the final topology/readiness/network discovery updates.
- `ruby -e 'require "json"; require "yaml"; JSON.parse(File.read("src/go/plugin/go.d/collector/vsphere/config_schema.json")); JSON.parse(File.read("src/go/plugin/go.d/collector/vsphere/testdata/config.json")); YAML.safe_load(File.read("src/go/plugin/go.d/collector/vsphere/testdata/config.yaml"), permitted_classes: [], aliases: true); YAML.safe_load(File.read("src/go/plugin/go.d/collector/vsphere/metadata.yaml"), permitted_classes: [], aliases: true); YAML.safe_load(File.read("src/go/plugin/go.d/collector/vsphere/charts.yaml"), permitted_classes: [], aliases: true); YAML.safe_load(File.read("src/go/plugin/go.d/config/go.d/vsphere.conf"), permitted_classes: [], aliases: true)'` passed from the repo root after the final config/docs updates.
- `python3 integrations/gen_integrations.py && python3 integrations/gen_docs_integrations.py -c go.d.plugin/vsphere` passed from the repo root after the final Function/topology metadata updates.
- `git diff --check` passed from the repo root after the final Function/topology updates.
- `.agents/sow/audit.sh .agents/sow/current/SOW-0015-20260507-vsphere-v2-parity-enrichment.md` passed for this SOW after the final Function/topology updates; the audit still reports pre-existing repo-wide partial-state warnings for `SOW-0012` and legacy non-project skill classification.
- `go test ./plugin/go.d/collector/vsphere/discover ./plugin/go.d/collector/vsphere/scrape -run 'TestDiscoverer_DiscoverDatastoreClustersFailSoft|TestVSANQueryIDsUseConcreteDiscoveredEntities|TestParseVSANEntityMetrics' -count=1` passed from `src/go/` after post-review hardening.
- `go test ./plugin/go.d/collector/vsphere -run 'TestCollector_Init_ReturnsFalseIfInvalidVMDiskConfig|TestCollector_Init_ReturnsFalseIfInvalidUserMetadataLabelConfig|TestUserMetadataPatternMatcherPreservesListItems' -count=1` passed from `src/go/` after post-review selector validation hardening.
- `python3 integrations/gen_integrations.py && python3 integrations/gen_docs_integrations.py -c go.d.plugin/vsphere` passed from the repo root after correcting the topology Function metadata ID and vSAN query-scope docs.
- `go test ./plugin/go.d/collector/vsphere/... -count=1` passed from `src/go/` after post-review hardening.
- `go test -race ./plugin/go.d/collector/vsphere -run 'TestCollector_Collect|TestCollector_Collect_Run|TestCollector_VMDisksSelectorAndCap|TestCollector_ChartTemplateYAML|TestFuncReadiness' -count=1` passed from `src/go/` after post-review hardening.
- `go test ./plugin/agent/jobmgr/funcctl -run 'TestControllerRegisterJobMethods|TestController.*Method' -count=1` passed from `src/go/` after post-review hardening.
- `go test ./tools/functions-validation/... -count=1` passed from `src/go/` after post-review Function metadata hardening.
- `ruby -ryaml -rjson -e 'YAML.load_file("src/go/plugin/go.d/collector/vsphere/metadata.yaml"); JSON.parse(File.read("src/go/plugin/go.d/collector/vsphere/config_schema.json")); JSON.parse(File.read("src/go/plugin/go.d/collector/vsphere/testdata/config.json")); puts "metadata/config parse ok"'` passed from the repo root after post-review docs/schema hardening.
- `git diff --check` passed from the repo root after post-review hardening.
- `.agents/sow/audit.sh` passed from the repo root after post-review hardening and after removing `AGENTS.md` from the PR diff; it still reports existing repo-wide skill-classification warnings, but exits 0.
- `go test ./plugin/go.d/collector/vsphere/... -run 'TestCollector_InitReentrantResetsRuntimeState|TestDiscoverer_DiscoverNetworkTopologyFailSoft|TestDiscoverer_DiscoverCustomAttributeErrorFailSoft|TestDiscoverer_DiscoverTagErrorFailSoft|TestFuncTopology_HandleWithEmptyInventoryCache|TestFuncReadiness_HandlePartialInitState' -count=1` passed from `src/go/` after Round 3 reviewer hardening.
- `go test ./plugin/go.d/collector/vsphere/... -count=1` passed from `src/go/` after Round 3 reviewer hardening.
- `go test -race ./plugin/go.d/collector/vsphere -run 'TestCollector_Collect|TestCollector_Collect_Run|TestCollector_InitReentrantResetsRuntimeState|TestCollector_VMDisksSelectorAndCap|TestCollector_ChartTemplateYAML|TestFuncReadiness|TestFuncTopology' -count=1` passed from `src/go/` after Round 3 reviewer hardening.
- `go test ./plugin/agent/jobmgr/funcctl -run 'TestControllerRegisterJobMethods|TestController.*Method' -count=1` passed from `src/go/` after Round 3 reviewer hardening.
- `go test ./tools/functions-validation/... -count=1` passed from `src/go/` after Round 3 reviewer hardening.
- `git diff --check` passed from the repo root after Round 3 reviewer hardening.
- `.agents/sow/audit.sh .agents/sow/current/SOW-0015-20260507-vsphere-v2-parity-enrichment.md` passed from the repo root after Round 3 reviewer hardening; it still reports existing repo-wide skill-classification warnings, but exits 0.
- `python3 integrations/gen_integrations.py && python3 integrations/gen_docs_integrations.py -c go.d.plugin/vsphere` passed from the repo root after correcting the `autodetection_retry` metadata default.
- `go test ./plugin/go.d/collector/vsphere/... -count=1` passed from `src/go/` after Round 4 reviewer hardening.
- `ruby -ryaml -rjson -e 'YAML.load_file("src/go/plugin/go.d/collector/vsphere/metadata.yaml"); YAML.load_file("src/go/plugin/go.d/config/go.d/vsphere.conf"); JSON.parse(File.read("src/go/plugin/go.d/collector/vsphere/config_schema.json")); puts "metadata/config parse ok"'` passed from the repo root after Round 4 reviewer hardening.
- `go test -race ./plugin/go.d/collector/vsphere -run 'TestCollector_Collect|TestCollector_Collect_Run|TestCollector_InitReentrantResetsRuntimeState|TestCollector_VMDisksSelectorAndCap|TestCollector_ChartTemplateYAML|TestFuncReadiness|TestFuncTopology' -count=1` passed from `src/go/` after Round 4 reviewer hardening.
- `go test ./plugin/agent/jobmgr/funcctl -run 'TestControllerRegisterJobMethods|TestController.*Method' -count=1` passed from `src/go/` after Round 4 reviewer hardening.
- `go test ./tools/functions-validation/... -count=1` passed from `src/go/` after Round 4 reviewer hardening.
- `git diff --check` passed from the repo root after Round 4 reviewer hardening.
- `.agents/sow/audit.sh .agents/sow/current/SOW-0015-20260507-vsphere-v2-parity-enrichment.md` passed from the repo root after Round 4 reviewer hardening; it still reports existing repo-wide skill-classification warnings, but exits 0.
- `go test ./plugin/go.d/collector/vsphere/client -run 'TestClient_Close|TestNew_LogsOutOnContainerViewFailure' -count=1` passed from `src/go/` after Round 5 reviewer hardening.
- `go test ./plugin/go.d/collector/vsphere -run 'TestCollector_CollectVSANUsesSelectorsAndCaps|TestCollector_ConfigurationSerialize|TestFuncReadiness_HandleWithVSANCachedData' -count=1` passed from `src/go/` after Round 5 reviewer hardening.
- `go test ./plugin/go.d/collector/vsphere/discover -run 'TestDiscoverer_collectMetricLists' -count=1` passed from `src/go/` after Round 5 reviewer hardening.
- `ruby -rjson -ryaml -e 'JSON.parse(File.read("src/go/plugin/go.d/collector/vsphere/config_schema.json")); YAML.load_file("src/go/plugin/go.d/collector/vsphere/metadata.yaml"); YAML.load_file("src/go/plugin/go.d/config/go.d/vsphere.conf"); puts "parse ok"'` passed from the repo root after Round 5 reviewer hardening.
- `python3 integrations/gen_integrations.py && python3 integrations/gen_docs_integrations.py -c go.d.plugin/vsphere` passed from the repo root after Round 5 reviewer hardening.
- `go test ./plugin/go.d/collector/vsphere/... -count=1` passed from `src/go/` after Round 5 reviewer hardening.
- `go test ./plugin/agent/jobmgr/funcctl -run 'TestControllerRegisterJobMethods|TestController.*Method' -count=1` passed from `src/go/` after Round 5 reviewer hardening.
- `go test ./tools/functions-validation/... -count=1` passed from `src/go/` after Round 5 reviewer hardening.
- `go test -race ./plugin/go.d/collector/vsphere -run 'TestCollector_Collect|TestCollector_Collect_Run|TestCollector_InitReentrantResetsRuntimeState|TestCollector_CollectVSANUsesSelectorsAndCaps|TestCollector_ChartTemplateYAML|TestFuncReadiness|TestFuncTopology' -count=1` passed from `src/go/` after Round 5 reviewer hardening.
- `git diff --check` passed from the repo root after Round 5 reviewer hardening.
- `go test ./collector/vsphere/scrape -run 'TestScraper_calcMaxQuery|TestScraper_ScrapeHosts|TestScraper_ScrapeVMs' -count=1` passed from `src/go/plugin/go.d` after Round 6 reviewer hardening.
- `go test ./collector/vsphere/client -run 'TestClient_Close|TestNew_LogsOutOnContainerViewFailure' -count=1` passed from `src/go/plugin/go.d` after Round 6 reviewer hardening.
- `go test ./collector/vsphere -run 'TestCollector_Collect_HostNoPerfData|TestCollector_Collect_VMNoPerfData|TestCollector_ConfigurationSerialize|TestCollector_V1CompatibilityManifest|TestCollector_V2CompatibilitySurface' -count=1` passed from `src/go/plugin/go.d` after Round 6 reviewer hardening.
- `go test ./collector/vsphere/... -count=1` passed from `src/go/plugin/go.d` after Round 6 reviewer hardening.
- `python3 integrations/gen_integrations.py && python3 integrations/gen_docs_integrations.py -c go.d.plugin/vsphere` passed from the repo root after Round 6 reviewer hardening.
- `go test -race ./collector/vsphere -run 'TestCollector_Collect|TestCollector_Collect_Run|TestCollector_Collect_HostNoPerfData|TestCollector_Collect_VMNoPerfData|TestCollector_InitReentrantResetsRuntimeState|TestCollector_CollectVSANUsesSelectorsAndCaps|TestCollector_ChartTemplateYAML|TestFuncReadiness|TestFuncTopology' -count=1` passed from `src/go/plugin/go.d` after Round 6 reviewer hardening.
- `ruby -rjson -ryaml -e 'JSON.parse(File.read("src/go/plugin/go.d/collector/vsphere/config_schema.json")); YAML.load_file("src/go/plugin/go.d/collector/vsphere/metadata.yaml"); YAML.load_file("src/go/plugin/go.d/config/go.d/vsphere.conf"); puts "parse ok"'` passed from the repo root after Round 6 reviewer hardening.
- `.agents/sow/audit.sh .agents/sow/current/SOW-0015-20260507-vsphere-v2-parity-enrichment.md` passed from the repo root after Round 6 reviewer hardening; it still reports existing repo-wide skill-classification warnings, but exits 0.
- `git diff --check` passed from the repo root after Round 6 reviewer hardening.
- `go test ./collector/vsphere -run 'TestUpdateResourcePoolFromProperties_ZeroValueOptionalProperties|TestCollector_Collect_HostNoPerfData|TestCollector_Collect_VMNoPerfData|TestCollector_ConfigurationSerialize' -count=1` passed from `src/go/plugin/go.d` after Round 7 cleanup.
- `go test ./collector/vsphere/client -run 'TestClient_Close|TestNew_LogsOutOnContainerViewFailure' -count=1` passed from `src/go/plugin/go.d` after Round 7 cleanup.
- `go test ./collector/vsphere/scrape -run 'TestScraper_calcMaxQuery' -count=1` passed from `src/go/plugin/go.d` after Round 7 cleanup.
- `go test ./collector/vsphere/... -count=1` passed from `src/go/plugin/go.d` after Round 7 cleanup.
- `go test -race ./collector/vsphere -run 'TestCollector_Collect|TestCollector_Collect_Run|TestUpdateResourcePoolFromProperties_ZeroValueOptionalProperties|TestCollector_InitReentrantResetsRuntimeState|TestCollector_CollectVSANUsesSelectorsAndCaps|TestCollector_ChartTemplateYAML|TestFuncReadiness|TestFuncTopology' -count=1` passed from `src/go/plugin/go.d` after Round 7 cleanup.
- `python3 integrations/gen_integrations.py && python3 integrations/gen_docs_integrations.py -c go.d.plugin/vsphere` passed from the repo root after Round 7 cleanup.
- `ruby -rjson -ryaml -e 'JSON.parse(File.read("src/go/plugin/go.d/collector/vsphere/config_schema.json")); YAML.load_file("src/go/plugin/go.d/collector/vsphere/metadata.yaml"); YAML.load_file("src/go/plugin/go.d/config/go.d/vsphere.conf"); puts "parse ok"'` passed from the repo root after Round 7 cleanup.
- `.agents/sow/audit.sh .agents/sow/current/SOW-0015-20260507-vsphere-v2-parity-enrichment.md` passed from the repo root after Round 7 cleanup; it still reports existing repo-wide skill-classification warnings, but exits 0.
- `git diff --check` passed from the repo root after Round 7 cleanup.

Real-use evidence:

- The vSphere package tests run against `govmomi/simulator` (`vcsim`) through `prepareVSphereSim`, which exercises discovery, scraping, chart creation, datastore/cluster/resource-pool paths, and the V2 metric-store bridge without production vCenter access.
- No production vCenter data was used or stored.

Reviewer findings:

- External review rounds were requested and run read-only against this SOW and the branch diff. Earlier rounds returned mixed `NEEDS CHANGES` / `PRODUCTION READY` votes. The material findings handled in this SOW are datastore-cluster fail-soft behavior, pure-negative selector validation, topology Function metadata ID drift, stale `AGENTS.md` artifact notes, vSAN API-scope controls, re-entrant `Init()` runtime reset, optional network topology fail-soft behavior, enrichment API fail-soft test coverage, partial-init Function test coverage, vSphere client cleanup, vnode GUID URL fallback removal, datastore-cluster enrichment label emission, malformed parent/depth/cycle guards, NIC `instance:` selector aliases, `autodetection_retry` docs/default alignment, REST/vAPI session cleanup, partial-login cleanup, dedicated vSAN selectors/caps, metadata health-alert name alignment, missing-performance-counter warnings, vCenter 7+/8+ performance query batching, fail-soft empty host/VM performance cycles, DynCfg selector required-list cleanup, and lazy vAPI/vSAN client hardening. Round 6 returned six clean approvals plus one `NEEDS CHANGES`; the remaining actionable Round 6 findings were fixed, and the storage-adapter counter spelling finding was rejected with VMware API evidence. Round 7 returned production-ready votes from Codex, Claude, Qwen, MiMo, Kimi, and MiniMax. GLM's single `NEEDS CHANGES` finding was rejected with govmomi type evidence because `mo.ResourcePool.Runtime` is a value struct, not a pointer; Round 7 cleanup added a proof test/comment and handled low-risk cleanup notes. Round 8 returned production-ready votes from Codex, Claude, GLM, Kimi, MiMo, and MiniMax; the first Qwen Round 8 process timed out with no output, and the same-scope Qwen rerun returned `PRODUCTION READY`. The SOW and implementation are considered production-ready together, with residual non-blocking risks limited to no live production vCenter/vSAN validation, accepted chart-ID continuity loss, vCenter/ESXi events excluded by user decision, deeper vSAN internals excluded pending product/NIDL mapping, sensitive device identity labels excluded pending allowlist/cap policy, and readiness remaining cached/local rather than live permission probing.
- A 2026-05-09 reference-implementation review round compared this branch and SOW against mirrored `open-telemetry/opentelemetry-collector-contrib`, `DataDog/integrations-core`, `zabbix/zabbix`, `grafana/vmware_exporter`, `Checkmk/checkmk`, `newrelic/nri-vsphere`, `Appdynamics/vmware-vsphere-monitoring-extension`, `sensu-plugins/sensu-plugins-vsphere`, `logicmonitor/dashboards`, and `influxdata/telegraf`. GLM and MiMo returned `PRODUCTION READY`; Qwen, Claude, and Kimi returned `NEEDS CHANGES`; MiniMax returned an invalid review output that made false claims about hardcoded performance counter IDs even though `discover/metric_lists.go` uses `PerformanceManager.CounterInfoByName()` through the govmomi counter registry.
- Actionable reference-review fixes implemented: add missing `divisor: 100` chart scaling for Datadog-confirmed percentage counters `disk.scsiReservationCnflctsPct.avg` and `storageAdapter.OIOsPct.avg`; tighten DynCfg include-selector schema patterns so empty strings are not accepted where collector validation rejects them; warn that opt-in custom-attribute label values are sent verbatim and may contain secrets; clarify VM disk `disk` label meaning across capacity/performance contexts; reword snapshot age as `oldest snapshot age`; add vSAN parser tests for OTel-supported host/VM label sets and rate handling; add a short resource-pool compressed-memory unit comment; and rate-limit optional discovery/enrichment warnings with stable low-cardinality limiter keys.
- Reference-review findings rejected or accepted as residual risk: vSAN throughput/congestion division by interval is kept because OTel `receiver/vcenterreceiver/metrics.go` divides these values and OTel `client.go` defaults missing vSAN intervals to 300 seconds; vSAN explicit per-entity query specs are kept because the collector enforces selectors/caps before query and avoids fetching all entities; vSAN latest CSV-token timestamp validation remains a live-vSAN residual risk; govmomi URL credential embedding and keepalive re-login logging are pre-existing patterns outside this PR's scope; VM virtual disk capacity fallback remains because govmomi documents `VirtualDisk.CapacityInBytes` as always populated by the server and the fallback is harmless compatibility handling.
- Round 2 of the same reference-review scope was run after those fixes. Qwen, GLM, and MiniMax returned `PRODUCTION READY`; Claude returned `NEEDS CHANGES`; MiMo and Kimi outputs were invalid because they timed out before producing a verdict. Actionable Round 2 fixes implemented: add datastore-cluster `overallStatus` collection and a `vsphere.datastore_cluster_overall_status` chart; fix the resource-pool compressed-memory comment to document KiB input and MB chart display scale; add collector-level vSAN assertions for throughput, latency, and congestion; add vSAN space-utilization edge-case tests; align vSAN CSV metric values to the latest `SampleInfo` bucket when `SampleInfo` is present; and reset vSAN matchers in re-entrant `Init()`.
- Round 2 findings rejected with evidence: V1+V2 double chart emission is not a runtime risk because `collectorapi.CollectorV2` does not include `Charts()` and `jobruntime/job_v2.go` loads only `ChartTemplateYAML()`; vSAN throughput/congestion interval division is confirmed by OTel `receiver/vcenterreceiver/metrics.go`; vSAN host `clientCacheHitRate` under `host-domclient` is confirmed by OTel `receiver/vcenterreceiver/client.go`; stale datastore values during property-refresh failures are existing fail-soft behavior and should not be changed in this PR without a broader stale-data policy decision; snapshot alert `calc` semantics preserve the explicit user thresholds for age and chain depth.
- Round 3 of the same reference-review scope was run after the Round 2 fixes. Qwen and GLM returned `PRODUCTION READY`; Claude returned `NEEDS CHANGES` primarily because that reviewer could not access mirrored references and also listed low-severity observations; Kimi returned `NEEDS CHANGES`; MiniMax and MiMo timed out with invalid/incomplete outputs. Actionable Round 3 fixes implemented: correct the resource-pool compressed-memory comments to say vSphere reports KiB and the chart keeps V1's MB display scale, change the resource struct comment to `KiB`, and preserve empty vSAN health responses in the `Health` map so the existing writer emits `unknown=1` instead of no health series.
- Round 3 findings rejected or accepted as residual risk: vSAN host cache-hit-rate should not get `divisor: 100` because OTel records raw values such as `82` and `88` with unit `%`, while Datadog divides by 100 to convert its percentage metrics to a 0-1 ratio; vSAN sample alignment intentionally skips stale per-series values when the latest `SampleInfo` bucket is empty so Netdata emits gaps instead of mixing timestamps; vSAN latency units remain based on OTel metadata declaring microseconds and are recorded as a real-vSAN residual validation gap; datastore label refresh and per-cycle datastore property refresh are pre-existing low-risk lifecycle/scale behaviors outside this PR's compatibility-safe scope.

Same-failure scan:

- V2 migration same-failure scan covered local V2 collectors (`powervault`, `powerstore`, `ping`, `mysql`, `azure_monitor`) for `CreateV2`, `MetricStore`, `ChartTemplateYAML`, cycle-managed tests, and chart-template validation patterns.
- Initial same-domain scan covered mirrored vSphere monitoring implementations listed above.

Sensitive data gate:

- This SOW contains no raw secrets, credentials, bearer tokens, SNMP communities, customer names, personal data, non-private customer-identifying IPs, private endpoints, real vCenter URLs, real VM names, real hostnames, real datastore names, or raw production output.

Artifact maintenance gate:

- AGENTS.md: unchanged in the final PR diff; the new framework V2 skill remains available at `.agents/skills/project-writing-go-modules-framework-v2/` without changing repo-wide instructions.
- Runtime project skills: added `.agents/skills/project-writing-go-modules-framework-v2/SKILL.md` because framework V2 collector style is reusable HOW-to-work-here knowledge for the vSphere migration and future V2 collectors; compressed it to keep only actionable guidance plus source paths. Updated `.agents/skills/integrations-lifecycle/gotchas.md` with the scoped-docs `blob/master` metadata-link normalization gotcha found while regenerating the vSphere README.
- Legacy runtime skills: reworded one GitHub SSH example in `.agents/skills/mirror-netdata-repos/SKILL.md` to avoid a SOW audit false positive.
- Integration generator: updated `integrations/gen_docs_integrations.py` to normalize both current `blob/master` and older `edit/master` metadata links; this was required for scoped regeneration of the vSphere integration README.
- Follow-up SOWs: the four pending vSphere follow-up SOW files created during the earlier split attempt were deleted after the user decided to keep all remaining non-event vSphere parity work in this SOW/PR. Events remain out of this PR by user decision, not by a pending SOW.
- Collector tests/fixtures: added the V1 compatibility golden test and fixture, generated V2 `charts.yaml`, chart-template validation, V2 compatibility-surface validation, chart coverage validation, snapshot tree traversal tests, power-state config validation tests, power-state discovery tests, non-powered-on property-only collection tests, VM/host property-status tests, perf query skip tests, opt-in inventory/guest label enrichment tests, opt-in vSphere tag/custom-attribute label tests, user-metadata pattern tests for names with spaces, opt-in VM disk capacity/performance tests, wildcard `virtualDisk.*` metric-list tests, opt-in VM network-interface performance tests, wildcard VM `net.*` metric-list tests, aggregate VM network compatibility coverage, opt-in host physical network-interface performance tests, wildcard host `net.*` metric-list tests, aggregate host network compatibility coverage, opt-in host disk/LUN/device performance tests, wildcard host `disk.*` metric-list tests, aggregate host disk compatibility coverage, opt-in host storage-adapter performance tests, wildcard host `storageAdapter.*` metric-list tests, aggregate storage-adapter maximum-latency coverage, opt-in host storage-path performance tests, wildcard `storagePath.*` metric-list tests, aggregate storage-path maximum-latency coverage, opt-in host CPU-instance performance tests, wildcard host `cpu.*` metric-list tests, opt-in host/VM power metric tests, optional `power.*` metric-list tests, opt-in datastore cluster tests, read-only readiness Function tests, cached topology Function tests, opt-in Network/DVPG topology discovery tests, re-entrant `Init()` reset tests, optional network/topology fail-soft tests, enrichment API fail-soft tests, partial-init Function tests, and context-aware high-cardinality chart lookup tests.
- Optional vnode tests/fixtures: removed after the 2026-05-20 hard-removal decision. Config serialization fixtures no longer include `esxi_vnodes` or `vm_vnodes`.
- Specs: added `.agents/sow/specs/vsphere-v1-compatibility-manifest.md` and `.agents/sow/specs/vsphere-parity-matrix.md`; updated the compatibility manifest to record the accepted chart-ID break, additive V2 `id` instance label, power-state config keys, property-only non-powered-on semantics, fail-soft empty host/VM performance cycles, selector keys being optional in DynCfg schema, additive cluster DRS/HA property-status charts, static inventory-count chart, opt-in inventory/VM guest label keys, opt-in vSphere tag/custom-attribute label keys/config, opt-in VM disk capacity/performance, opt-in VM network-interface performance, opt-in host physical network-interface performance, opt-in host disk/LUN/device performance, opt-in host storage-adapter performance, opt-in host storage-path performance, opt-in host CPU-instance performance, opt-in host/VM power metrics, opt-in datastore cluster contexts/config, `collect_network_topology`, and the cached Function contract; updated parity rows P01/P02/P06/P07/P08/P09/P10/P11/P12/P13/P14/P15/P16/P17/P18/P19/P20/P21/P22/P28/P29/P30/P31/P32 to implemented or implemented-subset status.
- End-user/operator docs: updated `README.md` and `metadata.yaml` for new snapshot metrics, snapshot alerts, datastore aggregate metrics, VM/host power-state metrics, VM/host property-status metrics, cluster DRS/HA property-status metrics, inventory-count metrics, opt-in VM disk capacity/performance metrics, opt-in VM network-interface performance metrics, opt-in host physical network-interface performance metrics, opt-in host disk/LUN/device performance metrics, opt-in host storage-adapter performance metrics, opt-in host storage-path performance metrics, opt-in host CPU-instance performance metrics, opt-in host/VM power metrics, opt-in datastore cluster metrics, opt-in inventory/VM guest labels, opt-in vSphere tag/custom-attribute labels, readiness/topology Functions, `collect_network_topology`, power-state discovery controls, zero no-snapshot semantics, inaccessible datastore capacity semantics, and the `id` label; expanded the stock `go.d/vsphere.conf` comments so file-based configuration is self-explanatory. Chart-ID continuity loss remains accepted and not emphasized in end-user docs.
- End-user/operator skills: no update needed for this internal collector-framework migration slice.
- SOW lifecycle: moved to `.agents/sow/current/` after user approved proceeding; the previous follow-up split was consolidated back into this SOW by user decision; no pending vSphere follow-up SOW files remain. At close, this SOW is marked completed and moved to `.agents/sow/done/` with the implementation commit.

Specs update:

- Added `.agents/sow/specs/vsphere-v1-compatibility-manifest.md` as the current v1 compatibility contract for the migration, including the accepted chart-ID break, V2 `id` label, additive default-safe metrics, optional inventory/guest label enrichment, optional vSphere tag/custom-attribute label enrichment, optional VM disk capacity/performance, optional VM network-interface performance, optional host physical network-interface performance, optional host disk/LUN/device performance, optional host storage-adapter performance, optional host storage-path performance, opt-in host CPU-instance performance, optional datastore cluster metric contracts, `collect_network_topology`, and cached Function contracts.
- Added `.agents/sow/specs/vsphere-parity-matrix.md` as the current parity/default-policy contract for implementation planning, including implemented rows P06-P22/P30-P32, implemented-subset row P29, and explicitly excluded event row P27.

Project skills update:

- Added `.agents/skills/project-writing-go-modules-framework-v2/SKILL.md` to preserve concise Go framework V2 collector patterns from accepted local collectors.

End-user/operator docs update:

- Snapshot, datastore aggregate, power-state, VM/host property-status, cluster DRS/HA property-status, inventory-count, optional VM disk capacity/performance, optional VM network-interface performance, optional host physical network-interface performance, optional host disk/LUN/device performance, optional host storage-adapter performance, optional host storage-path performance, optional host CPU-instance performance, optional host/VM power metrics, optional datastore cluster, optional inventory path label, optional VM guest label, optional vSphere tag/custom-attribute label, readiness/topology Function, and optional Network/DVPG topology docs are now updated in `README.md` and `metadata.yaml`; stock `go.d/vsphere.conf` now documents selectors, secrets, vnode behavior, all valid host/VM power states, opt-in labels, opt-in high-cardinality groups, opt-in power metrics, optional network topology discovery, and safe discovery defaults inline.

End-user/operator skills update:

- No end-user/operator skills update needed for the V2 migration or snapshot metrics; there is no public AI-skill workflow for operating this collector affected by these changes.

Lessons:

- vSphere parity is not a single metric-list task. Competitor coverage combines metrics, properties, events, topology, hardware sensors, tags/custom attributes, and high-cardinality per-instance counters, so default safety policy is the central product decision.

Follow-up mapping:

- Product decisions are tracked in `## Implications And Decisions`.
- Opt-in parity rows P13-P22, P26, P28, and P29 are now tracked inside this SOW.
- Topology, network/distributed-port-group state, and troubleshooting rows P30-P32 were implemented inside this SOW as non-metric surfaces.
- Collector-generated ESXi/VM vnode work is excluded from this PR by the 2026-05-20 user decision; datastore-vnode exclusion row P35 remains tracked inside this SOW.
- vCenter/ESXi event parity row P27 is explicitly out of this PR by user decision on 2026-05-08.
- Covered-elsewhere rows P23-P25 and P36 are documented or already represented by existing Netdata collectors; no new implementation SOW is needed for them.

## Outcome

Implementation is completed and production-ready for this SOW/PR scope. Framework V2 migration, compatibility manifest, default-safe metrics, opt-in high-cardinality metrics, opt-in labels, opt-in datastore/vSAN additions, read-only readiness Function, cached topology Function, and opt-in Network/DVPG topology discovery are implemented, locally validated, and reviewed. Collector-generated ESXi/VM vnodes were hard-removed before merge by user decision.

Known unresolved parity gaps are not blocked by code mechanics:

- P27 vCenter/ESXi events are excluded from this PR by user decision.
- Deeper vSAN disk-group/disk/component/CMMDS coverage has open-source implementation evidence, but needs an explicit NIDL/context mapping and bounded config policy before implementation because the Telegraf-style entity list is very broad and dynamic.
- MAC/IQN/WWN/device identity labels need an explicit allowlist/cap policy before they can be exposed safely.
- Readiness does not perform live permission probes; it reports cached/local collector state only.

## Lessons Extracted

- vSphere parity must be split by commit and product surface even when it is one SOW/PR. Default-safe metrics, opt-in high-cardinality metrics, events/logs, topology, and vnodes have different compatibility and review risks.

## Followup

No new SOW file is needed for the implemented scope. P27 events are excluded from this PR by user decision. Deeper vSAN internals, sensitive device identity labels, and live readiness probes are excluded from this PR rather than deferred implementation items; each requires a new user-approved product/NIDL decision and a separate SOW if pursued later.

## Regression Log

### Regression - 2026-05-09

What broke:

- PR #22458 CI reported failing `yamllint`, Codacy, and SonarCloud checks after the SOW was marked completed.
- Confirmed yamllint failure is in this PR: `src/go/plugin/go.d/collector/vsphere/charts.yaml` uses YAML sequence indentation accepted by the parser but rejected by repo yamllint.
- Confirmed SonarCloud failures are in this PR: `funcTopology.Cleanup` and `funcReadiness.Cleanup` had empty bodies without explanatory comments.
- Codacy check has no GitHub annotations for this run. Local anonymous `codacy-analysis-cli` Docker execution first failed before producing valid JSON because several Codacy tool containers could not read their generated `/.codacyrc` configuration. The public Codacy v3 PR-issues endpoint was later used to identify the two still-blocking generated-Markdown issues without a configured `CODACY_TOKEN`.
- Additional pre-merge hardening requested by the user: verify NIDL compatibility, DynCfg schema coverage/tab size, metadata coverage, change-size justification, real-vSphere risk/test coverage, and supportable error messages.

Evidence:

- `gh pr checks 22458 --repo netdata/netdata --watch=false` reported `yamllint`, `Codacy Static Code Analysis`, and `SonarCloud Code Analysis` as failed or action-required.
- `gh run view 25590676851 --repo netdata/netdata --job 75127727127 --log-failed` showed `charts.yaml` indentation errors, starting at `src/go/plugin/go.d/collector/vsphere/charts.yaml:4`.
- `gh api repos/netdata/netdata/check-runs/75127802896/annotations --paginate` returned SonarCloud annotations for `src/go/plugin/go.d/collector/vsphere/func_topology.go:70` and `src/go/plugin/go.d/collector/vsphere/func_readiness.go:207`.
- `gh api repos/netdata/netdata/check-runs/75127799386/annotations --paginate` returned no Codacy annotations for the `Codacy Static Code Analysis` check run.
- `.agents/skills/codacy-audit/scripts/pr-issues.sh 22458` could not run because `.env`/`CODACY_TOKEN` is not configured in this worktree.
- `.agents/skills/codacy-audit/scripts/analyze-local.sh` wrote `.local/audits/codacy/local-20260509T034451Z.json`, but that file is not JSON; it contains Codacy tool-container failures such as `read /.codacyrc: is a directory`, so it cannot be used as finding evidence.
- `curl -fsS https://api.codacy.com/api/v3/analysis/organizations/gh/netdata/repositories/netdata/pull-requests/22458/issues?limit=100` returned Codacy PR issue details; the only `deltaType=Added` issues were two `markdownlint_MD013` long-line findings in the generated vSphere integration overview.

Why previous validation missed it:

- Local validation parsed `charts.yaml` as YAML and ran chart-template tests, but did not run repo yamllint.
- Static-analysis gates were checked only through external reviewers and local targeted tests, not through Codacy/SonarCloud PR APIs after the PR was opened.

Repair plan:

- Make generated `charts.yaml` yamllint-compliant without weakening chart-template validation.
- Add explanatory comments to intentionally empty Function cleanup methods.
- Fetch and triage Codacy findings through `.agents/skills/codacy-audit/` when a token is available, or through the public v3 PR-issues endpoint when GitHub annotations are empty and no token is configured.
- Fetch and triage SonarCloud findings through GitHub annotations and `.agents/skills/sonarqube-audit/` if the PR API evidence is insufficient.
- Split DynCfg host options into smaller short-title tabs.
- Verify `metadata.yaml` and `charts.yaml` context/unit/dimension parity.
- Add contextual wrapping to vSphere client/discovery/scrape error paths so customer reports identify the failed operation, config key, inventory resource type, path set, cluster/ref count, or API stage.
- Fix confirmed PR-owned findings, reject only tool false positives with evidence, and update this regression section with validation.

Validation plan:

- Run yamllint locally against changed YAML surfaces or the same command class used by CI.
- Run the vSphere chart-template/generation tests and full vSphere package tests after any generated `charts.yaml` change.
- Run Codacy and SonarCloud triage commands or local analyzer equivalents where available.
- Re-run PR CI after pushing the fix.

Local validation completed as of 2026-05-09:

- `UPDATE_VSPHERE_CHARTS=1 go test ./collector/vsphere -run TestCollector_ChartTemplateYAML -count=1` passed from `src/go/plugin/go.d`.
- `go test ./collector/vsphere/... -count=1` passed from `src/go/plugin/go.d` after error-context and schema hardening.
- `go test ./collector/vsphere/... -count=1` passed from `src/go/plugin/go.d` after the SonarCloud duplication refactor.
- `go test ./collector/vsphere/... -count=1` passed from `src/go/plugin/go.d` after the final supportability error-message hardening.
- `go test ./collector/vsphere/... -count=1` passed from `src/go/plugin/go.d` after tightening remaining loose warning/config-validation messages for supportability.
- `go test ./collector/vsphere/... -count=1` passed from `src/go/plugin/go.d` after hot-path repeated-log rate limiting.
- `UPDATE_VSPHERE_CHARTS=1 go test ./collector/vsphere -run TestCollector_ChartTemplateYAML -count=1` passed from `src/go/plugin/go.d` after adding the reference-review percentage-scaling fixes.
- `UPDATE_VSPHERE_COMPAT=1 go test ./collector/vsphere -run TestCollector_V1CompatibilityManifest -count=1` passed from `src/go/plugin/go.d` after rewording snapshot-age titles and regenerating the compatibility fixture.
- `go test ./collector/vsphere/... -count=1` passed from `src/go/plugin/go.d` after the reference-implementation review fixes.
- `UPDATE_VSPHERE_CHARTS=1 go test ./collector/vsphere -run TestCollector_ChartTemplateYAML -count=1` passed from `src/go/plugin/go.d` after adding datastore-cluster overall-status charts.
- `UPDATE_VSPHERE_COMPAT=1 go test ./collector/vsphere -run TestCollector_V1CompatibilityManifest -count=1` passed from `src/go/plugin/go.d` after datastore-cluster and vSAN Round 2 hardening.
- `go test ./collector/vsphere ./collector/vsphere/discover ./collector/vsphere/scrape -run 'TestCollector_(DatastoreClusters|VSAN)|TestNewStoragePod|TestParseVSANEntityMetrics' -count=1` passed from `src/go/plugin/go.d` after Round 2 reference-review hardening.
- `go test ./collector/vsphere/... -count=1` passed from `src/go/plugin/go.d` after Round 2 reference-review hardening.
- `go test ./collector/vsphere ./collector/vsphere/scrape -run 'TestCollector_VSAN|TestScraper_ScrapeVSANRecordsEmptyHealthAsUnknown|TestParseVSANEntityMetrics' -count=1` passed from `src/go/plugin/go.d` after Round 3 reference-review hardening.
- `go test ./collector/vsphere/... -count=1` passed from `src/go/plugin/go.d` after Round 3 reference-review hardening.
- `go test ./tools/functions-validation/... -count=1` passed from `src/go`.
- `yamllint src/go/plugin/go.d/collector/vsphere/charts.yaml src/go/plugin/go.d/collector/vsphere/metadata.yaml src/go/plugin/go.d/collector/vsphere/testdata/config.yaml` passed from the repo root.
- `jq empty src/go/plugin/go.d/collector/vsphere/config_schema.json src/go/plugin/go.d/collector/vsphere/testdata/config.json` passed from the repo root.
- `ruby -rjson -ryaml -e 'JSON.parse(...); YAML.load_file(...); puts "parse ok"'` passed for `config_schema.json`, `metadata.yaml`, `charts.yaml`, and stock `go.d/vsphere.conf`.
- `python3 integrations/gen_docs_integrations.py -c go.d.plugin/vsphere` passed and produced no generated integration-doc diff.
- `git diff --check` passed.
- `.agents/sow/audit.sh` passed with only the existing skill-classification warning already documented by the repo SOW bootstrap.

Artifact updates needed:

- SOW lifecycle: this SOW is reopened in `.agents/sow/current/` until PR CI findings are fixed or proven unrelated.
- Specs, project skills, end-user docs, and operator skills: update only if the fix changes durable behavior or reveals a reusable process gotcha.

Implemented regression fixes as of 2026-05-09:

- `chart_template_test.go` now uses `gopkg.in/yaml.v3` encoder indentation so generated `charts.yaml` matches repo yamllint sequence indentation.
- `charts.yaml` was regenerated with `UPDATE_VSPHERE_CHARTS=1 go test ./collector/vsphere -run TestCollector_ChartTemplateYAML -count=1`.
- `funcTopology.Cleanup` and `funcReadiness.Cleanup` now contain comments explaining that no per-invocation resources are allocated.
- `config_schema.json` tabs were split into short logical tabs: `Base`, `Filters`, `Vnodes`, `Labels`, `VMs`, `H-Net`, `H-Disk`, `H-Store`, `H-CPU`, `Power`, `Stores`, `vSAN`, `Topology`, `Auth`, `TLS`, `Proxy`, `Headers`.
- `metadata.yaml` trailing whitespace was removed after local yamllint reported it.
- vSphere errors were wrapped in `client`, `discover`, `scrape`, and collector init/collection paths with operation-specific context. Examples now include config option names, SOAP/vim25/login/container-view stages, inventory resource types and property path sets, tag/vSAN API stages, and resource/ref counts.
- Final supportability hardening tightened remaining generic paths: required config errors now name `url`, `username`, and `password`; periodic discovery logs say `periodic vSphere discovery failed`; collection errors are wrapped as `collect vSphere metrics`; include-filter parser errors identify the config surface, bad value, expected path shape, segment index, and segment value; performance scrape failures include query-spec counts and representative managed-object references; empty host/VM performance cycles report powered-on and discovered resource counts.
- Additional supportability hardening changed remaining loose config validation and property-refresh warnings to identify the config option, enabling option, vSphere collection phase, resource type, ref count, and property `pathSet`.
- Hot-path recoverable vSphere warnings/errors now use the existing Go logger `Limit(key, 1, time.Hour)` pattern with stable low-cardinality keys for no host/VM performance samples, datastore/cluster/resource-pool property refresh failures, periodic discovery failure, and performance query chunk failures. Focused tests assert repeated host/VM no-sample warnings and scrape errors emit once per limiter window.
- `.agents/skills/project-writing-go-modules-framework-v2/` was updated to document the maintainer-friendly V2 hot-path logging pattern: use `Limit()` for cross-cycle spam protection, keep limiter keys stable and low-cardinality, and do not rely on `Once()` because V2 resets it after each collection cycle.
- `.agents/skills/codacy-audit/` was updated with a concise gotcha/how-to for malformed local Codacy `.json` dumps that contain tool-runner logs, because this issue was discovered while triaging the PR CI failure.
- `.agents/skills/codacy-audit/` was updated with a no-token Codacy `action_required` triage how-to after the public v3 endpoint exposed the two blocking `markdownlint_MD013` generated-doc findings.
- The vSphere metadata overview was split into shorter generated paragraphs, and `integrations/gen_integrations.py` plus `integrations/gen_docs_integrations.py -c go.d.plugin/vsphere` regenerated the vSphere integration page.
- After that push, Codacy reported one remaining generated-doc `markdownlint_MD013` finding on the `autodetection_retry` config table row; the metadata table description was shortened while the fuller disable semantics remain documented in the stock config/schema surfaces.
- After pushing the first regression repair, SonarCloud reported no bugs, no vulnerabilities, and no code smells, but failed the quality gate on `new_duplicated_lines_density=4.8` against the `3` threshold. The largest duplicate contributors were repeated selector/cap test tables and the topology presentation literal.
- Selector/cap test cases were consolidated into shared test helpers, and topology presentation actor summary-field construction was factored through small local helpers. This preserves test intent while reducing copy-pasted blocks.
- Reference-implementation review found two host percentage counters missing chart divisors. `host_disk_device_scsi_reservation_conflicts_percentage` and `host_storage_adapter_outstanding_io_percentage` now use `divisor: 100` in the V2 chart template, regenerated `charts.yaml`, and regenerated the compatibility fixture.
- Reference-implementation review found DynCfg accepted empty include selectors that collector validation rejects. The four top-level include selector schema patterns now require `/`-prefixed inventory paths, matching `Init()` behavior.
- Reference-implementation review found custom-attribute label documentation did not explicitly warn that values are sent verbatim. `config_schema.json`, `metadata.yaml`, and stock `go.d/vsphere.conf` now warn users not to enable patterns that may match secrets.
- Reference-implementation review found optional discovery and user-metadata enrichment warning paths that could repeat every discovery cycle. These warnings now use the existing `Limit(key, 1, time.Hour)` pattern with stable low-cardinality keys.
- Reference-implementation review clarity fixes changed snapshot age wording to `oldest snapshot age`, clarified the VM disk `disk` label across capacity/performance charts, added a resource-pool compressed-memory unit comment, and expanded vSAN parser tests for OTel-compatible host/VM label sets and rate normalization.
- Round 2 reference review found datastore-cluster `overallStatus` was missing from the opt-in StoragePod surface. Discovery now requests `overallStatus`, `StoragePod` resources store it, `writeDatastoreClusterMetrics()` emits green/red/yellow/gray status metrics, generated `charts.yaml` has `vsphere.datastore_cluster_overall_status`, and `metadata.yaml` documents the context.
- Round 2 reference review found vSAN collector-level tests did not assert throughput/latency/congestion emission and vSAN space-utilization edge cases. The tests now assert cluster/host/VM throughput and latency metrics, host/cluster congestion metrics, and zero/over-free space handling.
- Round 2 reference review found vSAN CSV parsing could mix values from different sample buckets when `SampleInfo` is present. The parser now aligns values to the latest `SampleInfo` bucket, while retaining the previous latest-non-empty fallback when no `SampleInfo` is returned.
- Round 2 reference review found re-entrant `Init()` did not clear vSAN selector matchers. `resetRuntimeStateForInit()` now clears `vsanClusterMatcher`, `vsanHostMatcher`, and `vsanVMMatcher`.
- Round 2/3 reference review found the resource-pool compressed-memory comments were wrong. The comments now say vSphere reports KiB and the chart keeps the V1 MB display scale.
- Round 3 reference review found empty vSAN health responses produced no health series even though the writer already treats non-green/yellow/red as unknown. Empty health responses are now preserved and covered by `TestScraper_ScrapeVSANRecordsEmptyHealthAsUnknown`.

NIDL/config/metadata verification as of 2026-05-09:

- `docs/NIDL-Framework.md:116` through `docs/NIDL-Framework.md:170` require one instance type per context, related dimensions with one unit, hierarchy separation by contexts, and meaningful labels. The generated template has 161 contexts, no detected context/unit mismatch, and contexts are separated by vSphere resource type or child-resource type.
- `docs/NIDL-Framework.md:301` through `docs/NIDL-Framework.md:304` recommends no family with both charts and subfamilies and 3+ charts per leaf. The generated template has zero families with both charts and subfamilies. It still has singleton/two-chart families inherited from the existing vSphere dashboard grouping; changing those families would be a dashboard-layout decision, not a metric correctness fix.
- `src/go/BEST-PRACTICES.md:364` through `src/go/BEST-PRACTICES.md:367` were re-read; chart-template validation uses the Go V2 chart-template compiler as required by framework V2.
- Schema coverage check found 51 explicit collector JSON fields and 68 schema properties; no explicit collector config field is missing from `config_schema.json`. The 17 extra schema fields are embedded framework HTTP/TLS/proxy/auth fields.
- UI tab coverage check found no tab field that is not a schema property. The only schema properties not shown in tabs are hidden framework fields: `bearer_token_file`, `body`, `force_http2`, and `method`.
- Metadata coverage check found 68 metadata config options and 68 schema properties with no mismatch.
- Chart/metadata coverage check found 161 generated chart contexts and 161 metadata metric contexts with no missing, extra, unit-mismatched, or dimension-mismatched contexts.

Change-size analysis as of 2026-05-09:

- Diff against `upstream/master...HEAD` before the local regression commit: 77 files, 33,983 insertions, 502 deletions.
- Non-runtime bulk explains most of the size: 10,916 inserted lines are the V1 compatibility fixture, 3,176 are generated `charts.yaml`, 1,109 are generated integration docs, 2,585 are SOW/spec/skill artifacts, and 6,410 are tests.
- Runtime collector Go changes are about 6,929 inserted lines across 35 files before the 2026-05-20 vnode removal. This is large but maps to the approved broad scope: V2 migration, default-safe enrichment, opt-in high-cardinality metrics, labels, vSAN, Functions, topology, and fail-soft supportability.
- Potential reductions exist but have tradeoffs: shrinking the compatibility fixture weakens compatibility proof, dropping generated docs conflicts with the integration artifact workflow, and removing SOW/spec/skill artifacts conflicts with the project SOW process.

Real-vSphere risk and validation assessment as of 2026-05-09:

- Strongest validation: `govmomi/simulator` integration tests exercise login, discovery, property retrieval, performance registry lookup, collection, V2 metric store, chart generation, and Function behavior with official govmomi managed-object types.
- Strongest field-name evidence: vSphere properties use govmomi typed `mo`/`types` structures and official property path strings such as `snapshot`, `summary.runtime.powerState`, `summary.storage`, `summary`, and `overallStatus`. Performance counters are resolved through `CounterInfoByName()`; missing counters warn and skip dependent metrics rather than inventing data.
- Snapshot, datastore, resource-pool, tag/custom-attribute, vSAN parsing, and high-cardinality mapping all have focused unit tests. The V1 compatibility manifest protects metric/context/dimension/label compatibility except for the user-accepted chart-ID break.
- Residual risk remains because there is no live production vCenter/vSAN validation in this worktree. Default-safe existing/property metrics are low risk; opt-in performance counter groups are medium risk due counter availability/version/license differences; opt-in vSAN is medium-high risk because govmomi has typed APIs and parser tests but no live vSAN simulator coverage for every API response shape.
- The 2026-05-09 reference-implementation review increased confidence in field names, units, and scaling by comparing this branch against OTel, Datadog, Zabbix, Grafana vmware_exporter, Checkmk, New Relic, AppDynamics, Sensu, LogicMonitor, and Telegraf sources. It also exposed two real percentage-scaling defects, one schema/validation mismatch, missing datastore-cluster status, vSAN sample-bucket alignment risk, vSAN test-depth gaps, a misleading compressed-memory unit comment, and an empty vSAN health unknown-state gap, which were fixed before rerunning local validation.
