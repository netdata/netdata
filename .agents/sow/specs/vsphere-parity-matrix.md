# vSphere Collector Parity Matrix

Status: draft baseline for `SOW-0015`.

Purpose: normalize LogicMonitor, Datadog, and mirrored open-source vSphere
coverage into Netdata implementation groups. Every row is classified; there are
no `unknown` rows.

Classification values:

- `existing-default`: current Netdata vSphere collector already collects it by
  default.
- `new-default`: safe additive object-level metric group planned for default
  collection in this SOW.
- `opt-in`: implement only behind explicit config selectors/limits because of
  cardinality, cost, sensitivity, or reviewer scope.
- `covered-elsewhere`: Netdata already has another collector/surface for it.
- `follow-up`: valid requirement, but belongs to another ingestion path or SOW.
- `non-metric-surface`: valid parity surface, but must be implemented through
  topology output or Functions, not as metric labels.
- `out-of-scope-pr`: valid parity surface intentionally excluded from this PR by
  user decision.
- `excluded`: intentionally not supported by default because it conflicts with
  product direction or would duplicate a better Netdata source.

## Source Evidence

Official docs checked on 2026-05-08:

- LogicMonitor VMware vSphere Monitoring:
  `https://www.logicmonitor.com/support/vmware-vsphere-monitoring`
- Datadog vSphere integration:
  `https://docs.datadoghq.com/integrations/vsphere/`
- Broadcom vSphere Web Services API:
  - `VirtualMachineSnapshotInfo`
  - `VirtualMachineSnapshotTree`
  - `DatastoreSummary`
  - `PerformanceManager`
  - `VirtualDisk`
  - `VirtualEthernetCard`
  - `StoragePod`
  - `Network`
  - `NetworkSummary`
  - `DistributedVirtualPortgroup`
  - `HostHostBusAdapter`
  - `HostScsiDisk`
  - `HostMultipathInfo`
  - CPU, network, disk, virtual disk, storage adapter, storage path, and power
    performance-counter pages
- Broadcom vSphere Automation API:
  - `Cis Tagging Tag Association`
- Broadcom vSAN Management API:
  - API overview, managed objects, endpoints, and `VsanPerformanceManager`

Mirrored repository evidence:

Note: local mirrored repositories are shallow clones. Evidence below is
snapshot-only evidence from the checked HEAD commit; it supports current-source
parity comparisons, not history/blame/timeline conclusions.

- `DataDog/integrations-core @ 1befb9012c44152b0aedfb17142041bcc9c1dc61`
  - `vsphere/datadog_checks/vsphere/data/conf.yaml.example:91`
  - `vsphere/datadog_checks/vsphere/data/conf.yaml.example:115`
  - `vsphere/datadog_checks/vsphere/data/conf.yaml.example:175`
  - `vsphere/datadog_checks/vsphere/data/conf.yaml.example:262`
  - `vsphere/datadog_checks/vsphere/data/conf.yaml.example:326`
  - `vsphere/datadog_checks/vsphere/data/conf.yaml.example:375`
  - `vsphere/datadog_checks/vsphere/data/conf.yaml.example:386`
  - `vsphere/datadog_checks/vsphere/metrics.py:74`
  - `vsphere/datadog_checks/vsphere/metrics.py:211`
  - `vsphere/datadog_checks/vsphere/metrics.py:413`
  - `vsphere/datadog_checks/vsphere/metrics.py:497`
- `influxdata/telegraf @ 5a1147f1bb725ff8fd483ea55045506aa70db191`
  - `plugins/inputs/vsphere/README.md:43`
  - `plugins/inputs/vsphere/README.md:86`
  - `plugins/inputs/vsphere/README.md:145`
  - `plugins/inputs/vsphere/README.md:173`
  - `plugins/inputs/vsphere/README.md:187`
  - `plugins/inputs/vsphere/README.md:213`
  - `plugins/inputs/vsphere/vsphere.go:23`
  - `plugins/inputs/vsphere/vsphere.go:153`
  - `plugins/inputs/vsphere/vsan.go:41`
  - `plugins/inputs/vsphere/vsan.go:115`
  - `plugins/inputs/vsphere/vsan.go:201`
