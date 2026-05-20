# vSphere Collector V1 Compatibility Manifest

Status: draft baseline for `SOW-0015`; executable chart/metric subset is
checked by `TestCollector_V1CompatibilityManifest`.

This manifest records the pre-migration v1 contract that guides the framework v2 migration.
The migration preserves contexts, dimensions, old labels, units, configuration,
and metric meaning. Chart IDs are recorded for traceability but intentionally
change to normal framework V2 instance chart IDs by user decision on 2026-05-08.
The V2 chartengine path adds one new instance label, `id`, set to the vSphere
managed-object reference that V1 used as the chart-ID prefix.

Runtime substitution:

- `%s` in chart and dimension IDs is the vSphere managed object reference value
  stored as the Netdata resource ID, for example synthetic `vcsim` IDs such as
  `vm-62`, `host-21`, `datastore-59`, `domain-c28`, or `resgroup-27`.
- Default chart type is `line` when the code does not set `Type`.
- Default dimension algorithm is `absolute`; default multiplier and divisor are
  `1`.

## Sources

- `src/go/plugin/go.d/collector/vsphere/collector.go`
- `src/go/plugin/go.d/collector/vsphere/charts.go`
- `src/go/plugin/go.d/collector/vsphere/collect.go`
- `src/go/plugin/go.d/collector/vsphere/discover/metric_lists.go`
- `src/go/plugin/go.d/collector/vsphere/config_schema.json`
- `src/go/plugin/go.d/collector/vsphere/metadata.yaml`
- `src/go/plugin/go.d/config/go.d/vsphere.conf`
- `src/health/health.d/vsphere.conf`
- `src/go/plugin/go.d/collector/vsphere/collector_test.go`
- `src/go/plugin/go.d/collector/vsphere/compat_manifest_test.go`
- `src/go/plugin/go.d/collector/vsphere/testdata/v1_compat_manifest.json`

Executable golden scope:

- records V1 chart IDs and pins contexts, titles, units, families, chart types,
  priorities, label keys, label sources, dimension names/algorithms/scales/options,
  and metric sample keys/values from `vcsim`;
- does not pin simulator-specific label values because `vcsim` can assign VM
  runtime host labels differently between runs;
- must be regenerated only with intentional contract changes.

## Collector Registration And Defaults

| Field | Current v1 contract |
|---|---|
| Module | `vsphere` |
| Framework registration baseline | Pre-migration `Create`, returning `collectorapi.CollectorV1`; current implementation registers `CreateV2` while preserving this public chart/metric surface except chart IDs. |
| Default `update_every` | `20` seconds |
| Default HTTP timeout | `20s` |
| Default discovery interval | `5m` |
| Default host include | `/*` |
| Default VM include | `/*` |
| Default datastore include | `/*` |
| Default cluster include | `/*` |
| Default inventory path label | `false` |
| Default VM guest labels | empty allowlist |
| Default vSphere tag category labels | empty allowlist |
| Default custom attribute labels | empty allowlist |
| Default datastore cluster collection | `false` |
| Default datastore cluster include | `/*` |
| Default VM disk collection | `false` |
| Default VM disk performance collection | `false` |
| Default VM disk include | `*` |
| Default VM NIC performance collection | `false` |
| Default VM NIC include | `*` |
| Default host NIC performance collection | `false` |
| Default host NIC include | `*` |
| Default host disk performance collection | `false` |
| Default host disk include | `*` |
| Default host storage adapter performance collection | `false` |
| Default host storage adapter include | `*` |
| Default host storage path performance collection | `false` |
| Default host storage path include | `*` |
| Default host CPU instance performance collection | `false` |
| Default host CPU instance include | `*` |
| Default vSAN collection | `false` |
| Default network topology collection | `false` |
| Collection output baseline | Pre-migration `Collect(context.Context) map[string]int64`; current public collection path writes to the framework V2 metric store. |
| Config schema embed | `config_schema.json` |

## Configuration Contract

Struct fields and YAML/JSON keys that must remain accepted:

| YAML key | JSON key | Required by schema | Notes |
|---|---|---:|---|
| `vnode` | `vnode` | no | Existing job-level vnode association. |
| `update_every` | `update_every` | no | Default `20`. |
| `autodetection_retry` | `autodetection_retry` | no | Schema default `60`; metadata lists `0`. Preserve accepted key. |
| `url` | `url` | yes | From embedded HTTP config. |
| `timeout` | `timeout` | no | From embedded HTTP config. |
| `discovery_interval` | `discovery_interval` | no | Minimum `60`, default `300`. |
| `not_follow_redirects` | `not_follow_redirects` | no | From embedded HTTP config. |
| `host_include` | `host_include` | no | Selector list, default `/*`. |
| `vm_include` | `vm_include` | no | Selector list, default `/*`. |
| `datastore_include` | `datastore_include` | no | Selector list, default `/*`. |
| `cluster_include` | `cluster_include` | no | Selector list, default `/*`. |
| `tag_categories` | `tag_categories` | no | Optional vSphere tag category label allowlist. Default empty; each YAML list item is one glob pattern matching a tag category name. Matching categories become labels named `vsphere_tag_<sanitized_category>`; multiple tags in one category are sorted and joined with the pipe character. |
| `custom_attributes` | `custom_attributes` | no | Optional vSphere custom attribute label allowlist. Default empty; each YAML list item is one glob pattern matching a custom attribute name. Matching attributes become labels named `vsphere_custom_attribute_<sanitized_name>`. |
| `collect_datastore_clusters` | `collect_datastore_clusters` | no | Optional datastore cluster / StoragePod metrics. Default `false`. |
| `datastore_cluster_include` | `datastore_cluster_include` | no | Optional simple-pattern allowlist for datastore clusters. Default `/*`; matches `/Datacenter/DatastoreCluster`, datastore-cluster name, or managed object ID. |
| `collect_vm_disks` | `collect_vm_disks` | no | Optional VM virtual disk capacity metrics. Default `false`. |
| `collect_vm_disk_performance` | `collect_vm_disk_performance` | no | Optional VM virtual disk performance metrics. Default `false`; requests `virtualDisk.*` counters with wildcard instance `*` and emits only returned per-disk performance instances. |
| `vm_disk_include` | `vm_disk_include` | no | Optional simple-pattern allowlist for VM virtual disks. Default `*`; capacity collection matches disk display label, numeric disk key, or `key:<disk_key>`; performance collection matches the vSphere performance instance or `instance:<disk_instance>`. |
| `collect_vm_nic_performance` | `collect_vm_nic_performance` | no | Optional VM network-interface performance metrics. Default `false`; requests selected `net.*` counters with wildcard instance `*` and emits only returned per-interface performance instances. |
| `vm_nic_include` | `vm_nic_include` | no | Optional simple-pattern allowlist for VM network interfaces. Default `*`; matches the vSphere performance instance or `interface:<interface_instance>`. |
| `collect_host_nic_performance` | `collect_host_nic_performance` | no | Optional host physical network-interface performance metrics. Default `false`; requests selected `net.*` counters with wildcard instance `*` and emits only returned per-interface performance instances. |
| `host_nic_include` | `host_nic_include` | no | Optional simple-pattern allowlist for host physical network interfaces. Default `*`; matches the vSphere performance instance or `interface:<interface_instance>`. |
| `collect_host_disk_performance` | `collect_host_disk_performance` | no | Optional host disk/LUN/device performance metrics. Default `false`; requests selected `disk.*` counters with wildcard instance `*` and emits only returned per-device performance instances. |
| `host_disk_include` | `host_disk_include` | no | Optional simple-pattern allowlist for host disk/LUN/device performance instances. Default `*`; matches the vSphere performance instance or `instance:<disk_instance>`. |
| `collect_host_storage_adapter_performance` | `collect_host_storage_adapter_performance` | no | Optional host storage adapter performance metrics. Default `false`; requests selected `storageAdapter.*` counters with wildcard instance `*` and the aggregate-only maximum latency counter with empty instance. |
| `host_storage_adapter_include` | `host_storage_adapter_include` | no | Optional simple-pattern allowlist for host storage adapter performance instances. Default `*`; matches the vSphere performance instance, `adapter:<adapter_instance>`, or `instance:<adapter_instance>`. |
| `collect_host_storage_path_performance` | `collect_host_storage_path_performance` | no | Optional host storage path performance metrics. Default `false`; requests selected `storagePath.*` counters with wildcard instance `*` and the aggregate-only maximum latency counter with empty instance. |
| `host_storage_path_include` | `host_storage_path_include` | no | Optional simple-pattern allowlist for host storage path performance instances. Default `*`; matches the vSphere performance instance, `path:<path_instance>`, or `instance:<path_instance>`. |
| `collect_host_cpu_instance_performance` | `collect_host_cpu_instance_performance` | no | Optional host CPU/core/logical-CPU performance metrics. Default `false`; requests selected `cpu.*` counters with wildcard instance `*`. |
| `host_cpu_instance_include` | `host_cpu_instance_include` | no | Optional simple-pattern allowlist for host CPU performance instances. Default `*`; matches the vSphere performance instance, `cpu:<cpu_instance>`, or `instance:<cpu_instance>`. |
| `collect_power_metrics` | `collect_power_metrics` | no | Optional aggregate host/VM power and energy metrics. Default `false`; requests selected `power.*` counters with empty instance for included powered-on hosts and VMs. |
| `collect_vsan` | `collect_vsan` | no | Optional vSAN metrics. Default `false`; requests vSAN cluster space/health and vSAN cluster, host, and VM performance metrics through the vSAN Management API for vSAN-enabled clusters. |
| `vsan_cluster_include` | `vsan_cluster_include` | no | Optional simple-pattern allowlist for vSAN-enabled clusters. Default `/*`; matches `/Datacenter/Cluster`, cluster name, managed object ID, or `vsan_uuid:<uuid>`. |
| `vsan_host_include` | `vsan_host_include` | no | Optional simple-pattern allowlist for vSAN host performance entities. Default `/*`; matches `/Datacenter/Cluster/Host`, host name, managed object ID, or `vsan_node_uuid:<uuid>`. |
| `vsan_vm_include` | `vsan_vm_include` | no | Optional simple-pattern allowlist for vSAN VM performance entities. Default `/*`; matches `/Datacenter/Cluster/Host/VM`, VM name, managed object ID, or `instance_uuid:<uuid>`. |
| `collect_network_topology` | `collect_network_topology` | no | Optional vSphere Network discovery for the cached topology Function. Default `false`; discovers Network/Distributed Virtual Port Group objects for topology only and emits no charts or metrics. |
| `username` | `username` | yes | Sensitive. |
| `password` | `password` | yes | Sensitive. |
| `bearer_token_file` | `bearer_token_file` | no | Hidden in UI schema. |
| `force_http2` | `force_http2` | no | Hidden in UI schema. |
| `proxy_url` | `proxy_url` | no | From embedded HTTP config. |
| `proxy_username` | `proxy_username` | no | Sensitive. |
| `proxy_password` | `proxy_password` | no | Sensitive. |
| `headers` | `headers` | no | Object or null. |
| `tls_skip_verify` | `tls_skip_verify` | no | From embedded HTTP config. |
| `tls_ca` | `tls_ca` | no | Absolute path or empty. |
| `tls_cert` | `tls_cert` | no | Absolute path or empty. |
| `tls_key` | `tls_key` | no | Absolute path or empty. |
| `body` | `body` | no | Hidden in UI schema. |
| `method` | `method` | no | Hidden in UI schema. |

