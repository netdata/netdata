# Monitor Ceph

Netdata gives you a complete operational view of Ceph by collecting three complementary telemetry surfaces: the MGR Prometheus module, official `ceph-exporter`, and the Ceph Dashboard API. Deploy one Agent close to each Ceph node to monitor cluster state, daemon health, host resources, RGW traffic, and local hardware together.

## What you can monitor

- Cluster health, manager health, monitor quorum, and cluster capacity.
- OSD populations, placement groups, pools, recovery, and scrub activity.
- CephFS/MDS, RBD, RBD Mirror, SMB, and client I/O telemetry exposed by your Ceph release.
- Host-local daemon performance from official `ceph-exporter`.
- RGW requests, Lua execution, notifications, queues, retries, and access logs.
- RGW endpoint availability and TLS certificate health.
- NVMe-oF gateway, block-device, host, subsystem, and namespace telemetry from supported exporters.
- Node hardware health, cooling, power, memory, processors, storage, and temperature reporting on Tentacle.
- Dashboard API component integrity and on-demand Ceph investigation Functions.
- Per-node disks, filesystems, network interfaces, processes, and logs through Netdata's host collectors.

## Operational coverage map

Use this map to identify the Netdata surface that owns the operational question you are investigating.

| Operational need | What Netdata monitors | Primary owner | Operator behavior |
|---|---|---|---|
| Cluster health | Overall health, named Ceph health checks, daemon crashes, slow operations | MGR Prometheus | Categorical failures notify; workload-specific checks can be tuned |
| Monitor availability | MON quorum and quorum risk | MGR Prometheus | Investigate monitor membership and elections |
| Data integrity | Unfound objects, damaged PGs, scrub errors | MGR Prometheus | Treat as priority data-safety conditions |
| Placement groups | Active/clean PG counts, scrub status, PG density, recovery blockers | MGR Prometheus | Inspect affected pool and recovery progress |
| Capacity | Cluster physical capacity plus OSD/pool thresholds | Ceph Dashboard API and MGR Prometheus | Dashboard provides physical utilization; MGR exposes Ceph threshold states |
| OSD availability | Down OSDs, down OSD hosts, and down-OSD percentage | MGR Prometheus | Identify the affected daemon or host |
| Recovery and rebalance | Backfill/recovery blockers and recovery work | MGR Prometheus | Watch conditions that prevent repair or rebalance |
| CephFS | MDS damage, degradation, standby availability, and rank health | MGR Prometheus | Investigate the affected filesystem and MDS rank |
| RBD mirroring | Local/remote snapshot timestamp synchronization | MGR Prometheus | Identify mirrored images that have diverged |
| RGW service health | Notifications, Lua execution, queue pressure, retries, aborted requests | MGR Prometheus | Inspect aggregate gateway behavior |
| RGW request outcomes | Status classes, bytes, clients, and request duration | `web_log` | Analyze complete RGW access logs |
| RGW endpoint reachability | Unauthenticated HTTP liveness | `httpcheck` | Verify the selected endpoint is reachable |
| RGW certificates | Certificate expiration and revocation | `x509check` | Track certificate lifecycle independently of RGW traffic |
| Node health | CPU, memory, disk I/O, filesystems, network interfaces, and processes | Standard Netdata collectors | Continue using normal Agent monitoring for each Ceph node |
| API component integrity | Dashboard API collection status | Ceph Dashboard API | Detect component-specific collection failures |
| Incident investigation | Detailed health, OSD, pool, and daemon inventory | Ceph Dashboard Functions | Run on demand during an investigation |

### How Netdata maps Ceph health directives

Ceph publishes its own health-check directives. Netdata maps the directives that have a stable public metric signal to named alerts and charts, so operators can use Ceph's operational vocabulary while working in Netdata.

Examples:

| Ceph directive | Netdata operator view |
|---|---|
| `RECENT_CRASH` | Recent Ceph daemon crash |
| `RECENT_MGR_MODULE_CRASH` | Recent manager-module crash |
| `MON_DOWN` | Monitor down and monitor-quorum summaries |
| `OSD_DOWN` | Down OSD and OSD-host conditions |
| `OBJECT_UNFOUND` | Unfound object condition |
| `PG_DAMAGED` | Damaged placement-group condition |
| `PG_NOT_SCRUBBED` | Placement groups overdue for scrub |
| `POOL_FULL` | Full pool condition |
| `MDS_ALL_DOWN` | CephFS unavailable |
| `FS_WITH_FAILED_MDS` | CephFS MDS rank has failed with no standby |

