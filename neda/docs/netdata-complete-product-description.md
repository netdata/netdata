# Netdata: Complete Product Description

## Executive Summary

Netdata is a distributed, real-time observability platform that fundamentally reimagines infrastructure monitoring. Unlike traditional solutions that centralize data and create bottlenecks, Netdata distributes intelligence to the edge—processing, storing, and analyzing data where it's generated. This revolutionary architecture delivers **per-second monitoring with sub-2-second latency** at any scale, from single nodes to 100,000+ systems, while consuming minimal resources and maintaining predictable costs.

**Core Value Proposition**: Netdata provides complete observability—metrics, logs, and AI-powered insights—with zero configuration, automated dashboards, machine learning anomaly detection on every metric, and natural language troubleshooting, all while keeping your data sovereign and secure on your infrastructure.

## 1. Platform Architecture

### 1.1 Distributed Edge-Native Design

Netdata's architecture is built on a fundamental principle: **distribute the code, instead of centralizing the data**. This approach eliminates the scaling challenges, cost explosions, and performance bottlenecks inherent in centralized monitoring systems.

**Four Core Components:**

1. **Netdata Agent (ND)** - Complete observability engine on each monitored system
   - Collects 3,000-20,000 metrics per second per node (there is no upper limit: 20k is indicative for real world deployments)
   - Stores data locally in multi-tier time-series database (years of retention)
   - Trains 18 ML models locally per metric for anomaly detection
   - Evaluates health checks, triggers alerts, sends notifications (optional, enabled email by default), runs scripts (optional)
   - Serves local dashboards and REST API (single-node)
   - Streams data to Parents for centralization (optional, 1 parent at a time, with multiple configured as fallback)
   - Exports data to multiple 3rd party TSDBs at once (optional)
   - Is a Model Context Protocol (MCP) server (single-node, streamable http, sse, websocket - stdio with a bridge)
   - Resource usage: <5% CPU, 150-200 MB RAM (including ML). Memory is proportional to metrics discovered (~5000 metrics baseline) and affected by metric ephemerality. Resources scale linearly per metric. Can be tuned to <1% CPU, <100 MB RAM (offloading to parents and 32-bit IoT)
  
   Comparison with other monitoring agents at: https://www.netdata.cloud/blog/netdata-vs-datadog-dynatrace-instana-grafana/

2. **Netdata Parents** - Optional centralization points - same S/W as the Netdata Agent
   - Aggregate data from multiple Agents
   - Provide data replication and high availability
   - Handle millions of metrics per second
   - Support active-active clustering (2+ nodes in circular topology)
   - Optionally offload ML training, alerting, and storage from production systems
   - Serves local dashboards and REST API (multi-node)
   - Is a Model Context Protocol (MCP) server (multi-node, same protocols)
   - Resource usage: ~10 cores, ~40 GB RAM per million metrics/second

   Comparison with Prometheus at scale: https://www.netdata.cloud/blog/netdata-vs-prometheus-2025/

3. **Netdata Cloud (NC)** - Smart control plane (SaaS or On-Premise)
   - Unifies access across distributed Agents and Parents
   - Provides horizontal scalability (distributes queries to Agents and Parents)
   - Provides RBAC
   - Spaces (natural isolation, separate billing) and (War-)Rooms (logical isolation on shared infrastructure)
   - Facilitates team collaboration and isolation
   - Enables infrastructure-level dashboards
   - Centralizes alert notifications (Agents and Parents stream alert transitions to it, and they get deduplicated)
   - Provides auditing of infrastructure changes and user actions
   - Offers managed AI features (Netdata optimized playbooks)
   - Provides SSO to Netdata Agents and Parents.
   - **Critical**: No metric or log data stored in Cloud—only metadata. Queries sent to NC are distributed to Agents and Parents.

4. **Netdata UI** - Dashboards and Visualization
   - Same UI works on Agents (single-node), Parents (multi-node), and Netdata Cloud (infrastructure-wide)
   - Algorithmic dashboards generated based on the data available
   - NIDL-framework: slice and dice any dataset with point and click (no query language required)
   - Each chart is a complete analytical tool, providing 360 views of the underlying dataset, equivalent to 25+ Grafana charts
   - ML based anomaly detection is first class citizen (on every chart: anomaly ribbon on every chart, sorting by anomaly on any chart dimension and source, anomaly advisor - RCA, correlations on anomaly, "what is interesting now?" based on anomalies)
   - Metrics + Logs + Alerts + Correlations (RCA, blast radius detection) + Netdata Functions

**Key Architectural Benefits:**
- **Linear scalability**: Adding nodes doesn't affect existing performance
- **No single point of failure**: Each Agent operates independently
- **Data sovereignty**: All observability data stays on-premises
- **Predictable costs**: Resources scale linearly with infrastructure
- **Zero data loss**: 100% sample completeness, no sampling or downsampling
- **80% MTTR reduction**: special scoring engine identifies the needle in the heystack
- **Extreme high-availability**: Netdata Agents and Parents provide the same UI directly even on connectivity loss and network partitioning

### 1.2 Real-Time Performance

Netdata defines the industry standard for real-time monitoring:

- **1-second data collection** across all metrics
- **1-second visualization latency** from collection to dashboard
- **Sub-2-second total latency** from event to insight (worst case)
- **10-60× faster** than typical monitoring solutions

This real-time capability is critical because:
- Most operational anomalies last 2-10 seconds
- Traditional 30-second monitoring misses 90% of incidents
- Engineers need immediate feedback during troubleshooting
- Microbursts and transient issues are invisible to averaged data

**Independent Validation**: University of Amsterdam study (ICSOC 2023) confirmed Netdata as the most energy-efficient monitoring solution with the lowest CPU and memory overhead—even while collecting per-second data and running ML at the edge.

## 2. Observability Coverage & Collectors

### 2.1 Observability Pillars & Scope

- **Metrics**: Native per-second collection across hosts, containers, services, hardware, and custom workloads; exported via REST API v3, streaming, and integrations.
- **Logs & Events**: systemd-journal ingestion on Linux, Windows Event Logs/ETW/TL, Kubernetes events, Netdata health events, and cloud alert streams; correlated directly with metrics.
- **Alerts**: Edge-evaluated health checks with state transitions streamed to Parents and Cloud; anomaly ribbons embedded in each metric sample.
- **Traces**: Netdata does not yet ship distributed tracing.
- **Metadata**: Node inventory, topology (Parent/Child), ML model statistics, configuration state, RBAC assignments, and audit logs exposed through Cloud and REST APIs.

### 2.2 Collector Frameworks & Execution Model

Netdata ships purpose-built collector frameworks that share a unified lifecycle: detect → configure → collect → enrich → store locally → stream upstream.

| Framework | Language | Primary Scope | Notes |
|-----------|----------|---------------|-------|
| `proc.plugin` | C | Core OS/procfs metrics (CPU, RAM, disks, filesystems, networking) | Always-on; zero config |
| `apps.plugin` | C | Process, service, and application grouping | Uses cgroups, namespaces, container metadata |
| `cgroups.plugin` | C | Container and VM resource accounting | Native cgroup v1/v2 support |
| `go.d` | Go | Network/service-facing collectors (200+ integrations) | Auto-discovery, parallel polling |
| `scripts.d` | Go | Nagios plugins scheduler and collector (4000+ integrations) | Vast ecosystem and simple protocol for scripts execution and synthetic tests |
| `otelcol.plugin` | Go | OpenTelemetry collector distro (metrics/logs ingestion only) | Built from upstream OTel Collector with Netdata receivers |
| `otel-plugin` | Rust | OTel intake for metrics and logs (writes systemd-journal compatible files) | Rust journald writer preserving native format |
| `python.d` | Python | Long-tail integrations & IoT sensors | Ideal for quick custom collectors |
| `charts.d` | Bash | Lightweight shell-based collectors | Legacy but still maintained |
| `ebpf.plugin` | C/eBPF | Kernel instrumentation (syscalls, I/O, networking - does not in containers or kubernetes) | Requires kernel ≥4.11 |
| `statsd` | C | Ingest StatsD metrics over UDP | Supports tagging via labels |
| `log2journal` | C | Log transformation into journald | Provides structured fields |
| `systemd-journal` | C | Journald reader & indexer | Zero pipeline logging |
| `windows.plugin` | C++ | Windows performance counters | Includes Hyper-V, AD, IIS |
| `windows-events` | C++ | Windows Event Log ingestion | Supports ETW and TraceLogging |
| `nfacct`, `perf`, `tc`, `idlejitter`, `timex`, etc. | C | Specialized kernel data sources | Ship disabled unless detected |