- `grafana/vmware_exporter @ 3edc42190c6709567c0465304525f42ead2ac550`
  - `vsphere/test_metrics.txt:1`
  - `vsphere/test_metrics.txt:72`
  - `vsphere/test_metrics.txt:80`
- `elastic/beats @ 7bbe8ee6dcfbf416c53ceb7725909d37a499846c`
  - `metricbeat/module/vsphere/virtualmachine/virtualmachine.go:79`
  - `metricbeat/module/vsphere/virtualmachine/virtualmachine.go:169`
  - `metricbeat/module/vsphere/virtualmachine/data.go:75`
  - `metricbeat/module/vsphere/datastorecluster/datastorecluster.go:46`
  - `metricbeat/module/vsphere/network/network.go`
- `zabbix/zabbix @ bbf78e24c09c90ed9d18f1570b4fd2618981d72f`
  - `templates/app/vmware/template_app_vmware.yaml:1132`
  - `templates/app/vmware/template_app_vmware.yaml:1498`
  - `templates/app/vmware/template_app_vmware.yaml:1532`
  - `templates/app/vmware/template_app_vmware.yaml:1600`
  - `templates/app/vmware/template_app_vmware.yaml:1633`
  - `templates/app/vmware/template_app_vmware.yaml:1703`
  - `templates/app/vmware/template_app_vmware.yaml:1778`
  - `templates/app/vmware/template_app_vmware.yaml:4162`
  - `templates/app/vmware/template_app_vmware.yaml:4418`
- `newrelic/nri-vsphere @ 9366fcd3d597ae0712c94882331042d74fe38e22`
  - `README.md:8`
  - `README.md:26`
  - `README.md:30`
  - `README.md:42`
  - `internal/collect/vms.go:21`
  - `internal/collect/vms.go:22`
  - `internal/collect/networks.go:14`
  - `internal/collect/networks.go:37`
  - `internal/process/datacenter.go:104`
  - `internal/process/hosts.go:95`
  - `internal/process/vms.go:113`
  - `vsphere-performance.metrics:22`
  - `vsphere-performance.metrics:140`
  - `test-data/README.md:1`
- `open-telemetry/opentelemetry-collector-contrib @ 34ed18e037dc63e41c4b4a8356d2a13d55c768f4`
  - `receiver/vcenterreceiver/metadata.yaml:25`
  - `receiver/vcenterreceiver/metadata.yaml:75`
  - `receiver/vcenterreceiver/metadata.yaml:276`
  - `receiver/vcenterreceiver/metadata.yaml:434`
  - `receiver/vcenterreceiver/metadata.yaml:491`
  - `receiver/vcenterreceiver/metadata.yaml:689`
  - `receiver/vcenterreceiver/metadata.yaml:791`
  - `receiver/vcenterreceiver/resources.go:102`
  - `receiver/vcenterreceiver/internal/mockserver/README.md:1`
  - `receiver/vcenterreceiver/internal/mockserver/responses/cluster-vsan.xml`
- `grafana/alloy @ c1b740cd7fc7d2b521304ee15c9c9f61d0d5ceb0`
  - `internal/component/otelcol/receiver/vcenter`

## Matrix