Stock config examples:

- `vcenter1`: `url`, `username`, `password`
- `vcenter2`: `url`, `username`, `password`
- The stock config also documents selector formats, secret resolver syntax,
  optional `vnode`, safe discovery defaults, and every valid host/VM power
  state as commented examples.

## Label Contract

| Resource | Labels and value sources |
|---|---|
| Inventory | V2 adds `id=inventory`; no old V1 labels |
| VM | `datacenter=vm.Hier.DC.Name`, `cluster=getVMClusterName(vm)`, `host=vm.Hier.Host.Name`, `vm=vm.Name`; V2 also adds `id=vm.ID` |
| Host | `datacenter=host.Hier.DC.Name`, `cluster=getHostClusterName(host)`, `host=host.Name`; V2 also adds `id=host.ID` |
| Datastore | `datacenter=ds.Hier.DC.Name`, `datastore=ds.Name`, `type=ds.Type`; V2 also adds `id=ds.ID` |
| Cluster | `datacenter=cl.Hier.DC.Name`, `cluster=cl.Name`; V2 also adds `id=cl.ID` |
| Datastore cluster | `id=pod.ID`, `datacenter=pod.Hier.DC.Name`, `datastore_cluster=pod.Name` |
| VM virtual disk capacity | `id=vm.ID`, `datacenter=vm.Hier.DC.Name`, `cluster=vm.Hier.Cluster.Name`, `host=vm.Hier.Host.Name`, `vm=vm.Name`, `disk=<display-label>`, `disk_key=<virtual-device-key>` |
| VM virtual disk performance | `id=vm.ID`, `datacenter=vm.Hier.DC.Name`, `cluster=vm.Hier.Cluster.Name`, `host=vm.Hier.Host.Name`, `vm=vm.Name`, `disk=<vSphere-performance-instance>`, `disk_instance=<vSphere-performance-instance>` |
| VM network interface performance | `id=vm.ID`, `datacenter=vm.Hier.DC.Name`, `cluster=vm.Hier.Cluster.Name`, `host=vm.Hier.Host.Name`, `vm=vm.Name`, `interface=<vSphere-performance-instance>`, `interface_instance=<vSphere-performance-instance>` |
| Host physical network interface performance | `id=host.ID`, `datacenter=host.Hier.DC.Name`, `cluster=host.Hier.Cluster.Name`, `host=host.Name`, `interface=<vSphere-performance-instance>`, `interface_instance=<vSphere-performance-instance>` |
| Host disk/LUN/device performance | `id=host.ID`, `datacenter=host.Hier.DC.Name`, `cluster=host.Hier.Cluster.Name`, `host=host.Name`, `disk=<vSphere-performance-instance>`, `disk_instance=<vSphere-performance-instance>` |
| Host storage adapter performance | `id=host.ID`, `datacenter=host.Hier.DC.Name`, `cluster=host.Hier.Cluster.Name`, `host=host.Name`, `adapter=<vSphere-performance-instance>`, `adapter_instance=<vSphere-performance-instance>` |
| Host storage adapter aggregate performance | `id=host.ID`, `datacenter=host.Hier.DC.Name`, `cluster=host.Hier.Cluster.Name`, `host=host.Name` |
| Host storage path performance | `id=host.ID`, `datacenter=host.Hier.DC.Name`, `cluster=host.Hier.Cluster.Name`, `host=host.Name`, `path=<vSphere-performance-instance>`, `path_instance=<vSphere-performance-instance>` |
| Host storage path aggregate performance | `id=host.ID`, `datacenter=host.Hier.DC.Name`, `cluster=host.Hier.Cluster.Name`, `host=host.Name` |
| Host CPU instance performance | `id=host.ID`, `datacenter=host.Hier.DC.Name`, `cluster=host.Hier.Cluster.Name`, `host=host.Name`, `cpu=<vSphere-performance-instance>`, `cpu_instance=<vSphere-performance-instance>` |
| vSAN cluster | `id=cluster.ID`, `datacenter=cluster.Hier.DC.Name`, `cluster=cluster.Name`, `vsan_uuid=cluster.VSANUUID` |
| vSAN host | `id=host.ID`, `datacenter=host.Hier.DC.Name`, `cluster=getHostClusterName(host)`, `host=host.Name`, `vsan_node_uuid=host.VSANNodeUUID` |
| vSAN VM | `id=vm.ID`, `datacenter=vm.Hier.DC.Name`, `cluster=getVMClusterName(vm)`, `host=vm.Hier.Host.Name`, `vm=vm.Name`, `vm_instance_uuid=vm.InstanceUUID` |
| Resource pool | `datacenter=rp.Hier.DC.Name`, `cluster=rp.Hier.Cluster.Name`, `resource_pool=rp.Name`; V2 also adds `id=rp.ID` |

Compatibility details:

- `getVMClusterName()` returns an empty string when the VM cluster name equals
  the host name.
- `getHostClusterName()` returns an empty string when the host cluster name
  equals the host name.
- By default, no per-resource V2 host scopes are created; all metrics follow
  the current job/global host behavior, with optional job-level `vnode`.
- Collector-generated ESXi/VM vnodes are excluded from this PR by user decision
  on 2026-05-20. Reintroducing them requires a separate design for stable
  resource identity and host-scope lifecycle.
- Empty host or VM real-time performance scrape results do not abort the whole
  collection cycle. The collector logs a warning and still emits available
  property/status metrics plus the remaining datastore, cluster, resource-pool,
  and vSAN surfaces.
- Optional `tag_categories` and `custom_attributes` add user-defined
  vSphere metadata labels to VM, host, datastore, cluster, resource-pool, and
  datastore-cluster metrics when the matching resource has those values.
  Labels use sanitized keys prefixed with `vsphere_tag_` or
  `vsphere_custom_attribute_`. These labels are default-off because they may
  expose ownership, business, environment, or internal naming data.

## Function Contract

- `vsphere:readiness` is a read-only module Function with the framework job
  selector parameter. It reports cached collector readiness rows for
  target/credential presence, client/discovery/performance-counter state,
  inventory counts, optional metric/label gates, network-topology gate,
  and cached vSAN counts. It does not expose the configured vCenter URL,
  username, password, or object inventory names, and it does not issue extra
  vCenter API calls.
- `topology:vsphere` is the public read-only topology Function alias with the
  framework job selector parameter and response type `topology`. It reports
  cached inventory actors and links for datacenters, clusters, ESXi hosts, VMs,
  datastores, datastore clusters, and resource pools. When
  `collect_network_topology` is enabled, it also includes cached vSphere
  Network/Distributed Virtual Port Group actors, accessibility/status
  attributes, and host/VM network links. The canonical framework method also
  registers as `vsphere:topology:vsphere`, but topology consumers should use
  `topology:vsphere` to match the existing topology Function convention.
- Function surfaces are additive and do not change metric contexts,
  dimensions, chart templates, default host scopes, or existing configuration
  behavior.

## Chart And Dimension Contract