**Execution Features:**
- Unified scheduler with adaptive sampling, backoff, and fail-safe timeouts.
- Auto-discovery: go.d and python.d scan endpoints (files, sockets, APIs) and propose configs via Cloud UI.
- Hot reload: changes in `go.d.conf`, `python.d.conf`, or Cloud configuration apply without daemon restart.
- Sandboxing: collectors run as unprivileged users with capability drops; Windows plug-ins leverage per-service ACLs.
- Multi-source correlation: metrics tagged with labels (region, role, namespace) for NIDL slicing and streaming aggregation.

### 2.3 Collector Catalog (As of October 2025)

**go.d collectors:** `activemq`, `adaptecraid`, `ap`, `apache`, `apcupsd`, `beanstalk`, `bind`, `boinc`, `cassandra`, `ceph`, `chrony`, `clickhouse`, `cockroachdb`, `consul`, `coredns`, `couchbase`, `couchdb`, `dmcache`, `dnsdist`, `dnsmasq`, `dnsmasq_dhcp`, `dnsquery`, `docker`, `docker_engine`, `dockerhub`, `dovecot`, `elasticsearch`, `envoy`, `ethtool`, `exim`, `fail2ban`, `filecheck`, `fluentd`, `freeradius`, `gearman`, `geth`, `haproxy`, `hddtemp`, `hdfs`, `hpssa`, `httpcheck`, `icecast`, `intelgpu`, `ipfs`, `isc_dhcpd`, `k8s_kubelet`, `k8s_kubeproxy`, `k8s_state`, `lighttpd`, `litespeed`, `logind`, `logstash`, `lvm`, `maxscale`, `megacli`, `memcached`, `mongodb`, `monit`, `mysql`, `nats`, `nginx`, `nginxplus`, `nginxunit`, `nginxvts`, `nsd`, `ntpd`, `nvidia_smi`, `nvme`, `openldap`, `openvpn`, `openvpn_status_log`, `oracledb`, `pgbouncer`, `phpdaemon`, `phpfpm`, `pihole`, `pika`, `ping`, `portcheck`, `postfix`, `postgres`, `powerdns`, `powerdns_recursor`, `prometheus`, `proxysql`, `pulsar`, `puppet`, `rabbitmq`, `redis`, `rethinkdb`, `riakkv`, `rspamd`, `samba`, `scaleio`, `sensors`, `smartctl`, `snmp`, `spigotmc`, `squid`, `squidlog`, `storcli`, `supervisord`, `systemdunits`, `tengine`, `testrandom`, `tomcat`, `tor`, `traefik`, `typesense`, `unbound`, `upsd`, `uwsgi`, `varnish`, `vcsa`, `vernemq`, `vsphere`, `w1sensor`, `weblog`, `whoisquery`, `wireguard`, `x509check`, `yugabytedb`, `zfspool`, `zookeeper` and configurable SQL collector (collects metrics by running user-configurable SQL queries).

**python.d collectors:** `am2320`, `go_expvar`, `haproxy`, `pandas`, `traefik` (legacy modules remain in community repos; new development favors go.d).

**charts.d collectors:** Deployed for services addressable via shell: `apache`, `apcupsd`, `beanstalk`, `cpufreq`, `dovecot`, `exim`, `freeradius`, `gpsd`, `icecast`, `lm_sensors`, `mdstat`, `megacli`, `mysql`, `nginx`, `nut`, `opensips`, `phpfpm`, `postfix`, `powerdns`, `redis`, `samba`, `snmp`, `tor`, `varnish`, `zfs`, and more (legacy but still shipped for backwards compatibility).

**apps.plugin groups:** Auto-classifies processes into system, services, containers, VMs, orchestrator workloads, and per-user sessions; supports custom grouping via `/etc/netdata/apps_groups.conf`.

**Kernel & hardware collectors:** `cgroups.plugin`, `perf`, `tc`, `idlejitter`, `timex`, `slabinfo`, `nfacct`, `ebpf.plugin`, IPMI via `freeipmi`, `sensors` (lm-sensors), `nvme`, `smartctl`, `windows.plugin` (PerfCounters), Hyper-V, GPUs (NVIDIA/DCGM, Intel i915/iGPU, AMD via amdgpu metrics).

**Log ingestion:** `systemd-journal`, `windows-events`, `otelcol.plugin`, `otel-plugin`, Kubernetes events (go.d), `log2journal` transformer, and third-party shipping via journald uploaders.

**Synthetic & custom inputs:** StatsD, Prometheus remote-write, OpenTelemetry metrics/logs via `otelcol.plugin`/`otel-plugin`, HTTP JSON, command execution (`exec` collectors), MCP-triggered functions, and external scripts via `collector.conf` stanzas.

**otel-plugin:** OpenTelemetry collector for metrics and logs (traces is planned for the next quarter).

**scripts.d.plugin:** Run unmodified Nagios plugins for collecting metrics, logs and triggering alerts based on Nagios plugins statuses.

_Source of truth_: `netdata/src/collectors` (C plug-ins), `netdata/src/go/plugin/go.d/collector`, and `netdata/src/collectors/python.d.plugin` contain the authoritative inventories; regenerate this section when new collectors ship.

### 2.4 Coverage Snapshot

- **Operating Systems**: Linux (kernel ≥2.6.32), Windows Server 2016/2019/2022/2025 and Windows 10+, macOS ≥10.12, FreeBSD ≥11.
- **Infrastructure**: CPU, memory, storage, network, scheduler, NUMA, power/thermal sensors, firewall counters, QoS (tc), kernel schedulers, virtualization layers.
- **Containers & Orchestration**: Docker/Moby, containerd, Podman, LXC/LXD, Kubernetes (nodes/pods/services/objects), OpenShift, Nomad (via Prometheus), systemd nspawn; cgroups v1/v2 introspection with lifecycle tracking.
- **Cloud Providers**: AWS (EC2/ECS/EKS, CloudWatch metrics), Azure (VM, AKS, Monitor), GCP (GCE/GKE), DigitalOcean, OVH; additional providers integrated through generic REST/HTTP JSON collectors or Prometheus/OpenMetrics endpoints.
- **Applications**: Web servers, databases, caches, queues, storage backends, service mesh (Envoy, Traefik), identity (OpenLDAP), mail (Postfix, Dovecot), search (Elasticsearch, Solr), blockchain (geth), analytics (ClickHouse, Kafka ecosystem).
- **Network & Edge**: SNMPv1/v2c/v3 devices, routers, firewalls, load balancers, SD-WAN, IoT sensors (1-wire, Modbus via scripts), UPS/PDUs, syslog feeds for network appliances.
- **Windows Specialties**: Active Directory, IIS, Hyper-V, Exchange, SQL Server, DFS, Windows Defender, Print Server; full Event Log coverage.
- **Security & Compliance Inputs**: Fail2ban, auditd via journald, IDS feeds via SNMP/Prometheus, certificate expiry via `x509check`.

### 2.5 Zero-Configuration Philosophy

Netdata's auto-discovery eliminates manual configuration:

1. **Automatic detection** of all available metrics sources
2. **Instant dashboard generation** for discovered services
3. **Pre-configured alerts** for the most popular collectors
4. **Dynamic adaptation** as infrastructure changes
5. **No query languages required** (no PromQL, no SQL)