| ID | Surface | Main sources | Netdata target | Classification | Default policy and implementation requirements |
|---|---|---|---|---|---|
| P01 | VM aggregate CPU, memory, swap, disk IO, disk max latency, network traffic, packets, drops, overall alarm status, uptime | Current Netdata; LogicMonitor VM performance; Datadog VM metrics; Telegraf VM metrics | Existing contexts `vsphere.vm_*` in `vsphere-v1-compatibility-manifest.md` | `existing-default` | Preserve contexts, dimensions, labels, units, and sample keys exactly. Chart IDs intentionally change under framework V2 by user decision on 2026-05-08. Empty VM performance scrape results warn and continue with VM property/status metrics and later resource surfaces. |
| P02 | ESXi host aggregate CPU, memory, swap, disk IO, disk max latency, network traffic, packets, drops, errors, overall alarm status, uptime | Current Netdata; LogicMonitor host performance; Datadog host metrics; Telegraf host metrics | Existing contexts `vsphere.host_*` | `existing-default` | Preserve current default collection. Empty host performance scrape results warn and continue with host property/status metrics and later resource surfaces. |
| P03 | Datastore aggregate capacity/free/used/used percent, overall status, IO throughput, IOPS, latency | Current Netdata; LogicMonitor datastore usage/status/throughput; Datadog datastore metrics; Telegraf datastore metrics | Existing contexts `vsphere.datastore_*` | `existing-default` | Preserve datastore accessibility guard: capacity/free/used are trusted only when accessible. |
| P04 | Cluster host count, CPU/memory capacity, CPU topology, DRS/HA enabled, overall status, vMotions, DRS score/balance, VM count, DRS usage summary, aggregate performance, VM operation counters | Current Netdata; LogicMonitor clusters; Datadog cluster metrics; Telegraf cluster metrics; Grafana exporter cluster metrics | Existing contexts `vsphere.cluster_*` | `existing-default` | Preserve property-vs-perf two-phase lifecycle. |
| P05 | Resource pool CPU/memory usage, entitlement, allocation, memory breakdown, config, overall status | Current Netdata; LogicMonitor resource pools; Telegraf resource pools | Existing contexts `vsphere.resource_pool_*` | `existing-default` | Preserve current resource-pool property refresh behavior and labels. |
| P06 | VM snapshot aggregate count, maximum snapshot age, maximum chain depth | User requirement; LogicMonitor VM snapshots; Zabbix snapshot count/latest date; Elastic snapshot info; New Relic optional snapshots; Broadcom snapshot API | Implemented contexts: `vsphere.vm_snapshot_count`, `vsphere.vm_snapshot_max_age`, `vsphere.vm_snapshot_max_chain_depth` with labels `id`, `datacenter`, `cluster`, `host`, `vm` | `new-default` | Object-level per VM. Emits zero for VMs with no snapshots. Does not emit snapshot name/description/ID labels by default. Unit: snapshots, seconds, snapshots. Tests: empty, sibling, nested, zero create time, old create time. |
| P07 | VM snapshot health alerts | User requirement; LogicMonitor/Zabbix snapshot alerting surfaces | Implemented health templates on `vsphere.vm_snapshot_max_chain_depth` and `vsphere.vm_snapshot_max_age` | `new-default` | Warn when chain depth > 3. Critical when max age > 24h. Alert docs and metadata added with the metrics. |
| P08 | Datastore `accessible`, `maintenanceMode`, `uncommitted`, `multipleHostAccess` | Broadcom `DatastoreSummary`; Datadog datastore properties; LogicMonitor datastore status/usage | Implemented contexts: `vsphere.datastore_accessibility_status`, `vsphere.datastore_maintenance_status`, `vsphere.datastore_multiple_host_access`; existing `vsphere.datastore_space_usage` adds `uncommitted` | `new-default` | Object-level per datastore. Preserves capacity/free/uncommitted guard: values are emitted as zero when inaccessible. Maintenance and multi-host access are state-set charts with `unknown` for omitted API values. Initial discovery now keeps inaccessible datastores as status-only resources and datastore perf scraping skips inaccessible datastores. |
| P09 | VM power state, connection state, VMware tools running/version status, disk consolidation-needed status, configured CPU/memory/disk/NIC counts, aggregate storage usage, guest OS name | LogicMonitor VM status; Datadog property metrics; Zabbix tools/status; Elastic VM summary | Implemented contexts: `vsphere.vm_power_state`, `vsphere.vm_connection_state`, `vsphere.vm_tools_running_status`, `vsphere.vm_tools_version_status`, `vsphere.vm_consolidation_needed`, `vsphere.vm_config_cpu`, `vsphere.vm_config_memory`, `vsphere.vm_config_devices`, `vsphere.vm_storage_usage` | `new-default` | Object-level per VM. `vm_power_states` defaults to `poweredOn` and can opt in `poweredOff`/`suspended`. Non-powered-on VMs are property/status/snapshot-only; real-time performance scraping skips them. No guest hostname/IP labels by default. Tools and connection values are bounded state-set dimensions. Guest OS name remains a non-default label/property candidate because it is a free-form string. |
| P10 | Host connection state, power state, maintenance mode | LogicMonitor host status; Datadog property metrics | Implemented contexts: `vsphere.host_power_state`, `vsphere.host_connection_state`, `vsphere.host_maintenance_status` | `new-default` | Object-level per host. `host_power_states` defaults to `poweredOn` and can opt in `poweredOff`/`standBy`/`unknown`. Non-powered-on hosts are property/status-only; real-time performance scraping skips them. Collector-generated ESXi vnode routing is excluded from this PR. |
| P11 | Cluster DRS mode/vMotion rate and HA details beyond current enabled/admission-control booleans | Datadog property metrics; LogicMonitor HA/admission control | Implemented contexts: existing `vsphere.cluster_drs_config`, `vsphere.cluster_ha_config`; additive `vsphere.cluster_drs_mode`, `vsphere.cluster_drs_vmotion_rate`, `vsphere.cluster_ha_host_monitoring`, `vsphere.cluster_ha_vm_monitoring`, `vsphere.cluster_ha_vm_component_protection` | `new-default` | Object-level per cluster. Uses bounded state-set dimensions plus a numeric vMotion recommendation threshold. Does not expose free-form cluster config strings as labels. |
| P12 | Datacenter object counts and inventory counts | LogicMonitor object count/info; Datadog datacenter metrics; Telegraf datacenter controls | Implemented context: `vsphere.inventory_objects` with datacenters, folders, clusters, hosts, VMs, datastores, and resource-pool dimensions after include filters are applied | `new-default` | Job-level aggregate metric, not a mandatory vCenter vnode. V2 uses only the static `id=inventory` instance label. |
| P13 | Datastore clusters / storage pods capacity and usage | LogicMonitor datastore clusters; Broadcom `StoragePod`; Elastic datastorecluster module | Implemented optional contexts: `vsphere.datastore_cluster_space_utilization`, `vsphere.datastore_cluster_space_usage`, `vsphere.datastore_cluster_storage_drs_status` | `opt-in` | Default off. Enable with `collect_datastore_clusters`. `datastore_cluster_include` matches `/Datacenter/DatastoreCluster`, name, or managed object ID. Labels: `id`, `datacenter`, `datastore_cluster`. |
| P14 | VM virtual disk capacity by disk/device | LogicMonitor VM disk capacity; Broadcom `VirtualDisk`; Zabbix VM storage; Datadog property/perf metrics | Implemented optional context: `vsphere.vm_disk_capacity` by disk instance | `opt-in` | Default off. Enable with `collect_vm_disks`. `vm_disk_include` matches disk display label, numeric disk key, or `key:<disk_key>`. Labels: `id`, `datacenter`, `cluster`, `host`, `vm`, `disk`, `disk_key`, plus opt-in enrichment labels. |
| P15 | VM virtual disk performance by disk/device | Broadcom disk I/O counter docs; Datadog per-instance `virtualDisk.*`; Telegraf `virtualDisk.*`; OTel VM disk metrics; New Relic performance levels | Implemented optional contexts: `vsphere.vm_disk_device_io`, `vsphere.vm_disk_device_iops`, `vsphere.vm_disk_device_latency`, `vsphere.vm_disk_device_outstanding_io` | `opt-in` | Default off. Enable with `collect_vm_disk_performance`. Uses vSphere `virtualDisk` performance instances, for example `scsi0:0`, requested with wildcard instance `*`. `vm_disk_include` matches the performance instance or `instance:<disk_instance>`. Labels: `id`, `datacenter`, `cluster`, `host`, `vm`, `disk`, `disk_instance`, plus opt-in enrichment labels. |
| P16 | VM network interface throughput/packets/errors/drops by NIC | Broadcom latest network counters; Datadog per-instance VM `net.*`; Telegraf VM instance metrics; OTel VM vNIC metrics; LogicMonitor VM interface | Implemented optional contexts: `vsphere.vm_net_interface_traffic`, `vsphere.vm_net_interface_packets`, `vsphere.vm_net_interface_drops`, `vsphere.vm_net_interface_broadcast_packets`, `vsphere.vm_net_interface_multicast_packets` | `opt-in` | Default off. Enable with `collect_vm_nic_performance`. Uses vSphere `net.*` performance instances requested with wildcard instance `*`. `vm_nic_include` matches the raw performance instance or `interface:<interface_instance>`. Labels: `id`, `datacenter`, `cluster`, `host`, `vm`, `interface`, `interface_instance`, plus opt-in enrichment labels. VM packet error counters are not emitted because the current Broadcom network counter table documents `errorsRx`/`errorsTx` for `HostSystem`, not VM. |
| P17 | Host physical NIC metrics by NIC | Broadcom latest network counters; LogicMonitor ESXi network interfaces; Telegraf host instances; Datadog per-instance host metrics; OTel host pNIC metrics | Implemented optional contexts: `vsphere.host_net_interface_traffic`, `vsphere.host_net_interface_packets`, `vsphere.host_net_interface_drops`, `vsphere.host_net_interface_errors`, `vsphere.host_net_interface_broadcast_packets`, `vsphere.host_net_interface_multicast_packets`, `vsphere.host_net_interface_unknown_protocol_frames`, `vsphere.host_net_interface_usage` | `opt-in` | Default off. Enable with `collect_host_nic_performance`. Uses vSphere `net.*` performance instances requested with wildcard instance `*`. `host_nic_include` matches the raw performance instance or `interface:<interface_instance>`. Labels: `id`, `datacenter`, `cluster`, `host`, `interface`, `interface_instance`, plus opt-in enrichment labels. Existing aggregate ESXi host network metrics remain default-on and are not suppressed by this optional per-interface surface. |
| P18 | Host disk/LUN/device metrics by disk/device | Broadcom disk counters and `HostScsiDisk`; LogicMonitor ESXi disks; Datadog per-instance disk metrics; Telegraf host disk metrics; OTel host disk metrics | Implemented optional contexts: `vsphere.host_disk_device_io`, `vsphere.host_disk_device_iops`, `vsphere.host_disk_device_requests`, `vsphere.host_disk_device_latency`, `vsphere.host_disk_device_latency_breakdown`, `vsphere.host_disk_device_read_latency_breakdown`, `vsphere.host_disk_device_write_latency_breakdown`, `vsphere.host_disk_device_commands`, `vsphere.host_disk_device_command_events`, `vsphere.host_disk_device_queue_depth`, `vsphere.host_disk_device_scsi_reservation_conflicts`, `vsphere.host_disk_device_scsi_reservation_conflicts_percentage` | `opt-in` | Default off. Enable with `collect_host_disk_performance`. Uses vSphere `disk.*` performance instances requested with wildcard instance `*`. `host_disk_include` matches the raw performance instance or `instance:<disk_instance>`. Labels: `id`, `datacenter`, `cluster`, `host`, `disk`, `disk_instance`, plus opt-in enrichment labels. Existing aggregate ESXi host disk metrics remain default-on and are not suppressed by this optional per-device surface. IQN/WWN/serial labels are not emitted by default. |
| P19 | Host storage adapter metrics | Broadcom storage adapter counters and `HostHostBusAdapter`; Datadog/Telegraf/New Relic storageAdapter metrics | Implemented optional contexts: `vsphere.host_storage_adapter_io`, `vsphere.host_storage_adapter_commands`, `vsphere.host_storage_adapter_latency`, `vsphere.host_storage_adapter_queue`, `vsphere.host_storage_adapter_outstanding_io_percentage`, `vsphere.host_storage_adapter_throughput`, `vsphere.host_storage_adapter_throughput_contention`, `vsphere.host_storage_adapter_max_latency` | `opt-in` | Default off. Enable with `collect_host_storage_adapter_performance`. Uses vSphere `storageAdapter.*` performance instances requested with wildcard instance `*`, plus the aggregate-only `storageAdapter.maxTotalLatency.latest` counter. `host_storage_adapter_include` matches the raw performance instance, `adapter:<adapter_instance>`, or `instance:<adapter_instance>`. Per-adapter labels: `id`, `datacenter`, `cluster`, `host`, `adapter`, `adapter_instance`, plus opt-in enrichment labels. Aggregate maximum-latency labels: `id`, `datacenter`, `cluster`, `host`, plus opt-in enrichment labels. |
| P20 | Host storage path metrics | Broadcom storage path counters and `HostMultipathInfo`; Datadog/Telegraf/New Relic storagePath metrics | Implemented optional contexts: `vsphere.host_storage_path_io`, `vsphere.host_storage_path_commands`, `vsphere.host_storage_path_latency`, `vsphere.host_storage_path_command_events`, `vsphere.host_storage_path_throughput`, `vsphere.host_storage_path_throughput_contention`, `vsphere.host_storage_path_max_latency` | `opt-in` | Default off. Enable with `collect_host_storage_path_performance`. Uses vSphere `storagePath.*` performance instances requested with wildcard instance `*`, plus the aggregate-only `storagePath.maxTotalLatency.latest` counter. `host_storage_path_include` matches the raw performance instance, `path:<path_instance>`, or `instance:<path_instance>`. Per-path labels: `id`, `datacenter`, `cluster`, `host`, `path`, `path_instance`, plus opt-in enrichment labels. Aggregate maximum-latency labels: `id`, `datacenter`, `cluster`, `host`, plus opt-in enrichment labels. IQN/WWN labels are not emitted by default. |
| P21 | Host CPU core/thread/logical processor metrics | Broadcom CPU counters; LogicMonitor logical processors; Telegraf host CPU instances; New Relic host CPU instance counters; OTel idle CPU metric | Implemented optional contexts: `vsphere.host_cpu_instance_utilization`, `vsphere.host_cpu_instance_time` | `opt-in` | Default off. Enable with `collect_host_cpu_instance_performance`. Uses vSphere HostSystem instance counters `cpu.usage.average`, `cpu.utilization.average`, `cpu.coreUtilization.average`, `cpu.used.summation`, and `cpu.idle.summation` requested with wildcard instance `*`. `host_cpu_instance_include` matches the raw performance instance, `cpu:<cpu_instance>`, or `instance:<cpu_instance>`. Labels: `id`, `datacenter`, `cluster`, `host`, `cpu`, `cpu_instance`, plus opt-in enrichment labels. |
| P22 | Host and VM power/energy counters | Broadcom power counters; LogicMonitor ESXi power; Datadog/Telegraf/New Relic power counters | Implemented optional contexts: `vsphere.host_power_usage`, `vsphere.host_power_capacity_usage`, `vsphere.host_power_capacity_utilization`, `vsphere.host_energy_usage`, `vsphere.vm_power_usage`, `vsphere.vm_energy_usage` | `opt-in` | Default off. Enable with `collect_power_metrics`. Uses aggregate vSphere `power.*` counters requested with empty instance for discovered powered-on hosts and VMs. No child selector is added because this emits one aggregate set per included host/VM. Host labels: `id`, `datacenter`, `cluster`, `host`, plus opt-in enrichment labels. VM labels: `id`, `datacenter`, `cluster`, `host`, `vm`, plus opt-in enrichment labels. |
| P23 | Hardware health sensors: fans, power, storage, memory, processor, system sensors | LogicMonitor ESXi hardware/system sensors; SNMP `vmware-esx` profile | SNMP profile `vmware-esx`; optional future direct vSphere/CIM metrics | `covered-elsewhere` | Do not duplicate by default. vSphere docs point users to `snmp` with `vmware-esx`; if direct API sensors are added later, make opt-in and map against SNMP coverage. |
| P24 | ESXi HBA/environment/hardware health via SNMP | Netdata SNMP `vmware-esx` overlap | SNMP `vmware-esx` profile | `covered-elsewhere` | vSphere docs mention this as complementary per-host monitoring. |
| P25 | vCenter Server Appliance CPU, memory, disk, filesystem, services, health, VCHA, backup | LogicMonitor VCSA modules | Netdata `vcsa` collector | `covered-elsewhere` | Keep out of this vSphere collector. vSphere docs point users to `vcsa` for appliance health. |
| P26 | vSAN cluster, host, and VM capacity/performance/health/events | Broadcom vSAN Management API; Datadog vSAN metrics/events; Telegraf vSAN controls; OTel vSAN metrics/fixtures | Implemented opt-in contexts: `vsphere.vsan_cluster_space_usage`, `vsphere.vsan_cluster_space_utilization`, `vsphere.vsan_cluster_health_status`, `vsphere.vsan_cluster_operations`, `vsphere.vsan_cluster_throughput`, `vsphere.vsan_cluster_latency`, `vsphere.vsan_cluster_congestions`, `vsphere.vsan_host_operations`, `vsphere.vsan_host_throughput`, `vsphere.vsan_host_latency`, `vsphere.vsan_host_congestions`, `vsphere.vsan_host_cache_hit_rate`, `vsphere.vsan_vm_operations`, `vsphere.vsan_vm_throughput`, `vsphere.vsan_vm_latency` | `opt-in` | Default off. Enable with `collect_vsan`. Uses vSAN Management API only for vSAN-enabled clusters that pass `vsan_cluster_include`; host and VM vSAN performance entities pass `vsan_host_include` and `vsan_vm_include`. Emits cluster space/health and the OTel/Datadog common cluster/host/VM vSAN performance subset. vSAN events are excluded with P27. Deeper vSAN disk-group, disk, component, CMMDS, and all Telegraf entity-type metrics remain a residual parity gap because they need explicit Netdata NIDL/context mapping and bounded config policy before implementation. |
| P27 | vCenter and ESXi events, alarms, event filters | Datadog events; LogicMonitor LogSources; New Relic optional events | Future logs/events ingestion, likely OTEL/log path | `out-of-scope-pr` | User decision 2026-05-08: do not implement events in this PR. Do not overload metric collector. |
| P28 | vSphere tags and custom attributes as labels | Broadcom Tag Association API and CustomFieldsManager; Datadog tags/attributes; Telegraf custom attributes; Elastic custom fields; New Relic tags | Implemented opt-in labels `vsphere_tag_<sanitized_category>` and `vsphere_custom_attribute_<sanitized_name>` | `opt-in` | Default off. `vsphere_tag_categories` allowlists tag categories with one glob pattern per YAML list item; multiple tags in one category are sorted and joined with the pipe character. `custom_attributes` allowlists custom attributes with one glob pattern per YAML list item. Discovery fails open with warnings if tag/custom-attribute APIs are unavailable. |
| P29 | Inventory paths, folder lineage, guest hostnames, guest IPs, MAC/IQN/WWN ERI-like identity | LogicMonitor topology/ERI/netscan; Datadog filters; Telegraf IP addresses; OTel inventory-path resource attributes; Broadcom guest/device/storage objects | Implemented subset: optional `inventory_path`, `guest_hostname`, `guest_ip`, `guest_os` labels. Remaining MAC/IQN/WWN/device identity remains unimplemented. | `opt-in` | Default off. `collect_inventory_path_label` adds derived inventory paths for VM, host, datastore, cluster, and resource-pool charts. `vm_guest_labels` is an explicit allowlist for `guest_hostname`, `guest_ip`, and `guest_os`. Remaining MAC/IQN/WWN labels need separate allowlists/caps before implementation because they are sensitive multi-value identity surfaces. |
| P30 | Topology edges: cluster, datastore, network, VM topology | LogicMonitor topology sources; Elastic network/datastorecluster modules; OTel resource model; New Relic object fixtures; Netdata Function topology schema and SNMP topology pattern | Implemented public `topology:vsphere` cached Function alias with required job selector | `non-metric-surface` | Uses cached discovery state and emits topology actors/links, not metric labels. Default inventory topology includes datacenters, clusters, hosts, VMs, datastores, datastore clusters, and resource pools. Network actors and host/VM network links are included only when `collect_network_topology` is enabled. Canonical framework registration is also available as `vsphere:topology:vsphere`; topology consumers should use `topology:vsphere`. |
| P31 | Network and distributed virtual port group status | LogicMonitor network state; Elastic network module; New Relic network object fixtures; Broadcom `NetworkSummary.accessible`; Broadcom `DistributedVirtualPortgroup` | Implemented opt-in cached topology actors through `collect_network_topology` | `non-metric-surface` | Default off to avoid extra vCenter discovery calls for existing users. When enabled, discovers vSphere `Network` objects, including distributed virtual port groups returned by the Network view, and exposes cached `accessible`, `overall_status`, type, host count, VM count, and host/VM links in the topology Function. Does not create charts, metrics, or free-form network path labels. |
| P32 | Troubleshooter and permission/readiness checks | LogicMonitor troubleshooter; Datadog service checks; Netdata Function table schema | Implemented `vsphere:readiness` cached Function with required job selector | `non-metric-surface` | Read-only cached Function reporting target/credential presence, initialized client/discovery/performance-counter state, inventory counts, optional metric/label gates, network topology gate, and cached vSAN counts. It does not expose configured URL/credentials and does not issue extra vCenter API calls. Live permission probes remain intentionally absent from this PR. |
| P33 | VM vnodes | User discussion; Netdata agent-on-VM duplication risk; V2 host-scope spec; 2026-05-20 review decision | No collector-generated VM vnodes in this PR | `out-of-scope-pr` | Hard-removed before merge. Reintroduce only through a focused PR with stable VM identity, lifecycle, docs, and duplicate-node policy. |
| P34 | ESXi vnodes | User decision; V2 host-scope spec; 2026-05-20 review decision | No collector-generated ESXi vnodes in this PR | `out-of-scope-pr` | Hard-removed before merge. Reintroduce only through a focused PR with stable ESXi identity and lifecycle policy. |
| P35 | Datastore vnodes | User decision and cardinality/identity review | No datastore vnodes in this SOW | `excluded` | Keep datastores as metric instances, not nodes. Revisit only with product decision. |
| P36 | TKG/Kubernetes workload metrics inside vSphere VMs | Datadog TKG note; Netdata Kubernetes collectors/agents | Netdata Agent and Kubernetes collectors inside guest/cluster | `covered-elsewhere` | Do not collect container/pod/node workload metrics through vSphere. |