| Resource | Lifecycle | Chart ID template | Context | Family | Units | Type | Priority constant | Dimensions |
|---|---|---|---|---|---|---|---|---|
| Inventory | static job-level chart from collector initialization | `inventory_objects` | `vsphere.inventory_objects` | `inventory` | `objects` | `line` | `prioInventoryObjects` | `inventory_datacenters=>datacenters`; `inventory_folders=>folders`; `inventory_clusters=>clusters`; `inventory_hosts=>hosts`; `inventory_vms=>vms`; `inventory_datastores=>datastores`; `inventory_resource_pools=>resource_pools` |
| Datastore cluster | optional charted only when `collect_datastore_clusters` is enabled and the StoragePod matches `datastore_cluster_include` | `datastore_cluster_space_utilization` | `vsphere.datastore_cluster_space_utilization` | `datastore clusters space` | `percentage` | `line` | optional V2 template | `datastore_cluster_space_utilization_used=>used div=100` |
| Datastore cluster | optional charted only when `collect_datastore_clusters` is enabled and the StoragePod matches `datastore_cluster_include` | `datastore_cluster_space_usage` | `vsphere.datastore_cluster_space_usage` | `datastore clusters space` | `bytes` | `line` | optional V2 template | `datastore_cluster_space_usage_capacity=>capacity`; `datastore_cluster_space_usage_free=>free`; `datastore_cluster_space_usage_used=>used` |
| Datastore cluster | optional charted only when `collect_datastore_clusters` is enabled and the StoragePod matches `datastore_cluster_include` | `datastore_cluster_storage_drs_status` | `vsphere.datastore_cluster_storage_drs_status` | `datastore clusters status` | `status` | `line` | optional V2 template | `datastore_cluster_storage_drs_status_enabled=>enabled`; `datastore_cluster_storage_drs_status_disabled=>disabled` |
| vSAN cluster | optional charted only when `collect_vsan` is enabled and vSAN space usage is returned for a vSAN-enabled cluster | `vsan_cluster_space_utilization` | `vsphere.vsan_cluster_space_utilization` | `vSAN clusters space` | `percentage` | `line` | optional V2 template | `vsan_cluster_space_utilization_used=>used div=100` |
| vSAN cluster | optional charted only when `collect_vsan` is enabled and vSAN space usage is returned for a vSAN-enabled cluster | `vsan_cluster_space_usage` | `vsphere.vsan_cluster_space_usage` | `vSAN clusters space` | `bytes` | `stacked` | optional V2 template | `vsan_cluster_space_usage_used=>used`; `vsan_cluster_space_usage_free=>free`; `vsan_cluster_space_usage_total=>total hidden` |
| vSAN cluster | optional charted only when `collect_vsan` is enabled and vSAN health is returned for a vSAN-enabled cluster | `vsan_cluster_health_status` | `vsphere.vsan_cluster_health_status` | `vSAN clusters space` | `status` | `line` | optional V2 template | `vsan_cluster_health_status_green=>green`; `vsan_cluster_health_status_yellow=>yellow`; `vsan_cluster_health_status_red=>red`; `vsan_cluster_health_status_unknown=>unknown` |
| vSAN cluster performance | optional charted only when `collect_vsan` is enabled and vSAN cluster performance is returned | `vsan_cluster_operations`; `vsan_cluster_throughput`; `vsan_cluster_latency`; `vsan_cluster_congestions` | `vsphere.vsan_cluster_operations`; `vsphere.vsan_cluster_throughput`; `vsphere.vsan_cluster_latency`; `vsphere.vsan_cluster_congestions` | `vSAN clusters performance` | `operations/s`; `bytes/s`; `microseconds`; `congestions/s` | `line`/`area` | optional V2 template | read/write operations, read/write throughput, read/write latency, congestions |
| vSAN host performance | optional charted only when `collect_vsan` is enabled and vSAN host performance is returned | `vsan_host_operations`; `vsan_host_throughput`; `vsan_host_latency`; `vsan_host_congestions`; `vsan_host_cache_hit_rate` | `vsphere.vsan_host_operations`; `vsphere.vsan_host_throughput`; `vsphere.vsan_host_latency`; `vsphere.vsan_host_congestions`; `vsphere.vsan_host_cache_hit_rate` | `vSAN hosts performance` | `operations/s`; `bytes/s`; `microseconds`; `congestions/s`; `percentage` | `line`/`area` | optional V2 template | read/write operations, read/write throughput, read/write latency, congestions, hit_rate |
| vSAN VM performance | optional charted only when `collect_vsan` is enabled and vSAN VM performance is returned | `vsan_vm_operations`; `vsan_vm_throughput`; `vsan_vm_latency` | `vsphere.vsan_vm_operations`; `vsphere.vsan_vm_throughput`; `vsphere.vsan_vm_latency` | `vSAN VMs performance` | `operations/s`; `bytes/s`; `microseconds` | `line`/`area` | optional V2 template | read/write operations, read/write throughput, read/write latency |
| VM virtual disk | optional charted only when `collect_vm_disks` is enabled and the disk matches `vm_disk_include` | `vm_disk_capacity` | `vsphere.vm_disk_capacity` | `vms disk` | `bytes` | `line` | optional V2 template | `vm_disk_capacity_capacity=>capacity` |
| VM virtual disk performance | optional charted only when `collect_vm_disk_performance` is enabled and the returned vSphere performance instance matches `vm_disk_include` | `vm_disk_device_io` | `vsphere.vm_disk_device_io` | `vms disk devices` | `KiB/s` | `area` | optional V2 template | `vm_disk_device_io_read=>read`; `vm_disk_device_io_write=>write mul=-1` |
| VM virtual disk performance | optional charted only when `collect_vm_disk_performance` is enabled and the returned vSphere performance instance matches `vm_disk_include` | `vm_disk_device_iops` | `vsphere.vm_disk_device_iops` | `vms disk devices` | `operations/s` | `line` | optional V2 template | `vm_disk_device_iops_read=>read`; `vm_disk_device_iops_write=>write mul=-1` |
| VM virtual disk performance | optional charted only when `collect_vm_disk_performance` is enabled and the returned vSphere performance instance matches `vm_disk_include` | `vm_disk_device_latency` | `vsphere.vm_disk_device_latency` | `vms disk devices` | `milliseconds` | `line` | optional V2 template | `vm_disk_device_latency_read=>read`; `vm_disk_device_latency_write=>write` |
| VM virtual disk performance | optional charted only when `collect_vm_disk_performance` is enabled and the returned vSphere performance instance matches `vm_disk_include` | `vm_disk_device_outstanding_io` | `vsphere.vm_disk_device_outstanding_io` | `vms disk devices` | `operations` | `line` | optional V2 template | `vm_disk_device_outstanding_io_read=>read`; `vm_disk_device_outstanding_io_write=>write mul=-1` |
| VM network interface performance | optional charted only when `collect_vm_nic_performance` is enabled and the returned vSphere performance instance matches `vm_nic_include` | `vm_net_interface_traffic` | `vsphere.vm_net_interface_traffic` | `vms network interfaces` | `KiB/s` | `area` | optional V2 template | `vm_net_interface_traffic_received=>received`; `vm_net_interface_traffic_sent=>sent mul=-1` |
| VM network interface performance | optional charted only when `collect_vm_nic_performance` is enabled and the returned vSphere performance instance matches `vm_nic_include` | `vm_net_interface_packets` | `vsphere.vm_net_interface_packets` | `vms network interfaces` | `packets` | `line` | optional V2 template | `vm_net_interface_packets_received=>received`; `vm_net_interface_packets_sent=>sent mul=-1` |
| VM network interface performance | optional charted only when `collect_vm_nic_performance` is enabled and the returned vSphere performance instance matches `vm_nic_include` | `vm_net_interface_drops` | `vsphere.vm_net_interface_drops` | `vms network interfaces` | `drops` | `line` | optional V2 template | `vm_net_interface_drops_received=>received`; `vm_net_interface_drops_sent=>sent mul=-1` |
| VM network interface performance | optional charted only when `collect_vm_nic_performance` is enabled and the returned vSphere performance instance matches `vm_nic_include` | `vm_net_interface_broadcast_packets` | `vsphere.vm_net_interface_broadcast_packets` | `vms network interfaces` | `packets` | `line` | optional V2 template | `vm_net_interface_broadcast_packets_received=>received`; `vm_net_interface_broadcast_packets_sent=>sent mul=-1` |
| VM network interface performance | optional charted only when `collect_vm_nic_performance` is enabled and the returned vSphere performance instance matches `vm_nic_include` | `vm_net_interface_multicast_packets` | `vsphere.vm_net_interface_multicast_packets` | `vms network interfaces` | `packets` | `line` | optional V2 template | `vm_net_interface_multicast_packets_received=>received`; `vm_net_interface_multicast_packets_sent=>sent mul=-1` |
| Host physical network interface performance | optional charted only when `collect_host_nic_performance` is enabled and the returned vSphere performance instance matches `host_nic_include` | `host_net_interface_traffic` | `vsphere.host_net_interface_traffic` | `hosts network interfaces` | `KiB/s` | `area` | optional V2 template | `host_net_interface_traffic_received=>received`; `host_net_interface_traffic_sent=>sent mul=-1` |
| Host physical network interface performance | optional charted only when `collect_host_nic_performance` is enabled and the returned vSphere performance instance matches `host_nic_include` | `host_net_interface_packets` | `vsphere.host_net_interface_packets` | `hosts network interfaces` | `packets` | `line` | optional V2 template | `host_net_interface_packets_received=>received`; `host_net_interface_packets_sent=>sent mul=-1` |
| Host physical network interface performance | optional charted only when `collect_host_nic_performance` is enabled and the returned vSphere performance instance matches `host_nic_include` | `host_net_interface_drops` | `vsphere.host_net_interface_drops` | `hosts network interfaces` | `drops` | `line` | optional V2 template | `host_net_interface_drops_received=>received`; `host_net_interface_drops_sent=>sent mul=-1` |
| Host physical network interface performance | optional charted only when `collect_host_nic_performance` is enabled and the returned vSphere performance instance matches `host_nic_include` | `host_net_interface_errors` | `vsphere.host_net_interface_errors` | `hosts network interfaces` | `errors` | `line` | optional V2 template | `host_net_interface_errors_received=>received`; `host_net_interface_errors_sent=>sent mul=-1` |
| Host physical network interface performance | optional charted only when `collect_host_nic_performance` is enabled and the returned vSphere performance instance matches `host_nic_include` | `host_net_interface_broadcast_packets` | `vsphere.host_net_interface_broadcast_packets` | `hosts network interfaces` | `packets` | `line` | optional V2 template | `host_net_interface_broadcast_packets_received=>received`; `host_net_interface_broadcast_packets_sent=>sent mul=-1` |
| Host physical network interface performance | optional charted only when `collect_host_nic_performance` is enabled and the returned vSphere performance instance matches `host_nic_include` | `host_net_interface_multicast_packets` | `vsphere.host_net_interface_multicast_packets` | `hosts network interfaces` | `packets` | `line` | optional V2 template | `host_net_interface_multicast_packets_received=>received`; `host_net_interface_multicast_packets_sent=>sent mul=-1` |
| Host physical network interface performance | optional charted only when `collect_host_nic_performance` is enabled and the returned vSphere performance instance matches `host_nic_include` | `host_net_interface_unknown_protocol_frames` | `vsphere.host_net_interface_unknown_protocol_frames` | `hosts network interfaces` | `frames` | `line` | optional V2 template | `host_net_interface_unknown_protocol_frames_unknown=>unknown` |
| Host physical network interface performance | optional charted only when `collect_host_nic_performance` is enabled and the returned vSphere performance instance matches `host_nic_include` | `host_net_interface_usage` | `vsphere.host_net_interface_usage` | `hosts network interfaces` | `KiB/s` | `line` | optional V2 template | `host_net_interface_usage_usage=>usage` |
| Host disk/LUN/device performance | optional charted only when `collect_host_disk_performance` is enabled and the returned vSphere performance instance matches `host_disk_include` | `host_disk_device_io` | `vsphere.host_disk_device_io` | `hosts disk devices` | `KiB/s` | `area` | optional V2 template | `host_disk_device_io_read=>read`; `host_disk_device_io_write=>write mul=-1` |
| Host disk/LUN/device performance | optional charted only when `collect_host_disk_performance` is enabled and the returned vSphere performance instance matches `host_disk_include` | `host_disk_device_iops` | `vsphere.host_disk_device_iops` | `hosts disk devices` | `operations/s` | `line` | optional V2 template | `host_disk_device_iops_read=>read`; `host_disk_device_iops_write=>write mul=-1` |
| Host disk/LUN/device performance | optional charted only when `collect_host_disk_performance` is enabled and the returned vSphere performance instance matches `host_disk_include` | `host_disk_device_requests` | `vsphere.host_disk_device_requests` | `hosts disk devices` | `requests` | `line` | optional V2 template | `host_disk_device_requests_read=>read`; `host_disk_device_requests_write=>write mul=-1` |
| Host disk/LUN/device performance | optional charted only when `collect_host_disk_performance` is enabled and the returned vSphere performance instance matches `host_disk_include` | `host_disk_device_latency` | `vsphere.host_disk_device_latency` | `hosts disk devices` | `milliseconds` | `line` | optional V2 template | `host_disk_device_latency_total=>total`; `host_disk_device_latency_read=>read`; `host_disk_device_latency_write=>write` |
| Host disk/LUN/device performance | optional charted only when `collect_host_disk_performance` is enabled and the returned vSphere performance instance matches `host_disk_include` | `host_disk_device_latency_breakdown` | `vsphere.host_disk_device_latency_breakdown` | `hosts disk devices` | `milliseconds` | `line` | optional V2 template | `host_disk_device_latency_breakdown_device=>device`; `host_disk_device_latency_breakdown_kernel=>kernel`; `host_disk_device_latency_breakdown_queue=>queue` |
| Host disk/LUN/device performance | optional charted only when `collect_host_disk_performance` is enabled and the returned vSphere performance instance matches `host_disk_include` | `host_disk_device_read_latency_breakdown` | `vsphere.host_disk_device_read_latency_breakdown` | `hosts disk devices` | `milliseconds` | `line` | optional V2 template | `host_disk_device_read_latency_breakdown_device=>device`; `host_disk_device_read_latency_breakdown_kernel=>kernel`; `host_disk_device_read_latency_breakdown_queue=>queue` |
| Host disk/LUN/device performance | optional charted only when `collect_host_disk_performance` is enabled and the returned vSphere performance instance matches `host_disk_include` | `host_disk_device_write_latency_breakdown` | `vsphere.host_disk_device_write_latency_breakdown` | `hosts disk devices` | `milliseconds` | `line` | optional V2 template | `host_disk_device_write_latency_breakdown_device=>device`; `host_disk_device_write_latency_breakdown_kernel=>kernel`; `host_disk_device_write_latency_breakdown_queue=>queue` |
| Host disk/LUN/device performance | optional charted only when `collect_host_disk_performance` is enabled and the returned vSphere performance instance matches `host_disk_include` | `host_disk_device_commands` | `vsphere.host_disk_device_commands` | `hosts disk devices` | `commands/s` | `line` | optional V2 template | `host_disk_device_commands_issued=>issued` |
| Host disk/LUN/device performance | optional charted only when `collect_host_disk_performance` is enabled and the returned vSphere performance instance matches `host_disk_include` | `host_disk_device_command_events` | `vsphere.host_disk_device_command_events` | `hosts disk devices` | `events` | `line` | optional V2 template | `host_disk_device_command_events_issued=>issued`; `host_disk_device_command_events_aborted=>aborted`; `host_disk_device_command_events_bus_resets=>bus_resets` |
| Host disk/LUN/device performance | optional charted only when `collect_host_disk_performance` is enabled and the returned vSphere performance instance matches `host_disk_include` | `host_disk_device_queue_depth` | `vsphere.host_disk_device_queue_depth` | `hosts disk devices` | `queue depth` | `line` | optional V2 template | `host_disk_device_queue_depth_max=>max` |
| Host disk/LUN/device performance | optional charted only when `collect_host_disk_performance` is enabled and the returned vSphere performance instance matches `host_disk_include` | `host_disk_device_scsi_reservation_conflicts` | `vsphere.host_disk_device_scsi_reservation_conflicts` | `hosts disk devices` | `conflicts` | `line` | optional V2 template | `host_disk_device_scsi_reservation_conflicts_conflicts=>conflicts` |
| Host disk/LUN/device performance | optional charted only when `collect_host_disk_performance` is enabled and the returned vSphere performance instance matches `host_disk_include` | `host_disk_device_scsi_reservation_conflicts_percentage` | `vsphere.host_disk_device_scsi_reservation_conflicts_percentage` | `hosts disk devices` | `percentage` | `line` | optional V2 template | `host_disk_device_scsi_reservation_conflicts_percentage_conflicts=>conflicts` |
| Host storage adapter performance | optional charted only when `collect_host_storage_adapter_performance` is enabled and the returned vSphere performance instance matches `host_storage_adapter_include` | `host_storage_adapter_io` | `vsphere.host_storage_adapter_io` | `hosts storage adapters` | `KiB/s` | `area` | optional V2 template | `host_storage_adapter_io_read=>read`; `host_storage_adapter_io_write=>write mul=-1` |
| Host storage adapter performance | optional charted only when `collect_host_storage_adapter_performance` is enabled and the returned vSphere performance instance matches `host_storage_adapter_include` | `host_storage_adapter_commands` | `vsphere.host_storage_adapter_commands` | `hosts storage adapters` | `commands/s` | `line` | optional V2 template | `host_storage_adapter_commands_issued=>issued`; `host_storage_adapter_commands_read=>read`; `host_storage_adapter_commands_write=>write mul=-1` |
| Host storage adapter performance | optional charted only when `collect_host_storage_adapter_performance` is enabled and the returned vSphere performance instance matches `host_storage_adapter_include` | `host_storage_adapter_latency` | `vsphere.host_storage_adapter_latency` | `hosts storage adapters` | `milliseconds` | `line` | optional V2 template | `host_storage_adapter_latency_read=>read`; `host_storage_adapter_latency_write=>write`; `host_storage_adapter_latency_queue=>queue` |
| Host storage adapter performance | optional charted only when `collect_host_storage_adapter_performance` is enabled and the returned vSphere performance instance matches `host_storage_adapter_include` | `host_storage_adapter_queue` | `vsphere.host_storage_adapter_queue` | `hosts storage adapters` | `I/O requests` | `line` | optional V2 template | `host_storage_adapter_queue_outstanding=>outstanding`; `host_storage_adapter_queue_queued=>queued`; `host_storage_adapter_queue_depth=>depth` |
| Host storage adapter performance | optional charted only when `collect_host_storage_adapter_performance` is enabled and the returned vSphere performance instance matches `host_storage_adapter_include` | `host_storage_adapter_outstanding_io_percentage` | `vsphere.host_storage_adapter_outstanding_io_percentage` | `hosts storage adapters` | `percentage` | `line` | optional V2 template | `host_storage_adapter_outstanding_io_percentage_outstanding=>outstanding` |
| Host storage adapter performance | optional charted only when `collect_host_storage_adapter_performance` is enabled and the returned vSphere performance instance matches `host_storage_adapter_include` | `host_storage_adapter_throughput` | `vsphere.host_storage_adapter_throughput` | `hosts storage adapters` | `KiB/s` | `line` | optional V2 template | `host_storage_adapter_throughput_usage=>usage` |
| Host storage adapter performance | optional charted only when `collect_host_storage_adapter_performance` is enabled and the returned vSphere performance instance matches `host_storage_adapter_include` | `host_storage_adapter_throughput_contention` | `vsphere.host_storage_adapter_throughput_contention` | `hosts storage adapters` | `milliseconds` | `line` | optional V2 template | `host_storage_adapter_throughput_contention_contention=>contention` |
| Host storage adapter aggregate performance | optional charted only when `collect_host_storage_adapter_performance` is enabled and the aggregate `storageAdapter.maxTotalLatency.latest` counter is returned | `host_storage_adapter_max_latency` | `vsphere.host_storage_adapter_max_latency` | `hosts storage adapters` | `milliseconds` | `line` | optional V2 template | `host_storage_adapter_max_latency_max=>max` |
| Host storage path performance | optional charted only when `collect_host_storage_path_performance` is enabled and the returned vSphere performance instance matches `host_storage_path_include` | `host_storage_path_io` | `vsphere.host_storage_path_io` | `hosts storage paths` | `KiB/s` | `area` | optional V2 template | `host_storage_path_io_read=>read`; `host_storage_path_io_write=>write mul=-1` |
| Host storage path performance | optional charted only when `collect_host_storage_path_performance` is enabled and the returned vSphere performance instance matches `host_storage_path_include` | `host_storage_path_commands` | `vsphere.host_storage_path_commands` | `hosts storage paths` | `commands/s` | `line` | optional V2 template | `host_storage_path_commands_issued=>issued`; `host_storage_path_commands_read=>read`; `host_storage_path_commands_write=>write mul=-1` |
| Host storage path performance | optional charted only when `collect_host_storage_path_performance` is enabled and the returned vSphere performance instance matches `host_storage_path_include` | `host_storage_path_latency` | `vsphere.host_storage_path_latency` | `hosts storage paths` | `milliseconds` | `line` | optional V2 template | `host_storage_path_latency_read=>read`; `host_storage_path_latency_write=>write` |
| Host storage path performance | optional charted only when `collect_host_storage_path_performance` is enabled and the returned vSphere performance instance matches `host_storage_path_include` | `host_storage_path_command_events` | `vsphere.host_storage_path_command_events` | `hosts storage paths` | `events` | `line` | optional V2 template | `host_storage_path_command_events_aborted=>aborted`; `host_storage_path_command_events_bus_resets=>bus_resets` |
| Host storage path performance | optional charted only when `collect_host_storage_path_performance` is enabled and the returned vSphere performance instance matches `host_storage_path_include` | `host_storage_path_throughput` | `vsphere.host_storage_path_throughput` | `hosts storage paths` | `KiB/s` | `line` | optional V2 template | `host_storage_path_throughput_usage=>usage` |
| Host storage path performance | optional charted only when `collect_host_storage_path_performance` is enabled and the returned vSphere performance instance matches `host_storage_path_include` | `host_storage_path_throughput_contention` | `vsphere.host_storage_path_throughput_contention` | `hosts storage paths` | `milliseconds` | `line` | optional V2 template | `host_storage_path_throughput_contention_contention=>contention` |
| Host storage path aggregate performance | optional charted only when `collect_host_storage_path_performance` is enabled and the aggregate `storagePath.maxTotalLatency.latest` counter is returned | `host_storage_path_max_latency` | `vsphere.host_storage_path_max_latency` | `hosts storage paths` | `milliseconds` | `line` | optional V2 template | `host_storage_path_max_latency_max=>max` |
| Host CPU instance performance | optional charted only when `collect_host_cpu_instance_performance` is enabled and the returned vSphere performance instance matches `host_cpu_instance_include` | `host_cpu_instance_utilization` | `vsphere.host_cpu_instance_utilization` | `hosts cpu instances` | `percentage` | `line` | optional V2 template | `host_cpu_instance_utilization_usage=>usage div=100`; `host_cpu_instance_utilization_utilization=>utilization div=100`; `host_cpu_instance_utilization_core=>core_utilization div=100` |
| Host CPU instance performance | optional charted only when `collect_host_cpu_instance_performance` is enabled and the returned vSphere performance instance matches `host_cpu_instance_include` | `host_cpu_instance_time` | `vsphere.host_cpu_instance_time` | `hosts cpu instances` | `milliseconds` | `line` | optional V2 template | `host_cpu_instance_time_used=>used`; `host_cpu_instance_time_idle=>idle` |
| VM | property/perf charted when VM is discovered | `%s_cpu_utilization` | `vsphere.vm_cpu_utilization` | `vms cpu` | `percentage` | `line` | `prioVMCPUUtilization` | `%s_cpu.usage.average=>used div=100` |
| VM | property/perf charted when VM is discovered | `%s_mem_utilization` | `vsphere.vm_mem_utilization` | `vms mem` | `percentage` | `line` | `prioVmMemoryUtilization` | `%s_mem.usage.average=>used div=100` |
| VM | property/perf charted when VM is discovered | `%s_mem_usage` | `vsphere.vm_mem_usage` | `vms mem` | `KiB` | `line` | `prioVmMemoryUsage` | `%s_mem.granted.average=>granted`; `%s_mem.consumed.average=>consumed`; `%s_mem.active.average=>active`; `%s_mem.shared.average=>shared` |
| VM | property/perf charted when VM is discovered | `%s_mem_swap_usage` | `vsphere.vm_mem_swap_usage` | `vms mem` | `KiB` | `line` | `prioVmMemorySwapUsage` | `%s_mem.swapped.average=>swapped` |
| VM | property/perf charted when VM is discovered | `%s_mem_swap_io_rate` | `vsphere.vm_mem_swap_io` | `vms mem` | `KiB/s` | `area` | `prioVmMemorySwapIO` | `%s_mem.swapinRate.average=>in`; `%s_mem.swapoutRate.average=>out` |
| VM | property/perf charted when VM is discovered | `%s_disk_io` | `vsphere.vm_disk_io` | `vms disk` | `KiB/s` | `area` | `prioVmDiskIO` | `%s_disk.read.average=>read`; `%s_disk.write.average=>write mul=-1` |
| VM | property/perf charted when VM is discovered | `%s_disk_max_latency` | `vsphere.vm_disk_max_latency` | `vms disk` | `milliseconds` | `line` | `prioVmDiskMaxLatency` | `%s_disk.maxTotalLatency.latest=>latency` |
| VM | property/perf charted when VM is discovered | `%s_net_traffic` | `vsphere.vm_net_traffic` | `vms net` | `KiB/s` | `area` | `prioVmNetworkTraffic` | `%s_net.bytesRx.average=>received`; `%s_net.bytesTx.average=>sent mul=-1` |
| VM | property/perf charted when VM is discovered | `%s_net_packets` | `vsphere.vm_net_packets` | `vms net` | `packets` | `line` | `prioVmNetworkPackets` | `%s_net.packetsRx.summation=>received`; `%s_net.packetsTx.summation=>sent mul=-1` |
| VM | property/perf charted when VM is discovered | `%s_net_drops` | `vsphere.vm_net_drops` | `vms net` | `drops` | `line` | `prioVmNetworkDrops` | `%s_net.droppedRx.summation=>received`; `%s_net.droppedTx.summation=>sent mul=-1` |
| VM | property/perf charted when VM is discovered | `%s_overall_status` | `vsphere.vm_overall_status` | `vms status` | `status` | `line` | `prioVmOverallStatus` | `%s_overall.status.green=>green`; `%s_overall.status.red=>red`; `%s_overall.status.yellow=>yellow`; `%s_overall.status.gray=>gray` |
| VM | property charted when VM is discovered | `%s_power_state` | `vsphere.vm_power_state` | `vms status` | `status` | `line` | `prioVMPowerState` | `%s_power_state.poweredOn=>powered_on`; `%s_power_state.poweredOff=>powered_off`; `%s_power_state.suspended=>suspended` |
| VM | property charted when VM is discovered | `%s_connection_state` | `vsphere.vm_connection_state` | `vms status` | `status` | `line` | `prioVMConnectionState` | `%s_connection_state.connected=>connected`; `%s_connection_state.disconnected=>disconnected`; `%s_connection_state.orphaned=>orphaned`; `%s_connection_state.inaccessible=>inaccessible`; `%s_connection_state.invalid=>invalid` |
| VM | property charted when VM is discovered | `%s_tools_running_status` | `vsphere.vm_tools_running_status` | `vms status` | `status` | `line` | `prioVMToolsRunningStatus` | `%s_tools_running_status.running=>running`; `%s_tools_running_status.notRunning=>not_running`; `%s_tools_running_status.executingScripts=>executing_scripts`; `%s_tools_running_status.unknown=>unknown` |
| VM | property charted when VM is discovered | `%s_tools_version_status` | `vsphere.vm_tools_version_status` | `vms status` | `status` | `line` | `prioVMToolsVersionStatus` | `%s_tools_version_status.current=>current`; `%s_tools_version_status.needUpgrade=>need_upgrade`; `%s_tools_version_status.notInstalled=>not_installed`; `%s_tools_version_status.unmanaged=>unmanaged`; `%s_tools_version_status.tooOld=>too_old`; `%s_tools_version_status.supportedOld=>supported_old`; `%s_tools_version_status.supportedNew=>supported_new`; `%s_tools_version_status.tooNew=>too_new`; `%s_tools_version_status.blacklisted=>blacklisted`; `%s_tools_version_status.unknown=>unknown` |
| VM | property charted when VM is discovered | `%s_consolidation_needed` | `vsphere.vm_consolidation_needed` | `vms status` | `status` | `line` | `prioVMConsolidationNeeded` | `%s_consolidation_needed.needed=>needed`; `%s_consolidation_needed.notNeeded=>not_needed` |
| VM | property/perf charted when VM is discovered | `%s_system_uptime` | `vsphere.vm_system_uptime` | `vms uptime` | `seconds` | `line` | `prioVmSystemUptime` | `%s_sys.uptime.latest=>uptime` |
| VM | property charted when VM is discovered | `%s_config_cpu` | `vsphere.vm_config_cpu` | `vms config` | `vCPUs` | `line` | `prioVMConfigCPU` | `%s_config_cpu=>vcpus` |
| VM | property charted when VM is discovered | `%s_config_memory` | `vsphere.vm_config_memory` | `vms config` | `MiB` | `line` | `prioVMConfigMemory` | `%s_config_memory=>memory` |
| VM | property charted when VM is discovered | `%s_config_devices` | `vsphere.vm_config_devices` | `vms config` | `devices` | `line` | `prioVMConfigDevices` | `%s_config_devices.disks=>disks`; `%s_config_devices.nics=>nics` |
| VM | property charted when VM is discovered | `%s_storage_usage` | `vsphere.vm_storage_usage` | `vms storage` | `bytes` | `line` | `prioVMStorageUsage` | `%s_storage.committed=>committed`; `%s_storage.uncommitted=>uncommitted`; `%s_storage.unshared=>unshared` |
| VM | snapshot property charted when VM is discovered | `%s_snapshot_count` | `vsphere.vm_snapshot_count` | `vms snapshots` | `snapshots` | `line` | `prioVMSnapshotCount` | `%s_snapshot_count=>count` |
| VM | snapshot property charted when VM is discovered | `%s_snapshot_max_age` | `vsphere.vm_snapshot_max_age` | `vms snapshots` | `seconds` | `line` | `prioVMSnapshotAge` | `%s_snapshot_max_age=>age` |
| VM | snapshot property charted when VM is discovered | `%s_snapshot_max_chain_depth` | `vsphere.vm_snapshot_max_chain_depth` | `vms snapshots` | `snapshots` | `line` | `prioVMSnapshotChainDepth` | `%s_snapshot_max_chain_depth=>depth` |
| Host | property/perf charted when host is discovered | `%s_cpu_usage_total` | `vsphere.host_cpu_utilization` | `hosts cpu` | `percentage` | `line` | `prioHostCPUUtilization` | `%s_cpu.usage.average=>used div=100` |
| Host | property/perf charted when host is discovered | `%s_mem_utilization` | `vsphere.host_mem_utilization` | `hosts mem` | `percentage` | `line` | `prioHostMemoryUtilization` | `%s_mem.usage.average=>used div=100` |
| Host | property/perf charted when host is discovered | `%s_mem_usage` | `vsphere.host_mem_usage` | `hosts mem` | `KiB` | `line` | `prioHostMemoryUsage` | `%s_mem.granted.average=>granted`; `%s_mem.consumed.average=>consumed`; `%s_mem.active.average=>active`; `%s_mem.shared.average=>shared`; `%s_mem.sharedcommon.average=>sharedcommon` |
| Host | property/perf charted when host is discovered | `%s_mem_swap_rate` | `vsphere.host_mem_swap_io` | `hosts mem` | `KiB/s` | `area` | `prioHostMemorySwapIO` | `%s_mem.swapinRate.average=>in`; `%s_mem.swapoutRate.average=>out` |
| Host | property/perf charted when host is discovered | `%s_disk_io` | `vsphere.host_disk_io` | `hosts disk` | `KiB/s` | `area` | `prioHostDiskIO` | `%s_disk.read.average=>read`; `%s_disk.write.average=>write mul=-1` |
| Host | property/perf charted when host is discovered | `%s_disk_max_latency` | `vsphere.host_disk_max_latency` | `hosts disk` | `milliseconds` | `line` | `prioHostDiskMaxLatency` | `%s_disk.maxTotalLatency.latest=>latency` |
| Host | property/perf charted when host is discovered | `%s_net_traffic` | `vsphere.host_net_traffic` | `hosts net` | `KiB/s` | `area` | `prioHostNetworkTraffic` | `%s_net.bytesRx.average=>received`; `%s_net.bytesTx.average=>sent mul=-1` |
| Host | property/perf charted when host is discovered | `%s_net_packets` | `vsphere.host_net_packets` | `hosts net` | `packets` | `line` | `prioHostNetworkPackets` | `%s_net.packetsRx.summation=>received`; `%s_net.packetsTx.summation=>sent mul=-1` |
| Host | property/perf charted when host is discovered | `%s_net_drops_total` | `vsphere.host_net_drops` | `hosts net` | `drops` | `line` | `prioHostNetworkDrops` | `%s_net.droppedRx.summation=>received`; `%s_net.droppedTx.summation=>sent mul=-1` |
| Host | property/perf charted when host is discovered | `%s_net_errors` | `vsphere.host_net_errors` | `hosts net` | `errors` | `line` | `prioHostNetworkErrors` | `%s_net.errorsRx.summation=>received`; `%s_net.errorsTx.summation=>sent mul=-1` |
| Host | property/perf charted when host is discovered | `%s_overall_status` | `vsphere.host_overall_status` | `hosts status` | `status` | `line` | `prioHostOverallStatus` | `%s_overall.status.green=>green`; `%s_overall.status.red=>red`; `%s_overall.status.yellow=>yellow`; `%s_overall.status.gray=>gray` |
| Host | property charted when host is discovered | `%s_power_state` | `vsphere.host_power_state` | `hosts status` | `status` | `line` | `prioHostPowerState` | `%s_power_state.poweredOn=>powered_on`; `%s_power_state.poweredOff=>powered_off`; `%s_power_state.standBy=>standby`; `%s_power_state.unknown=>unknown` |
| Host | property charted when host is discovered | `%s_connection_state` | `vsphere.host_connection_state` | `hosts status` | `status` | `line` | `prioHostConnectionState` | `%s_connection_state.connected=>connected`; `%s_connection_state.notResponding=>not_responding`; `%s_connection_state.disconnected=>disconnected` |
| Host | property charted when host is discovered | `%s_maintenance_status` | `vsphere.host_maintenance_status` | `hosts status` | `status` | `line` | `prioHostMaintenanceStatus` | `%s_maintenance_status.normal=>normal`; `%s_maintenance_status.inMaintenance=>in_maintenance` |
| Host | property/perf charted when host is discovered | `%s_system_uptime` | `vsphere.host_system_uptime` | `hosts uptime` | `seconds` | `line` | `prioHostSystemUptime` | `%s_sys.uptime.latest=>uptime` |
| Datastore | property charted when datastore is discovered; perf charts later only after perf data arrives | `%s_space_utilization` | `vsphere.datastore_space_utilization` | `datastores space` | `percentage` | `line` | `prioDatastoreSpaceUtilization` | `%s_used_space_pct=>used div=100` |
| Datastore | property charted when datastore is discovered; perf charts later only after perf data arrives | `%s_space_usage` | `vsphere.datastore_space_usage` | `datastores space` | `bytes` | `line` | `prioDatastoreSpaceUsage` | `%s_capacity=>capacity`; `%s_free_space=>free`; `%s_used_space=>used`; `%s_uncommitted=>uncommitted` |
| Datastore | property charted when datastore is discovered; perf charts later only after perf data arrives | `%s_overall_status` | `vsphere.datastore_overall_status` | `datastores status` | `status` | `line` | `prioDatastoreOverallStatus` | `%s_overall.status.green=>green`; `%s_overall.status.red=>red`; `%s_overall.status.yellow=>yellow`; `%s_overall.status.gray=>gray` |
| Datastore | perf chart created only after datastore perf data arrives | `%s_disk_io` | `vsphere.datastore_disk_io` | `datastores disk` | `KiB/s` | `area` | `prioDatastoreDiskIO` | `%s_datastore.read.average=>read`; `%s_datastore.write.average=>write mul=-1` |
| Datastore | perf chart created only after datastore perf data arrives | `%s_disk_iops` | `vsphere.datastore_disk_iops` | `datastores disk` | `operations/s` | `line` | `prioDatastoreDiskIOPS` | `%s_datastore.numberReadAveraged.average=>reads`; `%s_datastore.numberWriteAveraged.average=>writes mul=-1` |
| Datastore | perf chart created only after datastore perf data arrives | `%s_disk_latency` | `vsphere.datastore_disk_latency` | `datastores disk` | `milliseconds` | `line` | `prioDatastoreDiskLatency` | `%s_datastore.totalReadLatency.average=>read`; `%s_datastore.totalWriteLatency.average=>write` |
| Cluster | property chart created when cluster properties refresh | `%s_hosts` | `vsphere.cluster_hosts` | `clusters hosts` | `hosts` | `line` | `prioClusterHosts` | `%s_num_hosts=>total`; `%s_num_effective_hosts=>effective` |
| Cluster | property chart created when cluster properties refresh | `%s_cpu_capacity` | `vsphere.cluster_cpu_capacity` | `clusters cpu` | `MHz` | `line` | `prioClusterCPUCapacity` | `%s_total_cpu=>total`; `%s_effective_cpu=>effective` |
| Cluster | property chart created when cluster properties refresh | `%s_mem_capacity` | `vsphere.cluster_mem_capacity` | `clusters mem` | `bytes` | `line` | `prioClusterMemCapacity` | `%s_total_memory=>total`; `%s_effective_memory=>effective` |
| Cluster | property chart created when cluster properties refresh | `%s_cpu_topology` | `vsphere.cluster_cpu_topology` | `clusters cpu` | `count` | `line` | `prioClusterCPUTopology` | `%s_num_cpu_cores=>cores`; `%s_num_cpu_threads=>threads` |
| Cluster | property chart created when cluster properties refresh | `%s_drs_config` | `vsphere.cluster_drs_config` | `clusters config` | `status` | `line` | `prioClusterDRSConfig` | `%s_drs_enabled=>enabled` |
| Cluster | property chart created when cluster properties refresh | `%s_drs_mode` | `vsphere.cluster_drs_mode` | `clusters config` | `status` | `line` | `prioClusterDRSMode` | `%s_drs_mode.manual=>manual`; `%s_drs_mode.partiallyAutomated=>partially_automated`; `%s_drs_mode.fullyAutomated=>fully_automated`; `%s_drs_mode.unknown=>unknown` |
| Cluster | property chart created when cluster properties refresh | `%s_drs_vmotion_rate` | `vsphere.cluster_drs_vmotion_rate` | `clusters config` | `level` | `line` | `prioClusterDRSVmotionRate` | `%s_drs_vmotion_rate=>rate` |
| Cluster | property chart created when cluster properties refresh | `%s_ha_config` | `vsphere.cluster_ha_config` | `clusters config` | `status` | `line` | `prioClusterHAConfig` | `%s_ha_enabled=>enabled`; `%s_ha_adm_ctrl_enabled=>admission_control` |
| Cluster | property chart created when cluster properties refresh | `%s_ha_host_monitoring` | `vsphere.cluster_ha_host_monitoring` | `clusters config` | `status` | `line` | `prioClusterHAHostMonitoring` | `%s_ha_host_monitoring.enabled=>enabled`; `%s_ha_host_monitoring.disabled=>disabled`; `%s_ha_host_monitoring.unknown=>unknown` |
| Cluster | property chart created when cluster properties refresh | `%s_ha_vm_monitoring` | `vsphere.cluster_ha_vm_monitoring` | `clusters config` | `status` | `line` | `prioClusterHAVMMonitoring` | `%s_ha_vm_monitoring.vmMonitoringDisabled=>disabled`; `%s_ha_vm_monitoring.vmMonitoringOnly=>vm_monitoring_only`; `%s_ha_vm_monitoring.vmAndAppMonitoring=>vm_and_app_monitoring`; `%s_ha_vm_monitoring.unknown=>unknown` |
| Cluster | property chart created when cluster properties refresh | `%s_ha_vm_component_protection` | `vsphere.cluster_ha_vm_component_protection` | `clusters config` | `status` | `line` | `prioClusterHAVMComponentProtection` | `%s_ha_vm_component_protection.enabled=>enabled`; `%s_ha_vm_component_protection.disabled=>disabled`; `%s_ha_vm_component_protection.unknown=>unknown` |
| Cluster | property chart created when cluster properties refresh | `%s_overall_status` | `vsphere.cluster_overall_status` | `clusters status` | `status` | `line` | `prioClusterOverallStatus` | `%s_overall.status.green=>green`; `%s_overall.status.red=>red`; `%s_overall.status.yellow=>yellow`; `%s_overall.status.gray=>gray` |
| Cluster | property chart created when cluster properties refresh | `%s_vmotions` | `vsphere.cluster_vmotions` | `clusters migrations` | `migrations` | `line` | `prioClusterVMotions` | `%s_num_vmotions=>vmotions algo=incremental` |
| Cluster | property chart created when cluster properties refresh | `%s_drs_score` | `vsphere.cluster_drs_score` | `clusters drs` | `percentage` | `line` | `prioClusterDRSScore` | `%s_drs_score=>score` |
| Cluster | property chart created when cluster properties refresh | `%s_drs_balance` | `vsphere.cluster_drs_balance` | `clusters drs` | `score` | `line` | `prioClusterDRSBalance` | `%s_current_balance=>current div=1000`; `%s_target_balance=>target div=1000` |
| Cluster | property chart created when cluster properties refresh | `%s_vm_count` | `vsphere.cluster_vm_count` | `clusters vms` | `VMs` | `line` | `prioClusterVMCount` | `%s_usage_total_vm_count=>total`; `%s_usage_powered_off_vm_count=>powered_off` |
| Cluster | property chart created when cluster properties refresh | `%s_usage_cpu` | `vsphere.cluster_usage_cpu` | `clusters cpu` | `MHz` | `line` | `prioClusterUsageCPU` | `%s_usage_cpu_demand_mhz=>demand`; `%s_usage_cpu_entitled_mhz=>entitled`; `%s_usage_cpu_reservation_mhz=>reserved` |
| Cluster | property chart created when cluster properties refresh | `%s_usage_mem` | `vsphere.cluster_usage_mem` | `clusters mem` | `MB` | `line` | `prioClusterUsageMem` | `%s_usage_mem_demand_mb=>demand`; `%s_usage_mem_entitled_mb=>entitled`; `%s_usage_mem_reservation_mb=>reserved` |
| Cluster | perf chart created only after cluster perf data arrives | `%s_cpu_utilization` | `vsphere.cluster_cpu_utilization` | `clusters cpu` | `percentage` | `line` | `prioClusterCPUUtilization` | `%s_cpu.usage.average=>used div=100` |
| Cluster | perf chart created only after cluster perf data arrives | `%s_cpu_usage_mhz` | `vsphere.cluster_cpu_usage` | `clusters cpu` | `MHz` | `line` | `prioClusterCPUUsage` | `%s_cpu.usagemhz.average=>used`; `%s_cpu.totalmhz.average=>total` |
| Cluster | perf chart created only after cluster perf data arrives | `%s_mem_utilization` | `vsphere.cluster_mem_utilization` | `clusters mem` | `percentage` | `line` | `prioClusterMemUtilization` | `%s_mem.usage.average=>used div=100` |
| Cluster | perf chart created only after cluster perf data arrives | `%s_mem_usage` | `vsphere.cluster_mem_usage` | `clusters mem` | `KiB` | `line` | `prioClusterMemUsage` | `%s_mem.consumed.average=>consumed`; `%s_mem.active.average=>active`; `%s_mem.granted.average=>granted`; `%s_mem.shared.average=>shared`; `%s_mem.overhead.average=>overhead`; `%s_mem.swapused.average=>swap_used` |
| Cluster | perf chart created only after cluster perf data arrives | `%s_services_fairness` | `vsphere.cluster_services_fairness` | `clusters drs` | `score` | `line` | `prioClusterServicesFairness` | `%s_clusterServices.cpufairness.latest=>cpu`; `%s_clusterServices.memfairness.latest=>memory` |
| Cluster | perf chart created only after cluster perf data arrives | `%s_services_effective_cpu` | `vsphere.cluster_services_effective_cpu` | `clusters cpu` | `MHz` | `line` | `prioClusterServicesEffectiveCPU` | `%s_clusterServices.effectivecpu.average=>effective_cpu` |
| Cluster | perf chart created only after cluster perf data arrives | `%s_services_effective_mem` | `vsphere.cluster_services_effective_mem` | `clusters mem` | `MB` | `line` | `prioClusterServicesEffectiveMem` | `%s_clusterServices.effectivemem.average=>effective_mem` |
| Cluster | perf chart created only after cluster perf data arrives | `%s_services_failover` | `vsphere.cluster_services_failover` | `clusters ha` | `failures` | `line` | `prioClusterServicesFailover` | `%s_clusterServices.failover.latest=>failures_tolerable` |
| Cluster | perf chart created only after cluster perf data arrives | `%s_vm_migrations` | `vsphere.cluster_vm_migrations` | `clusters vmop` | `operations` | `line` | `prioClusterVMMigrations` | `%s_vmop.numVMotion.latest=>vmotion`; `%s_vmop.numSVMotion.latest=>svmotion`; `%s_vmop.numXVMotion.latest=>xvmotion` |
| Cluster | perf chart created only after cluster perf data arrives | `%s_vm_lifecycle` | `vsphere.cluster_vm_lifecycle` | `clusters vmop` | `operations` | `line` | `prioClusterVMLifecycle` | `%s_vmop.numPoweron.latest=>poweron`; `%s_vmop.numPoweroff.latest=>poweroff`; `%s_vmop.numCreate.latest=>create`; `%s_vmop.numDestroy.latest=>destroy`; `%s_vmop.numClone.latest=>clone`; `%s_vmop.numDeploy.latest=>deploy` |
| Cluster | perf chart created only after cluster perf data arrives | `%s_vm_management` | `vsphere.cluster_vm_management` | `clusters vmop` | `operations` | `line` | `prioClusterVMManagement` | `%s_vmop.numReconfigure.latest=>reconfigure`; `%s_vmop.numReset.latest=>reset`; `%s_vmop.numSuspend.latest=>suspend`; `%s_vmop.numRegister.latest=>register`; `%s_vmop.numUnregister.latest=>unregister` |
| Cluster | perf chart created only after cluster perf data arrives | `%s_vm_guest_ops` | `vsphere.cluster_vm_guest_ops` | `clusters vmop` | `operations` | `line` | `prioClusterVMGuestOps` | `%s_vmop.numRebootGuest.latest=>reboot`; `%s_vmop.numShutdownGuest.latest=>shutdown`; `%s_vmop.numStandbyGuest.latest=>standby` |
| Cluster | perf chart created only after cluster perf data arrives | `%s_vm_cold_migrations` | `vsphere.cluster_vm_cold_migrations` | `clusters vmop` | `operations` | `line` | `prioClusterVMColdMigrations` | `%s_vmop.numChangeDS.latest=>change_ds`; `%s_vmop.numChangeHost.latest=>change_host`; `%s_vmop.numChangeHostDS.latest=>change_host_ds` |
| Resource pool | property chart created when resource pool properties refresh | `%s_cpu_usage` | `vsphere.resource_pool_cpu_usage` | `resource pools cpu` | `MHz` | `line` | `prioResourcePoolCPUUsage` | `%s_cpu_usage=>usage`; `%s_cpu_demand=>demand` |
| Resource pool | property chart created when resource pool properties refresh | `%s_cpu_entitlement` | `vsphere.resource_pool_cpu_entitlement` | `resource pools cpu` | `MHz` | `line` | `prioResourcePoolCPUEntitlement` | `%s_cpu_entitlement_distributed=>distributed` |
| Resource pool | property chart created when resource pool properties refresh | `%s_cpu_allocation` | `vsphere.resource_pool_cpu_allocation` | `resource pools cpu` | `MHz` | `line` | `prioResourcePoolCPUAllocation` | `%s_cpu_reservation_used=>reservation_used`; `%s_cpu_unreserved_for_vm=>unreserved_for_vm`; `%s_cpu_max_usage=>max_usage` |
| Resource pool | property chart created when resource pool properties refresh | `%s_mem_usage` | `vsphere.resource_pool_mem_usage` | `resource pools mem` | `MB` | `line` | `prioResourcePoolMemUsage` | `%s_mem_usage_host=>host`; `%s_mem_usage_guest=>guest` |
| Resource pool | property chart created when resource pool properties refresh | `%s_mem_entitlement` | `vsphere.resource_pool_mem_entitlement` | `resource pools mem` | `MB` | `line` | `prioResourcePoolMemEntitlement` | `%s_mem_entitlement_distributed=>distributed` |
| Resource pool | property chart created when resource pool properties refresh | `%s_mem_allocation` | `vsphere.resource_pool_mem_allocation` | `resource pools mem` | `bytes` | `line` | `prioResourcePoolMemAllocation` | `%s_mem_reservation_used=>reservation_used`; `%s_mem_unreserved_for_vm=>unreserved_for_vm`; `%s_mem_max_usage=>max_usage` |
| Resource pool | property chart created when resource pool properties refresh | `%s_mem_breakdown` | `vsphere.resource_pool_mem_breakdown` | `resource pools mem` | `MB` | `line` | `prioResourcePoolMemBreakdown` | `%s_mem_private=>private`; `%s_mem_shared=>shared`; `%s_mem_swapped=>swapped`; `%s_mem_ballooned=>ballooned`; `%s_mem_overhead=>overhead`; `%s_mem_consumed_overhead=>consumed_overhead`; `%s_mem_compressed=>compressed div=1024` |
| Resource pool | property chart created when resource pool properties refresh | `%s_cpu_config` | `vsphere.resource_pool_cpu_config` | `resource pools cpu` | `MHz` | `line` | `prioResourcePoolCPUConfig` | `%s_cpu_reservation=>reservation`; `%s_cpu_limit=>limit` |
| Resource pool | property chart created when resource pool properties refresh | `%s_mem_config` | `vsphere.resource_pool_mem_config` | `resource pools mem` | `MB` | `line` | `prioResourcePoolMemConfig` | `%s_mem_reservation=>reservation`; `%s_mem_limit=>limit` |
| Resource pool | property chart created when resource pool properties refresh | `%s_overall_status` | `vsphere.resource_pool_overall_status` | `resource pools status` | `status` | `line` | `prioResourcePoolOverallStatus` | `%s_overall.status.green=>green`; `%s_overall.status.red=>red`; `%s_overall.status.yellow=>yellow`; `%s_overall.status.gray=>gray` |

