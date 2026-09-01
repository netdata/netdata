# Monitor Ceph

Netdata gives you a complete operational view of Ceph by collecting four complementary telemetry surfaces—the MGR
Prometheus module, official `ceph-exporter`, the NVMe-oF gateway exporter, and the Ceph Dashboard API—and by running
authenticated S3 lifecycle checks from the client vantages that depend on object storage.
Deploy one Agent close to each Ceph node to monitor cluster state, daemon health, host resources, RGW traffic,
client-visible S3 correctness, and local hardware.

## What you can monitor

- Cluster health, manager health, monitor quorum, and cluster capacity.
- OSD populations, placement groups, pools, recovery, and scrub activity.
- CephFS/MDS, RBD, RBD Mirror, SMB, and client I/O telemetry exposed by your Ceph release.
- Host-local daemon performance from official `ceph-exporter`.
- RGW requests, Lua execution, notifications, queues, retries, and access logs.
- Authenticated S3 write, read, list, delete, payload-integrity, cleanup, and latency results from selected vantages.
- Directional multisite S3 replication, payload integrity, recovery-point objective, and delete-propagation results.
- RGW endpoint availability and TLS certificate health.
- NVMe-oF gateway, block-device, host, subsystem, and namespace telemetry from supported exporters.
- Node hardware health, cooling, power, memory, processors, storage, and temperature reporting on Tentacle.
- Dashboard API component integrity and on-demand Ceph investigation Functions.
- Per-node disks, filesystems, network interfaces, processes, and logs through Netdata's host collectors.

## Ceph telemetry and Netdata

Ceph natively exposes metrics in the Prometheus exposition format through three interfaces:

- The **Ceph Manager Prometheus module** is the cluster-level surface. The active Manager periodically builds a metric cache from cluster state and publishes it at an HTTP metrics endpoint.
- The official **`ceph-exporter`** is the host-local surface. It gathers daemon and admin-socket telemetry on each Ceph node and publishes it in the same format.
- The **NVMe-oF gateway exporter** is the gateway-local surface. Each gateway daemon publishes its own Prometheus endpoint, normally on port 10008.

The word “Prometheus” here refers to the exposition wire format, not the Prometheus monitoring stack. Netdata reads Ceph's native endpoints directly, recognizes the official metric families with its built-in Ceph profile, and creates charts and alerts from that state. You do not need to install Prometheus Server, Alertmanager, or another database for this monitoring to work. If you already operate Prometheus, it can continue reading Ceph independently; coexistence is optional.

Telemetry freshness has two independent clocks:

1. **Producer interval:** Ceph controls how often the MGR metric cache or `ceph-exporter` daemon metrics are refreshed.
2. **Consumer interval:** Netdata controls how often it reads the currently exposed cache through the job's `update_every`.

Matching the Netdata interval to the producer interval normally produces one useful observation per Ceph refresh. Reading more frequently can repeat the same cached state, while reading less frequently can skip producer updates. Choose the interval according to the observation you need: every producer update, periodic trend sampling, lower endpoint traffic, or tighter visibility for a selected surface.

## Operational coverage map

Use this map to identify the Netdata surface that owns the operational question you are investigating.

| Operational need | What Netdata monitors | Primary owner | Operator behavior |
|---|---|---|---|
| Cluster health | Overall health, named Ceph health checks, daemon crashes, slow operations | MGR Prometheus | HEALTH_ERR and other categorical failures notify; HEALTH_WARN and workload-specific checks are silent until enabled and tuned |
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
| Authenticated S3 correctness | PUT, GET integrity, LIST, DELETE, cleanup, and latency | `s3check` | Verify client-visible object operations |
| Multisite S3 replication | Directional payload integrity, visibility lag, RPO, and delete propagation | `s3check` | Verify the replication paths that client applications depend on |
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
| MGR Prometheus module | One logical job per Ceph cluster | Cluster health, quorum, capacity, placement groups, pools, and RGW aggregate telemetry |
| `ceph-exporter` | One job on each node whose daemon telemetry you need | Host-local daemon performance and daemon inventory |
| NVMe-oF gateway exporter | One job on each gateway endpoint | Gateway-local runtime, block-device, host, subsystem, and namespace telemetry |
| Ceph Dashboard API | One logical job per Ceph cluster | Dashboard API component integrity and Ceph investigation Functions |
| Authenticated S3 check | One job for each selected client vantage | Client-visible S3 object lifecycle and latency |
| Directional multisite S3 check | One explicit source-to-destination job per replication path | Client-visible replication correctness, RPO, and delete propagation |
| Host collectors | Every Agent | Node disks, filesystems, network interfaces, processes, and logs |