Netdata's standard system alerts remain the owners of node-level conditions such as filesystem usage and packet drops. Ceph-specific alerts do not duplicate those incidents; they add cluster, daemon, and storage-service state that Ceph itself publishes.

## Deployment model

Run one Agent on each Ceph node. Each Agent monitors the Ceph services and host resources on its own node, preserving local admin-socket, disk, interface, process, and log context.

| Surface | Deployment | What it provides |
|---|---|---|
| MGR Prometheus module | One logical job per Ceph cluster | Cluster health, quorum, capacity, placement groups, pools, RGW aggregate telemetry, and exporter-local NVMe-oF telemetry |
| `ceph-exporter` | One job on each node whose daemon telemetry you need | Host-local daemon performance and daemon inventory |
| Ceph Dashboard API | One logical job per Ceph cluster | Dashboard API component integrity and Ceph investigation Functions |
| Host collectors | Every Agent | Node disks, filesystems, network interfaces, processes, and logs |

Use one stable job identity for the MGR surface. If the active MGR moves, update DNS or the reverse proxy to the current active endpoint rather than creating one job for every possible MGR. Multiple active MGR jobs for the same cluster create duplicate cluster alert owners.

For `ceph-exporter`, preserve each node's own job or virtual-node identity. Keep exporter jobs host-local rather than merging them across hosts.

A Parent can receive streams from every Ceph Agent. Parenting changes where data is stored and queried, not the identity of each Ceph node: child charts retain their node scope, labels, and alert instances. Organize Netdata Cloud rooms by site, cluster, or role, then use the combined view for infrastructure-wide investigation.

## Enable telemetry

### MGR Prometheus module

Enable the Ceph MGR Prometheus module and expose one stable HTTP or HTTPS endpoint for the cluster. Configure the Netdata Prometheus collector to scrape that endpoint. The built-in Ceph profile recognizes the official metric surface automatically.

For high availability, put a stable address in front of the active MGR and keep the Netdata job name and virtual-node identity stable.

### ceph-exporter

Deploy or enable official `ceph-exporter` on each Ceph node whose daemon telemetry you need. Collect from each exporter on its own host. Use the exporter priority limit to select the daemon counters published, and size `max_time_series` and `max_time_series_per_metric` for that surface.

### Ceph Dashboard API

Enable the Ceph Dashboard module, secure it with TLS, and create a read-only Dashboard user. Configure one native Ceph collector job per cluster. The Dashboard collector complements Prometheus: it owns API component integrity and provides Ceph investigation Functions.

## Supported releases

Netdata's built-in Ceph profile recognizes the metric surfaces of:

- Reef 18.2.8
- Squid 19.2.5
- Tentacle 20.2.3

Release-specific charts follow the telemetry exposed by each Ceph release. For example, node hardware and local NVMe-oF gateway charts appear when Tentacle exports those series.

## Authentication, TLS, and routing

- Give the Dashboard user read access to the required Ceph scopes. The built-in read-only Dashboard role is sufficient.
- Use TLS for Dashboard and MGR endpoints where practical.
- Point each job directly at the Ceph API or metrics endpoint, not a generic reverse-proxy login page.
- Preserve stable job identity through redirects and failover. Chart identity and alert ownership depend on it.
- If Ceph redirects HTTP to HTTPS, configure the final HTTPS URL explicitly rather than relying on redirect following.

## Collection cadence and cardinality

Netdata scrape frequency does not control Ceph producer refresh frequency.

- The MGR Prometheus module refreshes its metric cache on its configured Ceph-side interval.
- `ceph-exporter` refreshes daemon metrics on its configured interval.
- Netdata samples the exposed current cache at the job's `update_every`.

Match the Netdata interval to the producer cache interval when you want to avoid redundant samples. A faster scrape does not force Ceph to refresh more often.

Ceph can expose a large metric surface. Set job-level `max_time_series` and `max_time_series_per_metric` after measuring your release and exporter configuration. Enable broad optional debug or detail surfaces only after sizing the resulting chart and dimension cardinality.

## Alert ownership and routing

The MGR Prometheus profile owns cluster and exporter-local Ceph alerts:

- cluster health and manager-module health;
- monitor quorum and OSD population summaries;
- placement group, pool, capacity, and recovery conditions;
- hardware and NVMe-oF conditions exposed by supported exporters;
- RGW notification, Lua, request-fallback, queue-pressure, and multisite retry conditions.

The native Dashboard collector owns API component collection failures. Generic Netdata collectors own host-local and endpoint checks:

- host disks, filesystems, and network interfaces;
- `httpcheck` for unauthenticated RGW endpoint liveness;
- `x509check` for RGW certificate expiration and revocation;
- `web_log` for complete RGW access logs, including request outcomes and latency.