## Metric Source Lists

Performance counter lists are selected in
`src/go/plugin/go.d/collector/vsphere/discover/metric_lists.go`.

VM performance counters:

- `cpu.usage.average`
- `mem.usage.average`
- `mem.granted.average`
- `mem.consumed.average`
- `mem.active.average`
- `mem.shared.average`
- `mem.swapinRate.average`
- `mem.swapoutRate.average`
- `mem.swapped.average`
- `net.bytesRx.average`
- `net.bytesTx.average`
- `net.packetsRx.summation`
- `net.packetsTx.summation`
- `net.droppedRx.summation`
- `net.droppedTx.summation`
- `disk.read.average`
- `disk.write.average`
- `disk.maxTotalLatency.latest`
- `sys.uptime.latest`

Host performance counters:

- `cpu.usage.average`
- `mem.usage.average`
- `mem.granted.average`
- `mem.consumed.average`
- `mem.active.average`
- `mem.shared.average`
- `mem.sharedcommon.average`
- `mem.swapinRate.average`
- `mem.swapoutRate.average`
- `net.bytesRx.average`
- `net.bytesTx.average`
- `net.packetsRx.summation`
- `net.packetsTx.summation`
- `net.droppedRx.summation`
- `net.droppedTx.summation`
- `net.errorsRx.summation`
- `net.errorsTx.summation`
- `disk.read.average`
- `disk.write.average`
- `disk.maxTotalLatency.latest`
- `sys.uptime.latest`