**Configuration only needed for:**
- Services requiring authentication (databases, APIs)
- Custom application metrics
- Advanced alert customization
- Parent-child streaming relationships
- Retention configuration

### 2.6 Application Observability & APM Scope

- **Zero-code application insights**: `apps.plugin` (process/resource aggregation) and `ebpf.plugin` (kernel-level per-process telemetry - does not run in containers or kubernetes) deliver per-second CPU, memory, I/O, networking, OOM, and syscall visibility without touching application code.
- **Logs as first-class signals**: systemd-journal, `log2journal`, `weblog`, `otelcol.plugin`, and `otel-plugin` provide raw log search and logs-to-metrics conversion for web services, applications, and cloud-native sources.
- **Custom metrics ingestion**: Prometheus/OpenMetrics scraping, StatsD, and OTLP metrics via `otel-plugin` let instrumented code feed business or application KPIs into Netdata dashboards.
- **Synthetic monitoring**: go.d collectors like `httpcheck`, `ping`, `portcheck`, `dnsquery`, and `testrandom` run uptime/performance probes against services, APIs, and endpoints, feeding directly into alerts and dashboards + `scripts.d` provides access to 4000+ Nagios plugins.
- **APM gaps (explicit)**: No distributed tracing or span storage today (OTLP trace ingestion planned for Q2 2026), no code-level profiling/flame graphs, no transactional/user-experience monitoring, and limited error tracking (logs/process crashes only).
- **Positioning**: Netdata bridges infrastructure monitoring and APM—ideal for zero-instrumentation observability, but designed to complement trace-centric APM platforms for microservice call flows and deep code analysis.

## 3. Storage & Data Management

### 3.1 Multi-Tier Time-Series Database (DBENGINE)

Netdata's custom database engine is the default storage backend for both Netdata Agents and Parents and is optimized for edge deployment:

**Three Storage Tiers (default):**

| Tier | Resolution | Default Retention | Storage Efficiency | Use Case |
|------|------------|-------------------|-------------------|----------|
| **Tier 0** | Per-second | 14 days | ~0.6 bytes/sample | Recent troubleshooting |
| **Tier 1** | Per-minute | 3 months | ~4 bytes/sample | Medium-term trends |
| **Tier 2** | Per-hour | 2 years | ~4 bytes/sample | Long-term capacity planning |

**Key Features:**
- **All tiers updated in parallel** during data collection
- **No post-processing** or compaction jobs required
- **Write-once-read-many (WORM)** design for reliability
- **Gorilla + ZSTD compression** for maximum efficiency
- **Configurable retention** per tier (size and/or time-based)
- **Automatic tier rotation** when limits reached

**Storage Efficiency:**
- Industry-leading compression: ~0.6 bytes per sample (Tier 0)
- Years of data in gigabytes, not terabytes
- Minimal disk I/O: ~5 KiB/s per Agent, ~1 MiB/s per Parent (at 1M metrics/s)
- Append-only writes spread evenly over time (no I/O spikes)

**Data Point Structure:**
Each sample includes:
- Value (with gap indicator for missed collections)
- Timestamp (microsecond precision)
- Duration (time since previous collection)
- Anomaly flag (ML classification result)

For aggregated tiers, each point also includes:
- Sum, count, min, max values
- Anomaly rate (percentage of anomalous samples)

**Database Modes:**
- `dbengine`: Multi-tier persistent storage (default for production)
- `ram`: In-memory only, no disk I/O (for IoT/edge devices)
- `none`: No storage, streaming only (ultra-minimal footprint)

### 3.2 Performance Comparison

**Netdata Parent vs Prometheus** (at 4.6 million metrics/second):

| Metric | Netdata | Prometheus | Advantage |
|--------|---------|------------|-----------|
| CPU Usage | ~9.4 cores | ~14.8 cores | **36% less** |
| Memory Usage | ~47 GB | ~383 GB | **88% less** |
| Disk I/O | ~4.7 MB/s | ~147 MB/s | **97% less** |
| Retention (1TB) | ~1.25 days | ~2 hours | **15× longer** |
| Sample Completeness | ~100% | ~93.7% | **Zero data loss** |
| Query Latency (2hr) | ~0.11s | ~1.8s | **16× faster** |

## 4. Machine Learning & Anomaly Detection

### 4.1 Edge-Based ML Architecture

Netdata runs unsupervised machine learning **directly on each Agent**, eliminating cloud dependencies and data egress:

**Algorithm**: k-means clustering (k=2) via dlib library

**Key Characteristics:**
- **18 models per metric** trained on different time windows
- **6-hour training windows** with 3-hour staggered intervals
- **Consensus-based detection**: Anomaly flagged only when ALL models agree
- **99% false positive reduction** through multi-model consensus
- **Theoretical false positive rate**: 10^-36 (1% per model ^ 18 models)
- **Continuous training**: Models retrain every 3 hours automatically
- **Zero configuration**: Works out of the box for all metrics

**Resource Usage:**
- ~18 KB RAM per metric
- 2-4% CPU for 10,000 metrics
- Runs as low-priority background task
- Automatically yields to data collection

**Anomaly Types Detected:**
- Point anomalies (unusual high/low values)
- Contextual anomalies (pattern deviations)
- Collective anomalies (multivariate issues)
- Concept drifts (gradual baseline shifts)
- Change points (sudden state changes)

**Storage:**
- Anomaly bit stored with each metric sample
- Only 1 bit per metric per second
- Enables historical anomaly analysis
- Powers Anomaly Advisor correlation engine

### 4.2 Anomaly Advisor

Real-time correlation engine that transforms troubleshooting:

**Capabilities:**
- Correlates anomalies across all metrics within each node
- Generates node-level anomaly charts
- Ranks thousands of metrics by anomaly severity
- Surfaces root cause in top 30-50 results
- Reveals sequence of cascading failures
- Determines blast radius of incidents

**Workflow:**
1. Select time window on any chart (≥15 seconds)
2. System compares to baseline (4× window length)
3. Evaluates all available metrics
4. Returns filtered dashboard with most changed metrics
5. Provides AI-ready insights for further analysis

## 5. AI-Powered Troubleshooting

### 5.1 Netdata AI Assistant

CRITICAL: AI assistants are significantly more effective with Netdata, for the following reasons:

1. Real-time ML-based anomaly detection for all metrics - which is unbiased of future events (anomaly is committed to disk together with samples in real-time, during data collection, and is then queried)
2. High resolution metrics
3. Bottom-up component level alerts, providing "silent" transitions for hundreds of events, revealing abnormal or unusual insights
4. Metric queries in Netdata always return anomaly information together with aggregated samples, so that AI assistant get immediate visibility to anomalies even when they don't specifically ask for it.
 
This combination allows AI assistants to quickly spot what is relevant, reveal interdependencies even between seemingly unrelated components, identify the sequence of events in detail and determine the blast radius of incidents across the infrastructure.

Integrated AI capabilities that accelerate incident resolution:

**AI Insights** (Automated Reports):
- **Infrastructure Summary**: High-level health overview
- **Anomaly Analysis**: Pattern identification in anomalous behavior
- **Capacity Planning**: Predict resource exhaustion with recommendations
- **Performance Optimization**: Identify bottlenecks with specific tuning commands
- **Scheduled Reports**: Automated periodic analysis and delivery
- **PDF Export**: Professional documents ready for stakeholders

**AI Investigations / AI Troubleshooting**:
Dynamic and adaptive investigations on any screen, any graph, any dashboard. Global "AI Troubleshoot" button, provides:

- **Custom Investigations**: User-initiated deep dives into specific issues
- **Scheduled Investigations**: Automated periodic analysis
- **Root Cause Analysis**: Complete incident timeline with cascading effects
- **Blast Radius Detection**: Visualize impact spread across infrastructure

**AI Chat** (Natural Language Interface):
- Ask questions about infrastructure in plain language
- Context-aware responses specific to your systems
- No query languages required (no PromQL, SQL, or custom DSLs)
- Integrated directly in dashboard and CLI