### Silent and notifying alerts

Policy-dependent conditions are silent by default so you can tune them to your workload. Examples include:

- queue depth and notification backlog pressure;
- storage-capacity policy;
- traffic anomalies;
- client-error rates;
- selected hardware temperature thresholds.

Categorical failures notify by default. Enable silent alerts after tuning their thresholds for your environment. To change routing without changing stock thresholds, copy the health template into the local health configuration.

### Collection gaps and disappearing charts

The collection alert reports scrape failures, while data-state alerts continue to use the latest stored values. When Ceph withdraws an entity and its chart becomes obsolete, Netdata removes that instance alert.

## RGW and S3 observability

The MGR profile provides aggregate RGW telemetry for requests, aborted requests, Lua scripts, notifications, queues, and multisite retries.

Collect the RGW access log with `web_log` to analyze HTTP outcomes, bytes, clients, and latency. The Ceph JSON example maps request, status, size, and client fields, and declares `total_time` as a numeric custom field in milliseconds, preserving Ceph's exact duration field.

Use `httpcheck` for unauthenticated endpoint liveness and `x509check` for certificate expiration or revocation.

## Investigation Functions

The native Ceph Dashboard collector provides Functions for detailed health, OSD, pool, and incident investigation. Functions run on demand and do not create charts by themselves.

Use Functions when you need current inventory or component detail. Use charts and alerts for continuous monitoring. If a Function requires the Ceph orchestrator, ensure the Dashboard configuration exposes it.

## Retention and capacity planning

Plan retention separately on Children and Parents.

1. Deploy one Agent per Ceph node and run a representative workload for several days.
2. Measure the charts and dimensions produced by your Ceph release, selected exporter surfaces, and collectors.
3. Decide where long retention lives:
   - keep short high-resolution retention locally on each Child;
   - stream longer retention to one or more Parents.
4. Estimate disk requirements from measured chart and dimension counts, sample interval, tiering mode, and retention horizon.
5. Revalidate the estimate after several days and again after several weeks.
6. For multi-year storage, export to Prometheus remote storage, Grafana-compatible storage, or another external system rather than storing every sample in the Agent database.

Do not extrapolate from a single quiet day: hardware inventories, failover, optional exporter surfaces, and incident activity change cardinality. Chart cardinality follows enabled telemetry surfaces and labels, not Ceph storage capacity alone.

Use measured charts and exported long-term data for trend analysis.

## Netdata Cloud, rooms, and sites

Connect each Agent to Netdata Cloud unless you operate a fully on-prem deployment. Organize rooms by site, cluster, or operational responsibility. A room is a navigation and access boundary, not a data aggregation layer.

Parents preserve child identity. A Parent can provide long retention, unified querying, and a single TLS ingress while every Ceph node remains independently visible in Cloud.

For on-prem-only deployments, streaming and local dashboards remain available. Local Agent health and notification mechanisms continue to work.

## Grafana, Prometheus, Zabbix, and exporting

Netdata coexists with your existing monitoring:

- Keep or deploy Grafana dashboards independently.
- Keep existing Prometheus exporters; Netdata can read the same official Ceph endpoints without replacing that workflow.
- Keep an existing Zabbix deployment independently and collect Ceph through Zabbix's own integrations.
- Export Netdata metrics to external systems with the exporting engine.
- Send selected Netdata alerts to external systems through supported notification integrations.

Do not point multiple Netdata jobs at the same logical Ceph cluster MGR endpoint under different identities unless you intentionally want separate alert owners.

## Notifications

Route notifications centrally through Netdata Cloud when your Agents are connected, or configure notifications per Agent for on-prem deployments. Start with notifying categorical alerts, then enable and tune policy alerts where you have measured meaningful thresholds.

## Incident and configuration references

For the native physical-capacity alert, see the [Ceph cluster space usage](/src/health/guides/ceph/ceph_cluster_space_usage.md) incident guide.

For collector configuration details, see:

- [Ceph](/src/go/plugin/go.d/collector/ceph/integrations/ceph.md)
- [Ceph Prometheus](/src/go/plugin/go.d/collector/prometheus/integrations/ceph_prometheus.md)

For Agent deployment, streaming, retention, exporting, and notifications, see:

- [Deployment with centralization points](/docs/deployment-guides/deployment-with-centralization-points.md)
- [Metrics centralization points](/docs/observability-centralization-points/metrics-centralization-points/README.md)
- [Sizing Netdata Parents](/docs/netdata-agent/sizing-netdata-agents/disk-requirements-and-retention.md)
- [Exporting reference](/src/exporting/README.md)
- [Notifications](/docs/alerts-and-notifications/notifications/README.md)