Optional power counters requested only when `collect_power_metrics` is enabled:

- VM: `power.power.average`
- VM: `power.energy.summation`
- Host: `power.power.average`
- Host: `power.powerCap.average`
- Host: `power.energy.summation`
- Host: `power.capacity.usable.average`
- Host: `power.capacity.usage.average`
- Host: `power.capacity.usagePct.average`
- Host: `power.capacity.usageIdle.average`
- Host: `power.capacity.usageSystem.average`
- Host: `power.capacity.usageVm.average`

Datastore performance counters:

- `datastore.numberReadAveraged.average`
- `datastore.numberWriteAveraged.average`
- `datastore.totalReadLatency.average`
- `datastore.totalWriteLatency.average`
- `datastore.read.average`
- `datastore.write.average`

Cluster performance counters:

- `clusterServices.effectivecpu.average`
- `clusterServices.effectivemem.average`
- `clusterServices.cpufairness.latest`
- `clusterServices.memfairness.latest`
- `clusterServices.failover.latest`
- `cpu.usage.average`
- `cpu.usagemhz.average`
- `cpu.totalmhz.average`
- `mem.usage.average`
- `mem.consumed.average`
- `mem.overhead.average`
- `mem.active.average`
- `mem.granted.average`
- `mem.shared.average`
- `mem.swapused.average`
- `vmop.numVMotion.latest`
- `vmop.numSVMotion.latest`
- `vmop.numXVMotion.latest`
- `vmop.numPoweron.latest`
- `vmop.numPoweroff.latest`
- `vmop.numCreate.latest`
- `vmop.numDestroy.latest`
- `vmop.numClone.latest`
- `vmop.numDeploy.latest`
- `vmop.numReset.latest`
- `vmop.numSuspend.latest`
- `vmop.numReconfigure.latest`
- `vmop.numRegister.latest`
- `vmop.numUnregister.latest`
- `vmop.numChangeDS.latest`
- `vmop.numChangeHost.latest`
- `vmop.numChangeHostDS.latest`
- `vmop.numRebootGuest.latest`
- `vmop.numShutdownGuest.latest`
- `vmop.numStandbyGuest.latest`
- `clusterServices.clusterDrsScore.latest`
- `clusterServices.vmDrsScore.latest`