**LLM Support:**
- Netdata Managed (Netdata Cloud)
- Bring Your Own LLM (BYOLLM) via Model Context Protocol (MCP) on Netdata Agents and Parents

### 5.2 Model Context Protocol (MCP) Integration

Every Netdata Agent/Parent (v2.6.0+) is an MCP server:

**Capabilities:**
- **Node Discovery**: Hardware specs, OS details, streaming topology
- **Metrics Discovery**: Full-text search across contexts, instances, dimensions
- **Function Discovery**: Access to system functions (processes, network, logs)
- **Alert Discovery**: Real-time alert visibility
- **Metrics Queries**: Complex aggregations and grouping

**Supported MCP Clients:**
- Claude Desktop, Claude Code
- VS Code, Cursor, JetBrains IDEs
- Gemini CLI, Codex CLI
- Custom integrations via MCP protocol

### 5.3 Business Value
- Junior engineers get senior-level insights automatically
- Faster MTTR through natural language troubleshooting
- No specialized query language training required
- AI assistants have complete infrastructure context

### 5.4 AI Guardrails & Automation Workflow

- **LLM connectors**: Cloud-managed integrations for Claude, GPT, Gemini; BYO LLM connections via MCP keep credentials under customer control.
- **Data locality**: AI queries stream live context from Agents/Parents; Netdata Cloud persists only metadata and investigation output, never raw metrics or logs.
- **Transparency**: Each AI response links to the charts, nodes, and alerts it used, and conversations are stored in the Cloud audit log for review.
- **Automation hooks**: Playbooks and scheduled investigations can trigger Netdata Functions or external webhooks, but execution policies are fully user-defined.
- **Access controls**: AI capabilities require explicit RBAC permissions; usage is rate-limited per plan and observable via billing telemetry.

## 6. Alerting & Notifications

### 6.1 Intelligent Health Monitoring

**Pre-Configured Alerts:**
- 400+ alert templates out of the box
- Covers most popular integrations
- Based on real-world operational experience
- Intelligent thresholds that adapt to workload patterns (rolling windows, anomalies - not fixed thresholds)

**Alert Architecture:**
- Health engine evaluates rules every second
- Alerts run at the edge (Agent or Parent level)
- Supports local automation scripts (e.g. run this script on critical)
- Flexible notification dispatch via Cloud, Parents, or Agents (or all of them)

**Alert Severity Levels:**
- **CLEAR**: Metric returned to normal
- **WARNING**: Concerning behavior (investigate during business hours)
- **CRITICAL**: Serious problem (immediate response required)

**Alert Intelligence:**
- Hysteresis protection prevents notification floods
- Configurable notification delays avoid transient alerts
- Role-based routing ensures alerts reach appropriate stakeholders
- Dynamic configuration via Cloud UI (no restarts required)

**Alert Fatigue**
- Netdata provides component level alerts which are usually more accurate and actionable that generic fixed threshold alerts.
- Netdata does not have any features targeting alert fatigue, other than grouping alerts by category, class, or any label at the dashboards.
- ML based anomaly detection in Netdata is not directly used to reduce alert fatigue. Anomaly Detection is another signal, not a means to supress other signals. Netdata fully supports anomaly detection based alerts, but there is not correlation or filtering of non-ML based alerts with machine learning.

### 6.2 Notification Integrations

**Netdata Agent/Parent (edge) integrations** — implemented in `src/health/notifications/`:
- Email / SMTP (`email`, default `alarm-email.sh`)
- Amazon SNS (`awssns`)
- Alerta (`alerta`)
- Discord (`discord`)
- Dynatrace (`dynatrace`)
- Flock (`flock`)
- Gotify (`gotify`)
- iLert (`ilert`)
- IRC (`irc`)
- Kavenegar SMS (`kavenegar`)
- Matrix (`matrix`)
- MessageBird (`messagebird`)
- Microsoft Teams (`msteams`)
- ntfy (`ntfy`)
- Opsgenie (`opsgenie`)
- PagerDuty (`pagerduty`)
- Prowl (`prowl`)
- Pushbullet (`pushbullet`)
- Pushover (`pushover`)
- Rocket.Chat (`rocketchat`)
- SIGNL4 (`signl4`)
- Slack (`slack`)
- SMSEagle (`smseagle`)
- SMSTools3 (`smstools3`)
- Syslog (`syslog`)
- Telegram (`telegram`)
- Twilio SMS/Voice (`twilio`)
- Generic webhooks (`web`)
- Custom script hook (`custom`)

**Netdata Cloud centralized methods** — configured per Space in the Cloud UI (`cloud-netdata-assistant/docs/...`), with plan availability:
- Email (personal channel, available on every plan)
- Discord webhooks (Community+)
- Generic webhook with signed payloads (Pro+)
- Mattermost webhooks (Business+)
- Rocket.Chat webhooks (Business+)
- Slack webhooks (Business+)
- PagerDuty (Business+)
- Opsgenie (Business+)

Cloud rules support personal vs system channels, room scoping, severity filters, silencing windows, and flood protection. Edge notifications retain role-based routing, templated messages, and the ability to run automations on transition scripts.

**Notification Features:**
- Severity-based routing and role targeting
- Multi-channel fan-out with deduplication in Cloud
- Customizable thresholds and escalation policies
- Maintenance window silencing and flood protection
- Alert metadata packaged for downstream automations
- Works both disconnected (Agent) and centralized (Cloud) for resilience

## 7. Logs Management

### 7.1 systemd-journal Integration (Linux)

Netdata provides comprehensive logs management through native systemd-journal integration:

**Key Features:**
- **Zero pipeline architecture**: No log shipping, no central clusters, no ingestion
- **Direct file access**: Queries systemd-journal files where they live
- **Full field indexing**: Every field in every log entry searchable
- **90% cost reduction**: Eliminates Elasticsearch/Splunk infrastructure
- **200× query accuracy**: Analyzes 1M entries before sampling vs 5K for traditional tools
- **Instant correlation**: Logs and metrics from same source, no timestamp matching
- **OpenTelemetry ingestion**: `otelcol.plugin` ingests OTel logs/metrics and hands them to the Rust `otel-plugin` journald writer, keeping native format parity for analysis and streaming.
- **Native SIEM integration**: SIEMs support systemd journals ingestion

Netdata ships a pure Rust journal writer (`journal_file` library) that mirrors systemd's journal format on Linux, Windows, macOS, and FreeBSD, so any OTel, syslog, or native log stream becomes a first-class journal file regardless of platform.

Typical cloud-native pipeline: Fluentd/Fluent Bit/Filebeat/Vector → OTLP gRPC → `otelcol.plugin` → `otel-plugin` → journal files → systemd-journal.plugin → Netdata UI.

**Journal Sources:**
- System journals (kernel, audit, syslog, service units)
- User journals (per-user log streams)
- Namespace journals (isolated per project/service)
- Remote journals (centralized via systemd-journal-remote)
- Network device feeds via syslog (routers, switches, firewalls, load balancers)

**Visualization Features:**
- Real-time streaming with PLAY mode
- Powerful filtering on all journal fields
- Full-text search with wildcards
- Interactive histograms showing log frequency
- Enriched field display (priorities, UIDs, timestamps)
- Multi-node support for centralized analysis

**Log Transformation (log2journal):**
- Converts any structured log format to systemd-journal
- Supports JSON, logfmt, and PCRE2 patterns
- Field extraction, renaming, injection, and rewriting
- Enables structured logging for any application

**Log Centralization:**
- Infrastructure-wide log aggregation
- Uses standard systemd-journal-upload/remote
- Supports encryption with TLS
- Journal namespaces for isolation
- No additional software required

### 7.2 Windows Event Logs

**Native Windows Event Log Support:**
- Windows Event Logs (WEL)
- Event Tracing for Windows (ETW)
- TraceLogging (TL)
- All via native Windows Event Log API

