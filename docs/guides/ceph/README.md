# Monitor Ceph

Deploy Netdata close to each Ceph node, then collect the three complementary Ceph telemetry surfaces: the MGR Prometheus module, `ceph-exporter`, and the native Ceph Dashboard API collector. This guide describes the Phase 1 deployment, ownership boundaries, alert behavior, retention planning, and integration paths.

## Deployment model

Run one Agent on each Ceph node. Ceph telemetry is distributed: a single Agent cannot observe every daemon's admin socket or every host's disks, interfaces, logs, and processes.

| Surface | Deployment | Ownership |
|---|---|---|
| MGR Prometheus module | One logical job per Ceph cluster | Cluster health, quorum, capacity, placement groups, pools, RGW aggregate telemetry, and NVMe-oF exporter-local telemetry |
| `ceph-exporter` | One job on each node that should expose daemon/admin-socket telemetry | Host-local daemon performance and daemon inventory |
| Native Ceph Dashboard API | One logical job per Ceph cluster | Dashboard API component integrity and Ceph RCA Functions |
| Host collectors | Every Agent | Node disks, filesystems, network interfaces, processes, and logs |

Use one stable Prometheus job identity for the MGR surface. If the active MGR moves, update DNS or the reverse proxy to the current active endpoint rather than creating one job for each possible MGR. Multiple active MGR jobs for the same cluster create duplicate cluster alert owners.

For `ceph-exporter`, preserve each node's own job or vnode identity. Do not merge exporter jobs across hosts; their signals are host-local.

A Parent can receive streams from all Ceph Agents. Parenting changes where data is stored and queried, not the identity of each Ceph node: child charts retain their node scope, labels, and alert instances. Organize Cloud rooms by site, cluster, or role, and use the combined view when you need an infrastructure-wide investigation.

## Enable the telemetry surfaces

### MGR Prometheus module

Enable the Ceph MGR Prometheus module and expose one stable HTTPS or HTTP endpoint for the cluster. Configure the Netdata Prometheus collector to scrape that endpoint. The built-in Ceph profile matches the official Ceph metric surfaces automatically.

For high availability, put a stable address in front of the active MGR and keep the Netdata job name and vnode identity stable. Do not create duplicate jobs for standby MGR endpoints.

### ceph-exporter

On Reef and later, deploy or enable `ceph-exporter` on each Ceph node whose daemon telemetry you need. Collect from each exporter on its own host. The exporter's priority limit controls which daemon counters it publishes; size `max_time_series` and `max_time_series_per_metric` for the selected surface.

### Native Ceph Dashboard API

Enable the Ceph Dashboard module, secure it with TLS, and create a read-only Dashboard user. Configure one native Ceph collector job per Ceph cluster. This collector complements Prometheus rather than replacing it: it owns API component integrity and provides Ceph RCA Functions.

## Supported release surfaces

Phase 1 covers the public metric surfaces of:

- Reef 18.2.8
- Squid 19.2.5
- Tentacle 20.2.3

The profiles and alerts are source-pinned to these releases. Some telemetry is release-specific:

- Reef and Squid provide the core MGR and exporter surfaces.
- Tentacle adds node-proxy hardware, NVMe-oF local gateway signals, and other newer metrics.
- Tentacle-only charts and alerts do not appear on Reef or Squid when those series are absent.

## Permissions, TLS, and routing

- Give the Dashboard user read access to the required Ceph scopes. The built-in read-only Dashboard role is sufficient for the native collector.
- Use TLS for Dashboard and MGR endpoints where practical.
- Do not point a job at a generic reverse-proxy login page. The URL must reach the Ceph API or metrics endpoint directly.
- Preserve stable job identity through redirects and failover. Chart identity and alert ownership depend on it.
- If Ceph redirects HTTP to HTTPS, configure the final HTTPS URL explicitly rather than relying on redirect following.

## Collection cadence and limits

Netdata scrape frequency does not control Ceph producer refresh frequency.

- The MGR Prometheus module refreshes its metric cache on its own Ceph-side interval.
- `ceph-exporter` refreshes daemon metrics on its own configured interval.
- Netdata samples the exposed current cache at the job's `update_every`.

Match the Netdata interval to the producer cache interval when redundant samples are undesirable. A faster scrape does not force Ceph to perform more work or refresh more often.

The Ceph profile can expose a large metric surface. Set job-level `max_time_series` and `max_time_series_per_metric` after measuring your release and exporter configuration. Do not enable broad optional debug or detail surfaces unless you have sized the resulting chart and dimension cardinality.

## Telemetry and alert ownership

The MGR Prometheus profile owns the cluster-level Phase 1 alerts:

- cluster health and MGR module health;
- monitor quorum and OSD population summaries;
- placement group, pool, capacity, and recovery conditions;
- Tentacle hardware and NVMe-oF local conditions;
- RGW notification, Lua, request-fallback, queue-pressure, and multisite retry alerts.

The native Dashboard API collector owns component collection failure and does not duplicate MGR data-state alerts. Generic collectors remain the single owners of host-local and endpoint checks:

- host disks, filesystems, and network interfaces;
- `httpcheck` for basic unauthenticated RGW endpoint liveness;
- `x509check` for RGW certificate expiration and revocation;
- `weblog` for complete RGW access logs, including the `total_time` numeric field in milliseconds.

`httpcheck` does not prove authenticated S3 correctness. For authenticated object PUT/GET/LIST/DELETE verification, use the Phase 2 S3 check capability.

### Silent and notifying defaults