## Property Metric Semantics

| Resource | Property path requested | Metric behavior |
|---|---|---|
| VM | discovery requests `name`, `parent`, `runtime.host`, `runtime.connectionState`, `runtime.powerState`, `runtime.consolidationNeeded`, `summary.guest`, `summary.config`, `summary.storage`, `summary.overallStatus`, `snapshot`; also `config.hardware.device` only when `collect_vm_disks` or `collect_vm_disk_performance` is enabled; also `config.instanceUuid` only when `collect_vsan` is enabled | Emits `overall.status.{green,red,yellow,gray}`, `power_state.*`, `connection_state.*`, VMware Tools running/version state, disk consolidation-needed state, configured CPU/memory/device counts, aggregate storage usage, and snapshot aggregate metrics for all discovered VMs. Emits real-time performance counters only when the VM is `poweredOn`. If `runtime.host` is absent for a non-running VM, the VM folder parent is used to recover the datacenter label when possible. Guest hostname/IP and guest OS labels are excluded from this PR by user decision. VM virtual disk capacity is emitted only when `collect_vm_disks` is enabled. VM virtual disk performance is emitted only when `collect_vm_disk_performance` is enabled and vSphere returns per-instance `virtualDisk.*` metrics. VM network-interface performance is emitted only when `collect_vm_nic_performance` is enabled and vSphere returns per-instance `net.*` metrics. VM power and energy metrics are emitted only when `collect_power_metrics` is enabled and vSphere returns aggregate `power.*` counters. VM vSAN performance is emitted only when `collect_vsan` is enabled and vSAN returns a matching VM instance UUID. |
| Host | discovery requests `name`, `parent`, `runtime.connectionState`, `runtime.powerState`, `runtime.inMaintenanceMode`, `summary.overallStatus`; also `config.vsanHostConfig.clusterInfo.nodeUuid` only when `collect_vsan` is enabled | Emits `overall.status.{green,red,yellow,gray}`, `power_state.*`, `connection_state.*`, and `maintenance_status.*` for all discovered hosts. Emits real-time performance counters only when the host is `poweredOn`. Host physical network-interface performance is emitted only when `collect_host_nic_performance` is enabled and vSphere returns per-instance `net.*` metrics. Host disk/LUN/device, storage-adapter, storage-path, and CPU-instance performance are emitted only when their `collect_host_*_performance` groups are enabled and vSphere returns the matching per-instance counters. Host power, energy, and power-capacity metrics are emitted only when `collect_power_metrics` is enabled and vSphere returns aggregate `power.*` counters. Host vSAN performance is emitted only when `collect_vsan` is enabled and vSAN returns a matching host node UUID. |
| Datastore | refresh requests `summary`, `overallStatus` | Emits `capacity`, `free_space`, `used_space`, `used_space_pct`, and `overall.status.*`. Capacity/free/used are zeroed when `Accessible=false`. |
| Network | discovery requests `name`, `parent`, `summary`, `host`, and `vm` only when `collect_network_topology` is enabled | Emits no metrics. Cached Network and Distributed Virtual Port Group status/relationships are used only by the topology Function. |
| Datastore cluster | discovery requests `StoragePod` `name`, `parent`, `summary`, `podStorageDrsEntry`; only when `collect_datastore_clusters` is enabled | Emits optional StoragePod capacity, free, used, utilization, and Storage DRS enabled/disabled status for datastore clusters matching `datastore_cluster_include`. |
| Cluster | refresh requests `name`, `summary`, `configurationEx`, `overallStatus`; discovery includes `configurationEx.vsanConfigInfo` only when `collect_vsan` is enabled | Emits capacity/topology/DRS/HA/usage/overall-status property metrics, with conditional fields reset to zero before update. vSAN cluster space, health, and performance metrics are emitted only when `collect_vsan` is enabled, the cluster is vSAN-enabled, and vSAN API calls return data. |
| Resource pool | refresh requests `name`, `summary`, `config`, `runtime`, `overallStatus` | Emits quick stats, runtime allocation, config reservation/limit, memory breakdown, and overall-status metrics. |