**Features:**
- Automatic detection of all event channels
- Filtering on all System Event fields
- Full-text search on System and User fields
- Histogram visualization with field-value breakdown
- Severity-based coloring
- Real-time "tail" mode

**Event Sources:**
- Application, Security, System, Setup logs
- Admin, Operational, Analytic, Debug channels
- Forwarded events (EventCollector)
- Classic Event Log API channels
- Provider-specific channels

### 7.3 Security Logging & Light SIEM Use Cases

- **Audit preservation**: systemd audit records, Windows Security logs, and custom OTLP feeds are stored in tamper-evident journal files with forward-secure sealing.
- **Incident response**: Full-text search and field filtering across all nodes supports rapid investigations without external tooling.
- **Custom alerting**: Health rules can inspect journald fields, trigger Netdata Functions, or fire webhooks for automated response.
- **Retention & compliance**: Configurable journal rotation, remote mirroring, and Cloud metadata deliver long-term custody for regulated environments.
- **Scope limits**: Netdata provides light SIEM coverage (collection, storage, correlation) but does not ship threat intelligence feeds or automated threat hunting.

## 8. Visualization & Dashboards

Infrastructure is grouped into (war) rooms. Each node can appear in multiple rooms. The dashboard provides aggregated room-level views automatically, and even these can be sliced on-demand using the global nodes filter.

The dashboard has a **global nodes filter**, a **global datetime picker** and a **global highlighted timeframe**.

- **global nodes filter**: slices immediately the entire dashboard to the selected nodes - affecting all aspects of the dashboard, including metrics, logs, functions, alerts, custom dashboards.
- **global datetime picker**: slices immediately the entire dashboard to the selected timeframe - affecting all data presented: metrics, logs, events, alerts transitions (except functions and raised alerts which are always live).
- **global highlighted area**: reveals/highlights data on the selected timeframe (e.g. charts highlight this area, logs seek to the logs of this timeframe, alert transitions focus on this timeframe, etc)

The above are synchronized across rooms and spaces, allowing users to switch between them during troubleshooting. The result is a fully synchronized UI, across all aspects of all datasets, with highly dynamic slicing on different aspects of the infrastructure.

### 8.1 NIDL Framework (Query-Less Analysis)

Netdata introduces a revolutionary visualization model: **Nodes, Instances, Dimensions, Labels (NIDL)**

**Components:**
1. **Nodes**: Individual machines/hosts
2. **Instances**: Specific entities (disks, containers, interfaces)
3. **Dimensions**: Individual metrics (user, system, iowait)
4. **Labels**: Key-value metadata (namespace, environment, device_type)

**Interactive Controls:**
- Dropdown menus above each chart
- Real-time statistics (min, avg, max, anomaly rate)
- Flexible grouping and aggregation
- Instant filtering and slicing
- No query language required

**Benefits:**
- Zero learning curve for new users
- Automatic visualization of all metrics
- Instant root cause analysis
- Flexible custom dashboards with drag-and-drop
- Same interface for junior and senior engineers

### 8.2 Dashboard Features

**Automated Dashboards:**
- Single-node dashboards (local Agent access)
- Multi-node dashboards (via Parent and Cloud)
- Infrastructure-level dashboards (unified view via Cloud)
- Kubernetes-specific dashboards
- Custom dashboards (drag-and-drop creation)

**Chart Capabilities:**
- Real-time updates (1-second refresh)
- Interactive pan/zoom
- Multiple chart types (line, area, stacked, bar, heatmap)
- Anomaly ribbon visualization
- Info ribbon (gaps, resets, partial data)
- Click-to-filter on any dimension
- Slice with dynamic filtering on any label, dimension, instance, node
- Group by any label, dimension, instance, node (and all combinations of them)
- Expanded chart view provides:
  - aggregations for viewable dimensions
  - drill down the entire dataset
  - comparison vs previous periods (yesterday, last week, last month, or custom)
  - correlations to other charts
- Export to PNG, CSV, JSON

**Advanced Features:**
- Metric correlations (find related anomalies, spikes, dives)
- Time-synchronized views across logs and metrics
- Chart annotations for context sharing
- Text cards for documentation
- Shared dashboards for team collaboration

### 8.3 Netdata Functions (Runtime Diagnostics)

Replace SSH access with browser-based diagnostics:

| Function | Replaces | Description |
|----------|----------|-------------|
| **Processes** | `top`, `htop` | Real-time process monitoring |
| **Network Connections** | `netstat`, `ss` | Active network connections |
| **Systemd Journal** | `journalctl` | Log exploration |
| **Systemd Services** | `systemd-cgtop` | Service resource usage |
| **Block Devices** | `iostat` | Disk I/O activity |
| **Network Interfaces** | `bmon`, `bwm-ng` | Interface statistics |
| **Mount Points** | `df` | Disk usage |
| **Containers/VMs** | `docker stats` | Container resource usage |
| **IPMI Sensors** | `ipmi-sensors` | Hardware sensor readings |

**Benefits:**
- No SSH access required
- Historical data available (not just current state)
- Integrated with metrics and ML
- Secure execution via ACLK
- Same per-second precision as console tools

## 9. Deployment Options

### 9.1 Standalone Deployment

**Use Case**: Single node monitoring, development, testing

**Configuration**: Out-of-the-box defaults