Many Phase 1 conditions are silent by default because their threshold is workload policy rather than a categorical error. Examples include:

- queue depth and notification backlog pressure;
- storage capacity policy;
- traffic anomalies;
- client-error rates;
- some hardware temperature thresholds.

Categorical or hard-failure conditions notify by default. Route silent alerts explicitly only after tuning their thresholds for your environment. Override routing without changing stock thresholds by copying the health template into the local health configuration.

### Collection gaps and disappearance

A failed scrape does not fabricate a healthy value. Data-state alerts may continue to evaluate the last stored value, while the generic collection alert owns the scrape failure. When Ceph removes a chart and the collector obsoletes it, its instance alerts are removed rather than falsely clearing.

## RGW and S3 monitoring

For RGW, the MGR profile provides aggregate telemetry for requests, aborted requests, Lua scripts, notifications, queues, and multisite errors.

For HTTP outcomes and latency, collect the RGW access log with `weblog`. The Ceph JSON log example maps request, status, size, and client fields, and declares `total_time` as a numeric custom field in milliseconds. This preserves the exact Ceph duration field instead of misusing the standard integer request-time field.

Use `httpcheck` for unauthenticated endpoint liveness and `x509check` for certificate expiry or revocation. For authenticated S3 correctness, multisite payload checks, and delete propagation, use the Phase 2 capabilities rather than approximating them from counters.

### Phase 1 limits

- Per-topic persistent notification backlog has no source-published Prometheus numeric family in the covered releases.
- Selected tenant, user, and bucket detail is not enabled automatically because those optional surfaces can be high-cardinality.
- Quota, sync lag, recovery shards, RPO, lifecycle health, and garbage-collection age require the Phase 2 native or probe surfaces.

## RCA Functions

The native Ceph Dashboard collector provides Functions for detailed health, OSD, pool, and incident investigation. These Functions are on-demand API calls; they do not create charts or alerts by themselves.

Use Functions when you need detailed current inventory or component state. Use charts and alerts for continuous monitoring. If a Function requires the Ceph orchestrator, ensure the Dashboard configuration exposes it.

## Retention and capacity planning

Plan retention separately on Children and Parents.

1. Deploy one Agent per Ceph node and run a representative workload for days.
2. Measure the number of charts and dimensions actually produced for your Ceph release, optional exporter surfaces, and collectors.
3. Decide where long retention lives:
   - keep short high-resolution retention locally on each Child;
   - stream longer retention to one or more Parents.
4. Use the measured chart and dimension counts, sample interval, tiering mode, and intended retention horizon to estimate disk requirements.
5. Validate the estimate after several days and again after several weeks.
6. For multi-year storage, export to Prometheus, Grafana-compatible storage, or another external system rather than storing every sample in the Agent database.

Do not extrapolate from a single quiet day: hardware inventories, failover, optional exporter surfaces, and incident activity change cardinality. Do not assume linear growth from Ceph capacity alone; chart cardinality follows enabled telemetry surfaces and their labels.

Phase 1 does not forecast storage growth or capacity exhaustion. Use the measured charts and exported long-term data for trend analysis until a supported prediction capability exists.

## Cloud, rooms, and sites

Connect each Agent to Netdata Cloud unless you operate a fully on-prem deployment. Organize rooms by site, cluster, or operational responsibility. A room is a navigation and access boundary, not a data aggregation layer.

Parents preserve child identity. A Parent can provide long retention, unified querying, and a single TLS ingress, while each Ceph node remains independently visible in Cloud.

For on-prem-only deployments, streaming and local dashboards remain available. You lose Cloud-managed rooms, centralized silencing, and Cloud notifications, but local Agent health and notification mechanisms still work.

## Grafana, Prometheus, Zabbix, and exporting

The Ceph collectors coexist with existing monitoring:

- Keep or deploy Grafana dashboards independently.
- Keep existing Prometheus exporters; the Prometheus collector reads Ceph's official endpoints but does not replace them.
- Keep an existing Zabbix deployment independently and collect Ceph through Zabbix's own integrations. Netdata does not
  provide a direct Zabbix exporting connector.
- Export Netdata metrics to external systems with the exporting engine.
- Send selected Netdata alerts to external systems through supported notification integrations.

Do not point multiple Netdata jobs at the same logical Ceph cluster MGR endpoint under different identities unless you intentionally want separate alert owners.

## Notifications

Route alert notifications centrally through Netdata Cloud when your Agents are connected, or configure notifications per Agent for on-prem deployments. Start with the notifying categorical alerts, then enable and tune silent policy alerts only where you have measured a meaningful threshold.

## Incident guide

For the native physical-capacity alert, see the [Ceph cluster space usage](/src/health/guides/ceph/ceph_cluster_space_usage.md) incident guide.

For collector configuration details, see:

- [Ceph](/src/go/plugin/go.d/collector/ceph/integrations/ceph.md)
- [Ceph Prometheus](/src/go/plugin/go.d/collector/prometheus/integrations/ceph_prometheus.md)

For Agent deployment and streaming, see:

- [Deployment with centralization points](/docs/deployment-guides/deployment-with-centralization-points.md)
- [Metrics centralization points](/docs/observability-centralization-points/metrics-centralization-points/README.md)
- [Sizing Netdata Parents](/docs/netdata-agent/sizing-netdata-agents/disk-requirements-and-retention.md)
- [Exporting reference](/src/exporting/README.md)
- [Notifications](/docs/alerts-and-notifications/notifications/README.md)