## Lifecycle Contract

- Hosts, VMs, datastores, clusters, resource pools, and optional datastore
  clusters are tracked in discovered maps.
- Each absent resource increments a failure counter per collection.
- `failedUpdatesLimit` is `10`.
- When the failure counter reaches `10`, charts whose ID starts with
  `<resourceID>_` are marked removed and not-created, making them obsolete.
- Datastore property charts are created when the datastore is present.
- Datastore performance charts are created only after performance data arrives.
- Cluster property charts are created when the cluster is present.
- Cluster performance charts are created only after performance data arrives.
- Non-powered-on hosts and VMs are discovered when vSphere returns them and the
  include selectors keep them. Their property/status metrics keep the resource
  alive; real-time host/VM performance query specs are not generated for them.
- Optional VM virtual disk metrics are not part of the legacy V1 surface. They
  are emitted only when `collect_vm_disks` is enabled, only for disks matching
  `vm_disk_include`.
- Optional VM virtual disk performance metrics are not part of the legacy V1
  surface. They are requested only when `collect_vm_disk_performance` is enabled
  by adding `virtualDisk.*` counters with `Instance="*"` to powered-on VM
  performance queries. Returned per-instance metrics are emitted only when the
  vSphere performance instance matches `vm_disk_include`.
- Optional VM network-interface performance metrics are not part of the legacy
  V1 surface. They are requested only when `collect_vm_nic_performance` is
  enabled by adding selected `net.*` counters with `Instance="*"` to powered-on
  VM performance queries. Existing aggregate VM `net.*` metrics remain
  default-on and are not suppressed by this optional surface. Returned
  per-instance metrics are emitted only when the vSphere performance instance
  matches `vm_nic_include`.