**Features:**
- Full observability on single node
- Local dashboard access (http://localhost:19999)
- Optional Cloud connection for remote access
- All ML, alerts, and storage local

**Installation:**
```bash
# One-line install (Linux/macOS)
wget -O /tmp/netdata-kickstart.sh https://get.netdata.cloud/kickstart.sh
sh /tmp/netdata-kickstart.sh

# Docker
docker run -d --name=netdata -p 19999:19999 netdata/netdata

# Windows
# Download installer from netdata.cloud
```

### 9.2 Parent-Child Deployment

**Use Case**: Centralized monitoring with distributed collection

**Architecture:**
- **Children**: Lightweight collectors on production systems
- **Parents**: Centralized storage, ML, alerts, dashboards

**Benefits:**
- Reduce production system load (Children can run in RAM-only mode)
- Long-term retention on Parents
- High availability through Parent clustering
- Unified dashboards across infrastructure
- Ephemeral node support (Kubernetes, auto-scaling)

**Child Configuration (Minimal):**
```ini
[db]
    mode = ram
    retention = 1200  # 20 minutes
[ml]
    enabled = no
[health]
    enabled = no
```

**Parent Configuration (Full-Featured):**
```ini
[db]
    mode = dbengine
    storage tiers = 3
    
    dbengine tier 0 retention size = 12GiB
    dbengine tier 0 retention time = 14d

    dbengine tier 1 retention size = 8GiB
    dbengine tier 1 retention time = 6mo

    dbengine tier 2 retention size = 4GiB
    dbengine tier 2 retention time = 2y
```

**Streaming:**
- Real-time metric forwarding
- Automatic failover to multiple Parents
- Data replication after reconnection
- Compression (ZSTD) for bandwidth efficiency
- TLS/SSL encryption support

### 9.3 Kubernetes Deployment

**Helm Chart Components:**
- **DaemonSet**: Child pod on each node
- **Deployment**: Parent pod (centralized storage)
- **Deployment**: K8s state monitoring pod
- **CronJob**: Optional restarter for nightly updates

**Features:**
- Automatic node discovery
- Container monitoring (cgroups)
- Kubernetes resource monitoring
- Persistent volume support
- RBAC integration

**Installation:**
```bash
helm repo add netdata https://netdata.github.io/helmchart/
helm install netdata netdata/netdata
```

**Required Mounts:**
- `/proc`, `/sys`: System metrics
- `/var/log`: Journal logs
- `/etc/os-release`: Host info
- `/var/lib/netdata`: Persistence

### 9.4 High Availability

**Active-Active Parents:**
- Multiple Parents receive same data
- Automatic failover
- Data replication between Parents
- No single point of failure
- ML model sharing (first Parent trains, others receive)

**Configuration:**
```ini
# Child streams to multiple Parents
[stream]
    destination = parent-a:19999 parent-b:19999
```

**Intelligent Clustering:**
- Automatic work distribution
- No duplicate ML training
- Streaming replication
- Federated queries via Cloud

### 9.5 Enterprise On-Premise

**Use Case**: Air-gapped facilities, critical infrastructure, complete data isolation

**Features:**
- Full Netdata Cloud hosted on-premises
- All components within your datacenter
- Kubernetes cluster (1.23+)
- Custom integrations and support

**Requirements:**
- Kubernetes cluster
- 20+ CPU cores (2,000 nodes)
- 45+ GB RAM (2,000 nodes)
- 500+ GB storage (2,000 nodes)
- Scales to 10,000+ nodes

### 9.6 Packaging & Release Channels

- **Kickstart installer**: `https://get.netdata.cloud/kickstart.sh` supports interactive or `--non-interactive`, `--stable-channel`, and `--nightly-channel` flags.
- **Native packages**: Official repositories for Debian/Ubuntu (APT), RHEL/Alma/Rocky/Amazon Linux (YUM/DNF), openSUSE, Arch, and FreeBSD ports; packages signed and updated daily.
- **Container images**: `netdata/netdata` (Agent/Parent capable), `netdata/netdata-debug` (instrumented), and hardened minimal images published in Docker Hub and GHCR; support rootless operation and persistent volumes.
- **Helm chart**: `netdata/netdata` Helm chart with configurable RBAC, persistence, Pod Security Policies, and tolerations.
- **Windows installer**: Signed MSI with automatic updates via Windows Task Scheduler; installs service + optional desktop shortcut.
- **Nightly vs stable**: Nightly channel ships daily builds with feature flags; stable channel updated regularly (typically monthly). `netdata-updater.sh` automates upgrades and rollback snapshots.

### 9.7 Configuration, Lifecycle & Operations

- **Configuration layers**: Stock defaults under `/usr/lib/netdata/conf.d`, site overrides in `/etc/netdata`, dynamic Cloud policies stored in `/var/lib/netdata/cloud.d`; precedence is dynamic → local → stock.
- **File-based configs**: `netdata.conf`, `stream.conf`, `cloud.d/`, `health.d/`, `go.d/`, `scripts.d/`, `python.d/`, `apps_groups.conf`, `alerts.d/`, and `statsd.d/`. All are INI/YAML and version-control friendly.
- **Dynamic configuration (DynCfg)**: Cloud UI edits stay synchronized via ACLK; agents persist snapshots locally and revert on disconnect if desired.
- **Backups & restore**: Retention lives in `/var/lib/netdata/dbengine`; configs in `/etc/netdata`; Cloud metadata exported via API. Cold backups possible using `netdatacli shutdown-agent` followed by filesystem snapshot.
- **Upgrade cadence**: Major releases monthly, nightly builds daily. Agents support zero-downtime restarts; Parents in HA clusters upgrade one at a time.
- **Troubleshooting toolkit**: `netdata-claim.sh --check`, `netdatacli ping`, `./usr/libexec/netdata/plugins.d/tc-qos-helper`, log files (`/var/log/netdata`), `debugfs.plugin` for deep kernel metrics, and Netdata Functions for live diagnostics.
- **Ephemeral estates**: Automatic cleanup policies for stale nodes, configuration templates for autoscaling groups, and labeling via Cloud or `/etc/netdata/labels.d/`.

## 10. Scalability & Performance

### 10.1 Scale Characteristics

**Supported Range**: 1 to 100,000+ nodes

**Per-Node Overhead:**
- **Most deployments**: <5% CPU, 150-200 MB RAM (depends on metrics discovered, ~5000 metrics baseline)
- **Child (offloaded)**: ~2% CPU, 100-150 MB RAM, zero disk I/O
- **Minimal mode**: <1% CPU, <100 MB RAM

**Parent Capacity:**
- ~500 nodes per Parent cluster (recommended)
- ~2 million metrics/second per cluster
- 20 cores, 80 GB RAM per cluster

**Key Metrics:**
- **Billions of metrics/second** processed globally
- **100% sampling rate** (no data loss)
- **Unlimited metrics** with linear resource scaling per metric (no artificial limits, isolated blast radius)
- **Proven at scale**: 100,000+ node deployments

### 10.2 Architectural Advantages

**Linear Scaling:**
- Adding nodes doesn't degrade existing performance
- No centralized bottleneck
- Each Agent operates independently
- Parents scale horizontally

**No Sampling:**
- Every metric, every second
- No blind spots
- No statistical approximation
- Complete data fidelity

**Cost Efficiency:**
- Predictable resource usage
- No exponential cost curves
- Lower infrastructure costs than centralized solutions
- Energy-efficient (independently validated)

**Cardinality Handling:**
- Linear resource scaling per metric (doubling metrics roughly doubles resources, not exponentially)
- Isolated blast radius: each agent handles its own cardinality independently—explosions on one server can't cascade to take down entire monitoring
- Automated cardinality protection: ephemeral metrics are automatically cleaned from higher storage tiers (per-minute, per-hour) while real-time data (per-second) is always preserved
- No manual cardinality management required (unlike Prometheus, Datadog)

## 11. Security & Privacy

### 11.1 Security Design

**Agent Security:**
- Local data storage (no external transmission)
- Configurable authentication
- TLS/SSL support for streaming
- Reverse proxy integration
- OpenSSF best practices

**Cloud Security:**
- Outbound-only connections (ACLK via MQTT over WSS)
- Certificate-based authentication
- No metric/log data storage in Cloud
- Encrypted communication (TLS)
- SOC 2 Type 2 certified

**Data Sovereignty:**
- All monitoring data stays local
- Zero metric storage in Cloud
- Zero log storage in Cloud
- Only essential metadata in Cloud (node info, chart definitions, alert configs)

### 11.2 Compliance

**Certifications:**
- SOC 2 Type 2
- OpenSSF Best Practices badge
- CNCF member

**Regulatory Compliance:**
- GDPR (General Data Protection Regulation)
- CCPA (California Consumer Privacy Act)
- PCI DSS (Payment Card Industry)
- HIPAA (Health Insurance Portability)

**Privacy Features:**
- Data residency (local storage)
- Configurable data retention
- User consent management
- Audit logs for compliance monitoring

### 11.3 Access Control

**Role-Based Access Control (RBAC):**
- Administrators (full access)
- Troubleshooters (view and analyze)
- Managers (view only)
- Observers (limited view)
- Billing (billing management)

**Enterprise Authentication:**
- Single Sign-On (SSO) via OIDC
- Okta integration
- LDAP/AD group mapping via SCIM 2.0
- Multi-factor authentication
- IP whitelisting

## 12. Pricing & Licensing

### 12.1 Pricing Tiers

| Plan | Price | Target Audience | Key Features |
|------|-------|-----------------|--------------|
| **Community** | Free | personal, non-commercial use only | 5 nodes, 1 custom dashboard, 4hr alert retention |
| **Homelab** | $90/year or $10/month | personal, non-commercial use only | Unlimited nodes*, unlimited dashboards, 60-day alerts |
| **Business** | $6/node/month | freelancers, professionals, businesses | Unlimited everything, AI (10 sessions/mo), SSO, SCIM, Windows |
| **Enterprise On-Premise** | Custom (starts at 200 nodes) | Air-gapped, critical infra | Everything + on-premise hosting, priority support |
| **Open Source Agent** | Free (backend: GPL v3+, dashboard: NCUL1*) | Self-hosted | Complete agent, unlimited metrics, community support |

*Subject to Fair Usage Policy

*NCUL1 = Netdata Cloud UI License, free to use with Netdata Agents, Parents, not open-source - this is the same dashboard Netdata Cloud uses, with the ability to connect to Netdata Agents and Parents directly, optionally using Netdata Cloud as SSO provider, providing similar experience to community users as Netdata Cloud.

| Target | Open Source | Community | Homelab | Business | Enterprise |
|--------|-------------|-----------|---------|----------|------------|
| personal, non-commercial use only | ✅ | ✅ | ✅ | - | - |
| freelancers | ✅ | - | - | ✅ | - |
| professionals | ✅ | - | - | ✅ | - |
| businesses | ✅ | - | - | ✅ | - |
| Air-gapped facilities | ✅ | - | - | - | ✅ |
| Critical infrastructure | ✅ | - | - | - | ✅ |

Based on Netdata's Terms of Service, Community and Homelab plans are exclusively for personal non-commercial use. Freelancers, Professionals and Businesses can either use Open Source or the Business Plan.

### 12.2 Business Plan Details

**Pricing:** $4.5/node/month (billed yearly), or $6/node/month (billed monthly)

Volume discounts on yearly commitment:

| Commitment | $/node/month | Total Yearly |
|------------|------------------|---------------|
| 50 | $4.5 | $2,700 |
| 100 | $4.39 | $5,268 |
| 200 | $4.22 | $10,128 |
| 300 | $4.01 | $14,436 |
| 400 | $3.91 | $18,768 |
| 500 | $3.85 | $23,100 |
| 501+ | contact us | contact us |

**Billing:**
- Based on **active nodes only** (offline/stale excluded)
- Charged on **P90 usage** (90th percentile, excludes daily spikes and the top 3 days per month)
- No charges for containers (one agent monitors all containers on host)
- No charges for metrics volume
- No charges for logs volume
- No charges for users

**What's Included:**
- Unlimited metrics, logs, and data retention
- Netdata AI (10 free sessions/month, additional via credits)
- RBAC with all user roles
- SSO and SCIM integration
- Windows monitoring
- Centralized configuration management
- Enterprise notification integrations
- Audit logs
- 99.9% SLA
- Premium support

**14-Day Free Trial**: Unlimited nodes, no restrictions

**P90 Usage Explained**

Customers are billed for sustained node usage, not temporary spikes. Netdata automatically excludes the highest usage periods each day (P90) and the highest 3 days each month (P90).

We calculate billable nodes using a 90th percentile (P90) method at two levels:

1. Daily: We determine the node count at the 90th percentile of time-weighted usage distribution. If you briefly spike to 100 nodes for 1 hour but run 10 nodes for 23 hours, your daily P90 is 10 nodes—the spike doesn't inflate your bill.

2. Monthly: We take the P90 of all your daily values. In a 30-day month, this excludes your top 3 daily values, protecting you from occasional high-usage days.

Or simpler:

We use double 90th percentile billing: brief spikes during the day don't affect your bill, and your top 3 days each month are excluded too. You're billed for sustained usage, not peaks.

Example: If you run 10 nodes for 23 hours and spike to 100 nodes for 1 hour, the spike is excluded, we account 10 nodes for that day. And if you have 3 days of load testing at 50 nodes in a 30-day month, those 3 days won't inflate your monthly bill.

### 12.3 Universal Features (All Plans)

Regardless of tier, all users benefit from:
- ✅ Unlimited metrics (infrastructure, APM, custom, synthetic)
- ✅ Unlimited users (no per-user charges)
- ✅ 1-second granularity
- ✅ No storage charges (data on-premises)
- ✅ No container charges
- ✅ Distributed architecture (no cardinality limits—each agent handles cardinality independently, issues can't cascade)
- ✅ Real-time querying
- ✅ System journal logs

## 13. Integration Ecosystem

### 13.1 Data Export

**Time-Series Databases:**
- Prometheus (scrape endpoint + Remote Write)
- InfluxDB (Line Protocol)
- Graphite (Plaintext Protocol)
- OpenTSDB (Telnet/HTTP API)
- TimescaleDB (PostgreSQL extension)

**Cloud Services:**
- AWS Kinesis Data Streams
- Google Cloud Pub/Sub
- MongoDB

**Custom:**
- JSON (generic HTTP endpoint)
- Custom scripts via exporting engine

### 13.2 APIs & Automation Surfaces

- **REST API v3**: Unified endpoint set under `/api/v3/` for data (`/data`), contexts (`/contexts`), nodes (`/nodes`), alarms (`/alarms`), weights/correlations (`/weights`), logs (`/logs`), and configuration. OpenAPI schema lives at `src/web/api/netdata-swagger.yaml`.
- **API lifecycle**: v1/v2 endpoints remain for backward compatibility; new UI and tooling target v3 exclusively. Responses support JSON, CSV, and rendered charts; rate-limited by agent settings.
- **Streaming API**: Agents stream metrics to Parents over TLS using `stream.conf`; automatic buffering, delta compression (ZSTD), and replay on reconnect ensure zero data loss.
- **ACLK (Agent-Cloud Link)**: MQTT over WSS channel used for Cloud control plane, alert transition fan-out, configuration changes, and Function executions. Outbound-only; certificates issued per node/space.
- **CLI Tooling**: `netdatacli` (reload health, labels, claim state, shutdown), `netdata-claim.sh` (Cloud enrollment), `netdata-updater.sh` (nightly/stable updates), `netdata-installer.sh` (kickstart). All commands support non-interactive automation.
- **Automation Hooks**: Health alarms can trigger scripts/webhooks; Netdata Functions expose runtime diagnostics via REST/Cloud/MCP; exporting engine can run external programs or POST to arbitrary endpoints.
- **Model Context Protocol (MCP)**: Agents/Parents expose MCP servers for IDEs and LLM tooling, enabling query, alerts, and function execution without bespoke APIs.

### 13.3 Visualization

**Grafana Integration:**
- Native Grafana datasource plugin
- Query Netdata from Grafana dashboards
- Preserves Netdata's per-second granularity

### 13.4 Infrastructure as Code

**Terraform:**
- Terraform provider for Netdata Cloud
- Manage spaces, rooms, users, alerts as code

**Ansible:**
- Ansible playbooks for deployment
- Automated configuration management

**Configuration Management:**
- Simple text files (YAML, INI)
- Version control friendly
- Centralized management via Cloud UI

### 13.5 Mobile Apps

**iOS and Android Apps:**
- Push notifications (paid plans)
- Dashboard access
- Alert management
- Node monitoring

## 14. Support & Resources

### 14.1 Documentation

**Official Documentation**: https://learn.netdata.cloud
- Comprehensive guides and tutorials
- API reference
- Troubleshooting resources
- Best practices
- Video walkthroughs

### 14.2 Community Support

**Community Channels:**
- Forum: https://community.netdata.cloud
- GitHub Discussions: https://github.com/netdata/netdata/discussions
- Discord: https://discord.com/invite/2mEmfW735j
- GitHub Issues: Bug reports and feature requests

**Community Stats:**
- 76,300+ GitHub stars
- 615+ contributors
- 1.5 million downloads per day
- Active community engagement

### 14.3 Commercial Support

**Support Tiers:**
- Community: Public forums and tickets
- Business: Email/ticket support during business hours with SLA
- Enterprise: 24/7 availability, dedicated support team, phone support

**Professional Services:**
- Implementation assistance
- Architecture design
- Migration support
- Custom integrations
- Training programs

## 15. Key Differentiators

### 15.1 vs Traditional Monitoring

| Aspect | Netdata | Traditional Solutions |
|--------|---------|----------------------|
| **Granularity** | Per-second | Per-minute or worse |
| **Latency** | <2 seconds | Minutes to hours |
| **Configuration** | Zero-config | Extensive setup |
| **ML** | Built-in, edge-based | Optional, cloud-based |
| **Cost Model** | Predictable, linear | Exponential with scale |
| **Data Fidelity** | 100% (no sampling) | Sampled/downsampled |
| **Architecture** | Distributed | Centralized |
| **Scalability** | Linear | Exponential complexity |

### 15.2 vs Prometheus

**Netdata Advantages:**
- 36% less CPU, 88% less RAM
- 97% less disk I/O
- 15× longer retention (same disk)
- 16× faster queries
- Zero data loss (vs 93.7% completeness)
- Zero configuration
- Built-in ML and anomaly detection
- Integrated logs management
- AI-powered troubleshooting

### 15.3 vs Datadog/New Relic/Dynatrace

**Netdata Advantages:**
- True real-time (per-second vs per-minute)
- 90% cost reduction (predictable per-node pricing)
- Data sovereignty (all data on-premises)
- Zero vendor lock-in (open source agent)
- No sampling or data loss
- Edge-based ML (no cloud dependency)
- Faster deployment (minutes vs weeks)
- No specialized skills required

### 15.4 Unique Capabilities

1. **Edge-Native ML**: 18 models per metric, consensus-based detection
2. **NIDL Framework**: Query-less interactive analysis
3. **Functions**: Runtime diagnostics without SSH
4. **Distributed Architecture**: No central bottleneck
5. **Zero Data Loss**: 100% sample completeness
6. **Real-Time Everything**: Per-second collection, visualization, ML, alerts
7. **Logs Without Pipelines**: Direct systemd-journal access
8. **Console Replacement**: Browser-based debugging with history

## 16. Use Cases & Success Stories

### 16.1 Industry Adoption

**Sectors Using Netdata:**
- Technology & IT
- Financial Services (FinTech, Banking)
- Education & Research
- Government & Public Sector
- Transportation & Automotive
- Healthcare
- Hosting & Cloud Providers
- Gaming & Interactive Media
- Retail & E-commerce
- Logistics & Supply Chain

**Scale:**
- 1.5 million downloads per day
- 688M+ docker hub pulls
- 76,300+ GitHub stars
- 4.5+ billion metrics/second processed

### 16.2 Customer Success Metrics

**Performance Improvements:**
- 40× better storage efficiency
- 22× faster response times
- 15% reduced resource consumption
- ~25% reduction in infrastructure downtime
- 80% faster MTTR
- 90% lower TCO

## 17. Getting Started

### 17.1 Quick Start (5 Minutes)

**Step 1: Install Agent**
```bash
# Linux/macOS
wget -O /tmp/netdata-kickstart.sh https://get.netdata.cloud/kickstart.sh
sh /tmp/netdata-kickstart.sh

# Docker
docker run -d --name=netdata -p 19999:19999 netdata/netdata

# Kubernetes
helm repo add netdata https://netdata.github.io/helmchart/
helm install netdata netdata/netdata

# Windows
# Download installer from netdata.cloud
```

**Step 2: Access Dashboard**
- Local: http://localhost:19999
- Cloud: https://app.netdata.cloud (after claiming)

**Step 3: Claim to Cloud (Optional)**
- Click "Sign in" on dashboard
- Or use CLI: `sudo netdata-claim.sh -token=YOUR_TOKEN -rooms=YOUR_ROOM -url=https://app.netdata.cloud`

**Step 4: Configure (Optional)**
- Edit `/etc/netdata/netdata.conf` for main settings
- Configure collectors in `/etc/netdata/go.d/`, `/etc/netdata/python.d/`, `/etc/netdata/scripts.d/`
- Set up streaming in `/etc/netdata/stream.conf`

**Step 5: Explore**
- Navigate dashboards
- Review pre-configured alerts
- Explore logs (systemd-journal or Windows Event Logs)
- Try AI troubleshooting

### 17.2 Enterprise Deployment (Days Not Months)

**5 Steps:**

1. **Design Topology** (1 day)
   - Decide Parent placement (one cluster per ~500 nodes)
   - Position by geography/provider
   - Teams organize later via Rooms

2. **Deploy Everywhere** (1 day)
   - Use Ansible/Terraform/Helm
   - Install Agents + Parents
   - Same binary, different roles

3. **Add Collector Credentials** (2 hours)
   - UI shows discovered services needing auth
   - Configure databases, cloud accounts, SNMP
   - Enable custom app collectors

4. **Review Alerts** (2 hours)
   - 400+ pre-configured alerts already running
   - Tune thresholds, disable irrelevant ones
   - ML is already training

5. **Invite Teams** (2 hours)
   - Create Rooms for team isolation
   - Add users, set permissions
   - Each team sees their infrastructure slice

**Done**: Full enterprise monitoring operational in days, not months.

## 18. Roadmap & Future

### 18.1 Upcoming Features

**OpenTelemetry Support**:
- Native OTel collector integration (ingestion)
- OTLP traces ingestion (planned Q2 2026)
- Full MELT (Metrics, Events, Logs, Traces) visibility

**Enhanced Kubernetes Monitoring**:
- Deeper service mesh integration
- Advanced pod lifecycle tracking
- Enhanced resource optimization

**Expanded Cloud Integrations**:
- Additional cloud provider metrics
- Enhanced multi-cloud support
- Cloud cost optimization insights

**Advanced AI Capabilities**:
- Multi-model AI support
- Enhanced natural language queries
- Predictive analytics
- Automated remediation suggestions

### 18.2 Active Development Focus

- Performance optimizations
- Enhanced Windows monitoring
- Expanded integration ecosystem
- Improved user experience
- Advanced security features

## 19. Multi-Tenacy

Netdata Cloud supports Spaces (organizations) and Rooms.

- Spaces provide physical isolation (e.g. different customers of an MSP)
- Rooms provide logical isolation (e.g. different teams, services, roles, incidents)

Physical isolation: each space has its own infrastructure
- Each node belongs to single space
- Each space has its own rooms
- Dashboard settings are not shared between spaces (each space has its own set of settings)
- Each space has its own set of users and permissions
- Billing is applied per space

Logical isolation:
- Nodes can be added to multiple rooms of the same space
- Rooms can be private or public (within a space)

Users: each space has its own users, but a single session provides access to all spaces
- Each user may be a member of multiple spaces
- Each user may have a different role per space
- Each user may be limited to some specific rooms per space

Freelancers and MSP may have multiple spaces, and on each space they may have invited their customers and subcontractors. True multi-tenacy is supported via space-level (physical) isolation.

## 20. Summary

Netdata represents a fundamental rethink of observability architecture. By processing data at the edge, automating configuration, maintaining real-time resolution, applying ML universally, and making data accessible to everyone, it solves core monitoring challenges that have persisted for decades.

**Core Strengths:**
- ✅ Unparalleled real-time visibility (per-second metrics)
- ✅ Zero-configuration deployment
- ✅ Extensive integration ecosystem (800+)
- ✅ Lightweight and efficient resource usage
- ✅ Strong open-source community (76,300+ GitHub stars)
- ✅ Flexible deployment (cloud, on-premise, hybrid)
- ✅ Competitive pricing with generous free tier
- ✅ Advanced ML-based anomaly detection
- ✅ Native cross-platform support (Linux, Windows, macOS, FreeBSD)
- ✅ AI-powered troubleshooting
- ✅ Logs management without pipelines
- ✅ Complete data sovereignty

**The Netdata Difference:**
- **Distributed by design**: No central bottleneck, linear scaling
- **Real-time by default**: Per-second everything, not averaged or sampled
- **Intelligent by nature**: ML on every metric, AI for troubleshooting
- **Simple by philosophy**: Zero configuration, automatic dashboards
- **Sovereign by architecture**: Your data stays on your infrastructure

Whether you're monitoring a single server or a global infrastructure, Netdata's design philosophy creates a monitoring system that works with you rather than demanding constant attention.

---

## Contact & Resources

- **Website**: https://www.netdata.cloud
- **Documentation**: https://learn.netdata.cloud
- **GitHub**: https://github.com/netdata/netdata
- **Community**: https://community.netdata.cloud
- **Discord**: https://discord.com/invite/2mEmfW735j
- **Pricing**: https://netdata.cloud/pricing
- **Enterprise**: https://netdata.cloud/request-enterprise/
- **Support**: https://netdata.cloud/support/

**Start Your Free Trial**: https://app.netdata.cloud

---

*This comprehensive product description is based on official Netdata documentation, source code analysis, and publicly available information as of October 2025. All features and capabilities described are currently available or in active development.*