Use one stable job identity for the MGR surface. If the active MGR moves, update DNS or the reverse proxy to the current active endpoint rather than creating one job for every possible MGR. Multiple active MGR jobs for the same cluster create duplicate cluster alert owners.

For `ceph-exporter`, preserve each node's own job or virtual-node identity. Keep exporter jobs host-local rather than merging them across hosts.

A Parent can receive streams from every Ceph Agent. Parenting changes where data is stored and queried, not the identity of each Ceph node: child charts retain their node scope, labels, and alert instances. Organize Netdata Cloud rooms by site, cluster, or role, then use the combined view for infrastructure-wide investigation.

## Enable telemetry

### MGR Prometheus module

Enable the Ceph MGR Prometheus module and expose one stable HTTP or HTTPS endpoint for the cluster. Configure the Netdata Prometheus collector to read that endpoint. The built-in Ceph profile recognizes the official metric surface automatically.

For high availability, put a stable address in front of the active MGR and keep the Netdata job name and virtual-node identity stable.

### ceph-exporter

Deploy or enable official `ceph-exporter` on each Ceph node whose daemon telemetry you need. Collect from each exporter on its own host. Use the exporter priority limit to select the daemon counters published, and size `max_time_series` and `max_time_series_per_metric` for that surface.

### NVMe-oF gateway exporter

Enable the exporter in each Ceph NVMe-oF gateway deployment and collect every gateway endpoint whose local state you need. Preserve one stable job identity per gateway endpoint. The default gateway metrics port is 10008, but the deployment may configure a different port. Size each local job for that gateway's series surface.

### Ceph Dashboard API

Enable the Ceph Dashboard module, secure it with TLS, and create a read-only Dashboard user. Configure one native Ceph collector job per cluster. The Dashboard collector complements the metric endpoints: it owns API component integrity and provides Ceph investigation Functions.

### Authenticated S3 checks

Configure an `s3check` job for every client vantage whose object-storage behavior matters. Each job uses a dedicated
unversioned bucket and prefix, reconciles that prefix, performs one authenticated PUT, GET, LIST, DELETE, and
cleanup cycle, verifies the downloaded payload, and removes probe objects after interrupted cycles. Place jobs at each
site or RGW client path that requires a client-visible correctness signal.

For multisite replication, set `mode: multisite` and configure one explicit source and destination. The source uses the
job's top-level S3 settings; the destination has its own endpoint, region, bucket, prefix, credentials, addressing, and
transport settings. Add bounded `source_site` and `destination.site` labels, then create one job for each direction you
want to verify—for example site-a to site-b and site-b to site-a. Netdata never probes every combination automatically.
The destination prefix identifies where the replicated probe key is expected. If source and destination prefixes differ,
the replication policy must map the source route namespace onto the destination prefix.

After a multisite job deletes its exact source and destination probe keys, Netdata keeps the sanitized ownership journal until
the larger configured replication or delete deadline elapses. It then lists both owner-scoped namespaces, waits one more
collection interval, and repeats the lists in reverse endpoint order before releasing ownership. This bounded confirmation window
aligns object cleanup with the replication policy you configured.

Configure endpoint addresses that resolve to distinct S3 services; Netdata rejects literal, default-port, and
virtual-host aliases for the same bucket, but it does not resolve DNS names to guess whether two services share one
gateway.

A multisite job writes one small source object, verifies the destination object's SHA-256 digest, measures how long
client visibility takes, deletes the source, and optionally waits for the destination copy to disappear. It persists a
sanitized ownership journal across Agent restarts, reconciles both Agent-and-job-owned key namespaces before
new writes, and removes both objects when a visibility or delete deadline is reached. Set `rpo_threshold_ms`,
`replication_timeout_ms`, `delete_threshold_ms`, `delete_timeout_ms`, and `verify_delete` to match the replication
policy. Visibility and delete objectives must be at least one collection interval because Netdata polls each bounded
phase once per cycle; the two objective alerts are silent until you enable and tune them. Probe keys live in an Agent-and-job-owned namespace, so separate Agents, jobs, and reverse directions can coexist without reconciliation deleting one another’s active objects.

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