- Optional host physical network-interface performance metrics are not part of
  the legacy V1 surface. They are requested only when
  `collect_host_nic_performance` is enabled by adding selected `net.*` counters
  with `Instance="*"` to powered-on host performance queries. Existing
  aggregate host `net.*` metrics remain default-on and are not suppressed by
  this optional surface. Returned per-instance metrics are emitted only when
  the vSphere performance instance matches `host_nic_include`.
- Optional datastore cluster metrics are not part of the legacy V1 surface.
  They are emitted only when `collect_datastore_clusters` is enabled, only for
  StoragePod objects matching `datastore_cluster_include`.
- Optional host disk/LUN/device, storage-adapter, storage-path, and CPU-instance
  metrics are not part of the legacy V1 surface. They are requested only when their
  `collect_host_*_performance` groups are enabled by adding selected
  `disk.*`, `storageAdapter.*`, `storagePath.*`, or per-instance `cpu.*`
  counters to powered-on host performance queries. Existing aggregate host disk,
  network, CPU, memory, and uptime metrics remain default-on and are not
  suppressed by these optional surfaces. Returned per-instance metrics are emitted
  only when their include selector matches.
- Optional host/VM power metrics are not part of the legacy V1 surface. They
  are requested only when `collect_power_metrics` is enabled by adding selected
  aggregate `power.*` counters with empty instance to powered-on host and VM
  performance queries. They emit one aggregate set per included host or VM and
  therefore use the existing host/VM include selectors instead of a new child
  selector.
- Optional vSAN metrics are not part of the legacy V1 surface. They are queried
  only when `collect_vsan` is enabled, only for clusters whose vSAN config is
  enabled and match the dedicated vSAN selectors, and only through the
  vSAN Management API. Missing vSAN API support, missing vSAN Performance
  Service, or unavailable vSAN counters cause one-time warnings and no emitted
  vSAN series for that query. The first implemented vSAN surface covers cluster
  space, cluster health, and cluster/host/VM vSAN performance; vSAN events and
  deeper disk-group, disk, component, or CMMDS entity metrics are not emitted by
  this option.

## Health Alert Contract

| Template | Context | Lookup/calc | Warning | Critical | Recipient |
|---|---|---|---|---|---|
| `vsphere_vm_cpu_utilization` | `vsphere.vm_cpu_utilization` | `average -10m unaligned match-names of used` | `$this > (($status >= $WARNING) ? (75) : (85))` | `$this > (($status == $CRITICAL) ? (85) : (95))` | `silent` |
| `vsphere_vm_mem_utilization` | `vsphere.vm_mem_utilization` | `$used` | `$this > (($status >= $WARNING) ? (80) : (90))` | `$this > (($status == $CRITICAL) ? (90) : (98))` | `silent` |
| `vsphere_vm_snapshot_chain_depth` | `vsphere.vm_snapshot_max_chain_depth` | `$depth` | `$this > 3` | none | `sysadmin` |
| `vsphere_vm_snapshot_age` | `vsphere.vm_snapshot_max_age` | `$age` | none | `$this > 86400` | `sysadmin` |
| `vsphere_host_cpu_utilization` | `vsphere.host_cpu_utilization` | `average -10m unaligned match-names of used` | `$this > (($status >= $WARNING) ? (75) : (85))` | `$this > (($status == $CRITICAL) ? (85) : (95))` | `sysadmin` |
| `vsphere_host_mem_utilization` | `vsphere.host_mem_utilization` | `$used` | `$this > (($status >= $WARNING) ? (80) : (90))` | `$this > (($status == $CRITICAL) ? (90) : (98))` | `sysadmin` |

## Current Artifact Drift To Preserve Or Fix Explicitly

The migration must preserve runtime behavior from code. Existing artifact drift
must be fixed only as explicit metadata/docs updates:

- Code sets `vsphere.host_net_traffic` chart type to `area`; metadata currently
  says `line`.
- Code sets VM and host network drop units to `drops`; metadata currently says
  `packets`.
- Cluster performance counter selection includes
  `clusterServices.clusterDrsScore.latest` and
  `clusterServices.vmDrsScore.latest`, but the current chart templates do not
  expose chart dimensions for those exact counter keys. A v2 migration must not
  accidentally create public series for them unless an enrichment row explicitly
  adds new contexts/dimensions.