## Implementation Gates Derived From Matrix

Before framework v2 migration:

- Preserve all `existing-default` rows through the compatibility manifest.
- Add no `new-default` row until the v2 migration passes the manifest.

For `new-default` rows:

- Add one resource type per context.
- Use only stable state dimensions and numeric dimensions.
- Do not add sensitive labels by default.
- Add docs, metadata, config schema, stock config, and health alert updates in
  the same commit group.

For `opt-in` rows:

- Each row needs a config knob and selector/allowlist when the surface can
  emit child-instance or user-defined labels. User decision on 2026-05-20
  removed all `max_*` knobs from this collector; selectors and allowlists are
  the controls for optional resource instances and user-defined labels.
- Default must be off.
- Tests must cover both disabled and enabled behavior.

For `covered-elsewhere` rows:

- Update vSphere docs to explain the complementary collector.
- Do not duplicate default data unless a later SOW records a product decision.

For `follow-up` rows:

- No remaining row should use `follow-up` in this PR unless the user explicitly
  asks to split it again.

For `non-metric-surface` rows:

- Implement through topology output or Functions only.
- Do not add topology/resource identity as metric labels unless that label is
  separately covered by an opt-in label-enrichment row.

For `out-of-scope-pr` rows:

- Record the user decision and do not implement in this PR.