## Sizing and Ceph impact

Monitoring workload has four components:

1. **Ceph producer work:** the Manager refreshes its metric cache, and `ceph-exporter` gathers daemon counters. Ceph controls this work through its own configuration.
2. **Endpoint serving work:** each collection request reads and transfers the current exposition. Cost scales with the enabled metric surface.
3. **Netdata processing work:** parsing, chart planning, and health evaluation scale with the selected families and resulting chart cardinality.
4. **Storage and retention:** chart and dimension counts drive database volume; the streaming topology determines where long retention is stored.

Use the collection model to choose scope:

- Collect one logical MGR job per Ceph cluster, use a stable active-MGR endpoint or failover target, and align its interval with the Manager cache interval.
- Collect each `ceph-exporter` on the host whose daemon detail you need, and align its interval with the exporter refresh interval.
- Choose a Dashboard interval appropriate for cluster size, and use selectors and entity caps when complete OSD or pool detail is unnecessary.
- Enable broad optional diagnostic surfaces after measuring the resulting cardinality.
- Treat tenant, user, and bucket detail as a separate capacity decision: those surfaces can grow with the object-storage environment.
- Use hardware telemetry knowing that it scales with the reported component inventory.
- Run investigation Functions on demand during an incident rather than as a continuous inventory process.

Use job-level `max_time_series` and `max_time_series_per_metric` to bound an exposed surface. These limits do not reduce Ceph's producer cost, but they control Netdata's processing and storage boundary.

Size the deployment from measured behavior:

1. Enable the intended telemetry surfaces.
2. Run a representative workload for several days.
3. Measure charts, dimensions, labels, endpoint response size, collection duration, Agent CPU and memory, and storage growth.
4. Select local retention and Parent placement.
5. Export to long-term storage when multi-year history is required.

## Alert ownership and routing

The built-in Ceph profile recognizes all three Prometheus interfaces. Alert ownership follows the configured endpoint: one MGR job owns cluster-level alerts, and each gateway-exporter job owns its gateway-local alerts. The covered incidents are:

- cluster health and manager-module health;
- monitor quorum and OSD population summaries;
- placement group, pool, capacity, and recovery conditions;
- node-proxy hardware conditions exposed by MGR;
- gateway-local NVMe-oF conditions exposed by each gateway-exporter job;
- RGW notification, Lua, request-fallback, queue-pressure, and multisite retry conditions;
- authenticated S3 stage and multisite phase failures, plus configured latency objectives from each `s3check` job;
- directional multisite payload mismatches, RPO breaches, and delete-propagation objectives.

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

Collect the RGW JSON access log with `web_log` to analyze HTTP outcomes, bytes, clients, and latency. Configure RGW to emit its access log in JSON format and make that file available to the Agent. The Ceph JSON example maps request, status, size, and client fields, and declares `total_time` as a numeric custom field in milliseconds, preserving Ceph's exact duration field.

Use `s3check` for authenticated object lifecycle correctness, client-vantage latency, and directional multisite
replication. The MGR multisite counters show RGW replication work and retries; `s3check` proves what a client can
currently read at the destination and whether the payload is identical. Keep `httpcheck` for unauthenticated endpoint
liveness and `x509check` for certificate expiration or revocation.

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
- [S3 Compatible Object Storage](/src/go/plugin/go.d/collector/s3check/integrations/s3_compatible_object_storage.md)

For Agent deployment, streaming, retention, exporting, and notifications, see:

- [Deployment with centralization points](/docs/deployment-guides/deployment-with-centralization-points.md)
- [Metrics centralization points](/docs/observability-centralization-points/metrics-centralization-points/README.md)
- [Sizing Netdata Parents](/docs/netdata-agent/sizing-netdata-agents/disk-requirements-and-retention.md)
- [Exporting reference](/src/exporting/README.md)
- [Notifications](/docs/alerts-and-notifications/notifications/README.md)
