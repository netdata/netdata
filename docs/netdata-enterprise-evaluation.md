# Netdata at a Glance: Enterprise Evaluation Guide

## Introduction

Netdata is an open-source, real-time infrastructure monitoring platform designed for modern IT environments. It combines automations and innovations in several areas to provide comprehensive, real-time, full-stack observability, advanced troubleshooting capabilities, and simplified operations, even for lean teams.

### Core Components

Netdata consists of three components:

**1. Netdata Agents**: A highly optimized monitoring agent installed on all IT systems (Linux, Windows, FreeBSD, macOS). Netdata Agents are complete observability solutions packaged in a single application (data collection, storage, query engines, machine learning, alerts, dashboards).

**2. Netdata Parents**: Netdata Parents use the same software as Netdata Agents, configured to accept and maintain data on behalf of other Netdata Agents, acting as observability centralization points.

**3. Netdata Cloud**: A highly scalable control plane, available as both on-premises and SaaS. Its role is to abstract access to Netdata Agents and Parents, centralize alert management, provide user and role management, and deliver infrastructure-level dashboards.

When deploying Netdata, organizations have the flexibility to:

1. Use standalone Agents (fully distributed) or centralize observability data to one or more Netdata Parents
2. Use a single large cluster of Netdata Parents (fully centralized) or multiple independent Netdata Parents (mixed distributed and centralized)
3. Deploy Netdata Cloud on-premises or use it as a SaaS

Typically, organizations use the mixed approach, where they install a number of independent Netdata Parent clusters, centralizing observability data per data center, service, department, or geography. These independent Netdata Parents become unified infrastructure observability through Netdata Cloud (on-premises or SaaS), like this:

```
┌─── Datacenter A ───┐  ┌─── Datacenter B ───┐  ┌─── Cloud Region ───┐
│     500 Agents     │  │     700 Agents     │  │     400 Agents     │
│         ↓          │  │         ↓          │  │         ↓          │
│  Parent Cluster A  │  │  Parent Cluster B  │  │  Parent Cluster C  │
└────────────────────┘  └────────────────────┘  └────────────────────┘
           ↓                      ↓                        ↓
┌────────────────────────────────────────────────────────────────────┐
│                 Netdata Cloud (On-Prem or SaaS)                    │
│                  Unified View & Access Control                     │
└────────────────────────────────────────────────────────────────────┘
```

For more information see [Deployment Strategies](/docs/deployment-guides/deployment-with-centralization-points.md).

## Evaluation Criteria

How Netdata approaches the criteria an enterprise evaluation usually covers, with pointers to the details:

- **Scale.** Netdata scales out, not up. Independent Parents run locally at each site, data center or region, and capacity grows by adding Parents. For network monitoring, SNMP collection runs on distributed hubs close to the devices, including branch offices connected over DSL lines. See [Scalability](#scalability) and [Network Performance Monitoring](/docs/npm/README.md).
- **High availability.** Parents cluster active-active, and each Parent in a cluster keeps its own full copy of the data. During an internet outage, Agents and Parents keep collecting, evaluating alerts and serving their own dashboards locally. See [Clustering and High Availability of Netdata Parents](/docs/observability-centralization-points/metrics-centralization-points/clustering-and-high-availability-of-netdata-parents.md).
- **Deployment and configuration.** Configuration works both ways: as code, provisioned with Ansible, Puppet, Chef, Salt, Terraform or Helm, or centrally from the dashboard. See [Rollout / Lifecycle Management](#rollout--lifecycle-management) and [Dynamic Configuration](/docs/netdata-agent/configuration/dynamic-configuration.md).
- **Security and access.** Agents connect to Netdata Cloud outbound only and open no inbound ports for it. Users sign in with SSO, groups map to roles through SCIM, and access is scoped by spaces and rooms. See [Authentication, Authorization, and Accounting](#authentication-authorization-and-accounting-aaa) and [Security and Privacy Design](/docs/security-and-privacy-design/README.md).
- **Data sovereignty.** Metrics, logs and traces are stored on the Agents and Parents in your infrastructure. Netdata Cloud stores metadata only, and Netdata Cloud On-Prem keeps the control plane inside your network too. See [Compliance and Security](#compliance-and-security).
- **Integrations.** Open standards in and out: OpenTelemetry, OpenMetrics and StatsD in; Prometheus, InfluxDB, Graphite and others out. More than 800 collectors, and alert notifications to PagerDuty, Slack, Teams, ServiceNow and more. See [Integration](#integration).
- **Cost.** Agents run on spare capacity, Parents on ordinary VMs, pricing is per node, and observability data does not cross the WAN. See [Cost Efficiency](#cost-efficiency).
- **Support.** Community, business and enterprise support, with professional services for architecture, deployment and migration. See [Support and Maintenance](#support-and-maintenance).

## Infrastructure Requirements

Netdata has been architected to minimize resource consumption while maximizing observability capabilities. This efficiency is achieved through several key design decisions:

- **Storage Optimization**: Industry-leading compression achieving ~0.6 bytes per sample on disk for high-resolution data
- **Edge Computing**: Distributed architecture keeps data processing close to its source, reducing bandwidth and central processing requirements - Netdata distributes the code instead of centralizing the data
- **Intelligent ML**: Machine learning runs as low-priority background tasks, automatically yielding resources when needed for data collection. See [AI & ML Features](/docs/category-overview-pages/machine-learning-and-assisted-troubleshooting.md)
- **Built-in Scalability**: Native clustering and high-availability features enable organizations to scale horizontally without architectural changes. See [Observability Centralization Points](/docs/deployment-guides/deployment-with-centralization-points.md).
- **Stable and Predictable Resource Usage**: Agents are optimized to spread work over time and avoid sudden changes in resource consumption

This design philosophy enables Netdata to monitor infrastructure at unprecedented scale with minimal overhead, making it suitable for everything from IoT devices to high-performance computing clusters.

### Agent Requirements (Per Node)

**Netdata uses under 5% of one CPU core and about 200 MB of RAM on a typical server.** That is with default settings: per-second collection of about 3,000 metrics, machine learning on every metric, three storage tiers and all stock alerts. Usage grows with the number of metrics collected. [Sizing](/docs/netdata-agent/sizing-netdata-agents/README.md) covers busy hosts and Parents.

**On edge devices, Netdata runs in about 30 MiB.** A Raspberry Pi 3 B+ configured as a lightweight Child uses about 3% of one core at five-second collection, and under 1% at five-minute collection. See [edge devices](/docs/fleet-management/minimize-cpu-and-memory.md).

When connected to a Netdata Parent (the agent is in `child` mode), these requirements can be significantly reduced and, in the case of storage, even eliminated (no data on local disk).

| Type       | Metric/s | CPU                      | Memory      | Network  | Storage                          |
|------------|----------|--------------------------|-------------|----------|----------------------------------|
| standalone | about 3k | under 5% of a single core | about 200 MB | none     | about 4 GiB by default, configurable |
| child      | 3k - 10k | 2% - 10% of a single core | 100 - 300 MiB | \<1 Mbps | none                             |

For more information and ways to further reduce Agent resource utilization, see [Agent Resource Utilization](/docs/netdata-agent/sizing-netdata-agents/README.md).

### Parent Node Requirements (Centralization Points)

Parent nodes aggregate data from multiple child agents and require resources that scale with the volume of metrics collected and other deployment decisions (clustered parents, centralized machine learning, etc).

Size each Netdata Parent, or Parent cluster, for 100 to 500 monitored nodes (a planning unit for capacity, not a limit: about 4,000 metrics/second per node, leaving headroom; see [Parent Sizing Guidelines](/docs/scalability.md#parent-sizing-guidelines)):

| Monitored Nodes | Metric/s    | CPU Cores | Memory | Network  | Storage                   |
|-----------------|-------------|-----------|--------|----------|---------------------------|
| 100 nodes       | 0.5 million | 5 cores   | 20 GB  | 50 Mbps  | varies based on retention |
| 250 nodes       | 1 million   | 10 cores  | 40 GB  | 100 Mbps | varies based on retention |
| 500 nodes       | 2 million   | 20 cores  | 80 GB  | 200 Mbps | varies based on retention |

These figures include ingestion and query resources.

For more information see [Agent Resource Utilization](/docs/netdata-agent/sizing-netdata-agents/README.md).

For high availability configurations, see [Clustering and High Availability of Netdata Parents](/docs/observability-centralization-points/metrics-centralization-points/clustering-and-high-availability-of-netdata-parents.md).

### Netdata Cloud On-Prem

For organizations requiring complete control over their monitoring infrastructure, Netdata Cloud On-Prem provides the full control plane within the datacenter (air-gapped).

Netdata Cloud On-Prem runs on Kubernetes:

| Monitored Nodes | CPU Cores | Memory | Storage | Notes                  |
|-----------------|-----------|--------|---------|------------------------|
| 2,000 nodes     | 20 cores  | 45 GB  | 500 GB  | Minimal K8s cluster    |
| 10,000 nodes    | 100 cores | 200 GB | 2 TB    | Multi-node K8s cluster |

For more information see [Netdata Cloud On-Prem](https://github.com/netdata/netdata-cloud-onprem/blob/master/docs/learn.netdata.cloud/README.md) and [Installation Guide](https://github.com/netdata/netdata-cloud-onprem/blob/master/docs/learn.netdata.cloud/installation.md).

## Rollout / Lifecycle Management

Netdata users have multiple options to deploy Netdata on their systems. Netdata Agent configuration uses simple text files that can be provisioned with configuration management tools, versioned in git repositories, and maintained centrally for large infrastructures.

### Agent and Parent Deployment Options

1. **Configuration Management Tools**: Use Ansible, Puppet, Chef, Salt, Terraform
2. **Container Deployment**: Use official Docker images
3. **Native Package Management**: Use native GPG-signed packages for Debian, Ubuntu, Red Hat, CentOS, SUSE, etc., with the ability to mirror Netdata repositories internally for air-gapped environments
4. **One-Line Installation with Auto-Updates**: Single command installation that configures automatic updates, with a choice of release channels: stable (recommended) or nightly

**Note**: Netdata Parents use the same software as Agents with different configuration. All deployment methods apply to both.

For more information about Netdata Agents & Parents installation methods see [Netdata Agent Installation](/packaging/installer/README.md).

### Netdata Cloud On-Prem Installation & Updates

Installation and updates for Netdata Cloud On-Prem are performed manually using Helm.

For more information see [Netdata Cloud On-Prem Installation](https://github.com/netdata/netdata-cloud-onprem/blob/master/docs/learn.netdata.cloud/installation.md).

### Updates and Backwards Compatibility

Netdata maintains strong backward compatibility:

- Newer Parents accept streams from older Agents
- Configuration files remain compatible across versions
- Breaking changes are rare and well-documented
- Multi-version deployments are fully supported (although not recommended)

## Authentication, Authorization, and Accounting (AAA)

Netdata Agents and Parents are authorized to connect to Netdata Cloud via a process called [claiming](/src/claim/README.md). This process uses cryptographic certificates to authorize the agents to connect to Netdata Cloud and can be controlled via configuration management/provisioning tools, or manually.

To authorize users, Netdata Cloud supports email authentication, Google OAuth, GitHub OAuth, Okta SSO, OpenID Connect (OIDC). For more information see [Enterprise SSO Authentication](/docs/netdata-cloud/authentication-and-authorization/enterprise-sso-authentication.md).

Netdata supports automatic mapping of LDAP/AD groups into Netdata roles, using the System for Cross-domain Identity Management (SCIM) v2. For more information see [SCIM Integration](/integrations/cloud-authentication/metadata.yaml).

To control access to observability data, Netdata Cloud uses spaces and rooms to segment the infrastructure (by geography, type, or any grouping the organization decides). Users gain access to spaces and rooms and, depending on their role, they may have different access to different types of observability data. For more information see [Role-Based Access model (RBAC)](/docs/netdata-cloud/authentication-and-authorization/role-based-access-model.md).

## Scalability

Netdata scales out: each Parent cluster is an independent shard, so you add capacity by adding Parents, and no central component caps the size of the monitored infrastructure.

Ingestion and storage have no central component that caps them. When using standalone agents, each agent is independent of the others. When using parent-child agents, each Parent is sized on its own, and the system grows by adding Parents.

Queries run in parallel on the Agents and Parents that hold the data: each one aggregates its own nodes and Netdata Cloud merges the results, so adding Parents adds query capacity too. See [Fleet-wide queries](/docs/netdata-design.md#fleet-wide-queries).

## Impact on the Monitored Infrastructure

Netdata takes the impact on monitored infrastructure extremely seriously. Netdata is committed to being a friendly and polite citizen alongside other applications and services running on the monitored systems. This philosophy drives every architectural decision.

**Netdata's Commitment:**

- Any negative impact on monitored applications is considered a severe bug that must be fixed
- Continuous optimization to minimize resource consumption while maximizing observability value
- The solution is battle-tested by a large community across diverse environments
- Years of engineering effort have been invested to ensure Netdata performs exceptionally in resource efficiency

### Independent Validation

According to the [University of Amsterdam study](https://www.ivanomalavolta.com/files/papers/ICSOC_2023.pdf), examining the energy efficiency and impact of monitoring tools on Docker-based systems, Netdata was found to be the most energy-efficient tool, with best-in-class resource consumption and no impact on the monitored workloads. The study reports:

- **Energy consumption** - Lowest among the compared tools
- **CPU load and RAM usage** - Among the lowest of the compared tools
- **Execution time** - Unaffected when running Netdata

## Data Retention and Storage

For metrics, Netdata maintains ingested samples as close to the edge as possible. Netdata Parents receive these data and each parent in a cluster maintains its own copy of them. All metrics are stored in write-once files which are rotated according to configuration (size and/or time based). Each Netdata Agent or Parent maintains by default 3 storage tiers of the same data (high resolution - per second, medium resolution - per minute, low resolution - per hour). All tiers are updated in parallel during ingestion. Users can configure size and time retention per tier. High availability of the data is achieved via streaming (parent-child relationships) and parents clustering.

For detailed information see [Disk Requirements and Retention](/docs/netdata-agent/sizing-netdata-agents/disk-requirements-and-retention.md).

For logs collected from systemd journals, Netdata reads standard journal files (also readable with `journalctl`), so standard systemd-journald practices such as archiving, backup, centralization, exporting, and Forward Secure Sealing apply. OpenTelemetry logs use the OpenTelemetry plugin's indexed storage and configured retention limits.

## Logs

Netdata fits three ways of working with logs:

- **Logs left where they are.** Teams that do not centralize system logs can still search them: Netdata queries systemd journals and Windows event logs on each host, in place, without moving them. See [systemd journal](/src/collectors/systemd-journal.plugin/README.md) and [Windows events](/src/collectors/windows-events.plugin/README.md).
- **Logs centralized with OpenTelemetry.** Teams that centralize logs ship them through the OpenTelemetry Collector, which reads log files, journals, Windows event channels and Kubernetes container logs, with no changes to the applications. See [Centralizing Logs with OpenTelemetry](/docs/logs/centralizing-logs-with-opentelemetry.md).
- **Indexed log storage.** Centralized logs are stored on the Agents and Parents with every field indexed as they arrive, on the same backend as traces, and there is no separate log database to run.

## Application Monitoring and Tracing

Netdata monitors applications out of the box:

1. **Processes, services and containers.** Every process, systemd service, Windows service and container, per second: CPU, memory, disk and network I/O, open files, threads and everything else the operating system exposes for it. See [Processes](/docs/functions/processes.md).
2. **Network connections and dependencies.** Netdata reads the live socket table on each host and maps which process talks to which, attributed to its container, systemd unit or Kubernetes pod. See [Application Dependency Mapping](/docs/npm/topology/dependency-mapping.md).
3. **Logs through OpenTelemetry.** Application logs collected as described in [Logs](#logs).
4. **Distributed tracing through OpenTelemetry.** Applications instrumented with OpenTelemetry send spans to Netdata, which indexes every span field, including names, kinds, statuses and attributes, and keeps a trace ID index to load any trace whole. See [Trace Storage and Retention](/docs/opentelemetry/trace-storage-and-retention.md).
5. **Service map with RED metrics.** From the spans, Netdata maps how services call each other and the databases, queues and external endpoints they depend on, with request rate, errors and latency (p50, p95, p99) for every call, on both the client and the server side. See [Service map](/docs/opentelemetry/trace-storage-and-retention.md#service-map).
6. **Digital experience monitoring.** Real user monitoring collects Web Vitals, page and view traffic, JavaScript errors, resource timing and session timelines from real browsers, and can send browser traces to connect user sessions with backend spans. Synthetic monitoring runs Playwright browser journeys and Lighthouse audits on a schedule, from your own Agents. See [Real User Monitoring](/src/go/plugin/dem/collector/rum/README.md) and [Browser Journeys](/src/go/plugin/dem/collector/journey/README.md).

The first three need no instrumentation and no involvement from the application: processes, services, containers and connections are read from the operating system, and logs from the files the applications already write. All of it is collected in real time.

## Integration

Netdata is an open platform.

It can ingest metrics in commonly used open standards, including OpenMetrics, StatsD, JSON, and OpenTelemetry (OTLP over gRPC or HTTP). Netdata also receives OpenTelemetry logs and traces, and has approximately 800+ data collection modules and plugins to directly collect data from applications.

Netdata can export metrics to Prometheus, InfluxDB, Graphite, OpenTSDB, TimescaleDB, and more.

For systemd journal sources, standard systemd-journald practices apply. OpenTelemetry log storage has its own rotation and retention configuration.

For alert notifications, Netdata supports PagerDuty, Slack, Teams, email (SMTP), Discord, Telegram, Jira, ServiceNow, and custom webhooks.

For AI and Large Language Models, Netdata supports Model Context Protocol (MCP) — available via Netdata Cloud for infrastructure-wide access and on every Agent/Parent for direct local access. Netdata supports AI DevOps/SRE Copilots like Claude Code and Gemini CLI, and provides an AI Chat application (requires access to a cloud LLM provider such as Google, OpenAI, Anthropic, or DeepSeek, or a self-hosted provider like Ollama).

## Compliance and Security

Netdata provides enterprise-grade security and compliance capabilities designed to meet the requirements of regulated industries and security-conscious organizations.

**Compliance Standards:**
Netdata maintains SOC 2 Type 2 certification. The platform aligns with GDPR, CCPA, HIPAA (with BAA available), and PCI DSS requirements, making it suitable for financial services, healthcare, and government deployments.

**Data Sovereignty Architecture:**
The platform's architecture ensures complete data sovereignty - all metrics, logs and traces remain on customer infrastructure. Only metadata (node names, chart titles, alert configurations) is stored in Netdata Cloud; metric, log and trace data passes through Cloud only to answer the queries users run, with all data transmission protected by TLS encryption. Air-gapped deployment options are available for maximum security isolation.

**Enterprise Authentication:**
Comprehensive SSO integration supports LDAP/AD group mapping through SCIM 2.0, enabling automatic role assignment based on organizational structure. Multi-factor authentication, IP whitelisting, and role-based access control provide granular security controls with comprehensive audit logging.

**Security Operations:**
Regular third-party security audits, penetration testing, and an active bug bounty program ensure continuous security improvement. The platform supports configurable data anonymization and network segmentation for sensitive environments.

For comprehensive security information see [Security and Privacy Design](/docs/security-and-privacy-design/README.md).

## Support and Maintenance

Netdata provides tiered support options designed to meet varying enterprise requirements, from community-driven assistance to dedicated enterprise support with custom SLAs.

**Support Tiers:**
Community support includes GitHub discussions and Discord community access. Business support provides email/ticket support during business hours with SLA guarantees. Enterprise support offers 24/7 availability with dedicated support teams, custom SLAs, and phone support.

**Professional Services:**
Comprehensive implementation services include architecture design, deployment assistance, migration support, and integration services. Training programs offer administrator and user training, custom workshops, and certification programs.

**Maintenance Services:**
Proactive monitoring services provide health checks, performance reviews, capacity planning, and optimization recommendations. Update management includes scheduled updates, emergency patches, regression testing, and rollback procedures.

## Customization

Netdata's flexible architecture supports extensive customization to meet specific organizational requirements while maintaining ease of use and operational simplicity.

**Dashboard and Visualization:**
The platform provides drag-and-drop dashboard creation with custom visualizations, shared dashboards, and template libraries. More than a dozen chart types are available with custom color schemes, responsive layouts, and export capabilities.

**Alert Management:**
Comprehensive alert customization includes custom thresholds, complex conditions, ML-based alerts, and schedule-based rules. Pre-built and custom alert templates support bulk configuration with version control integration.

**Extensibility:**
The plugin architecture supports custom collectors in multiple languages (Go recommended, Python, Bash, Node.js). Custom exporters offer format customization, filtering rules, transformation options, and batch processing. API extensions enable webhook development, custom endpoints, data processing, and event handling.

## Cost Efficiency

Netdata costs less to run than centralized observability stacks, for four reasons:

1. **Agents run on spare capacity.** A standalone Agent uses under 5% of one CPU core, about 200 MB of RAM and a few GB of disk, which fits in the headroom servers already keep. Agents streaming to a Parent can use even less, with no local disk.
2. **Parents scale out on ordinary VMs.** Each Parent is independent and sized for 100 to 500 nodes (from 5 cores and 20 GB of RAM for 100 nodes to 20 cores and 80 GB for 500), on standard disks. There is no central database cluster or high-performance storage tier to buy, run or back up.
3. **Pricing is predictable.** Plans are priced per node. Metrics and system logs have no volume charge, and there is no retention fee. Application logs and trace spans come with large allowances, pooled across all nodes, and only usage above the pool is billed. Spikes don't raise the bill: the node count excludes the busiest 2 hours of each day and the busiest 3 days of each month.
4. **Egress cost is practically zero.** Observability data stays where it is collected and is not copied across the WAN or the internet.

For current pricing, see [netdata.cloud/pricing](https://www.netdata.cloud/pricing/).

**Return on Investment:**
Value starts at installation: auto-discovery collects every system and application it finds, with working dashboards and alerts from the first minute. Over time, machine-learning anomaly ranking and AI investigations shorten troubleshooting, and one platform for metrics, logs, traces and alerts reduces tool sprawl.

## Training and Documentation

Netdata provides comprehensive training and documentation resources designed to support successful deployment and ongoing operations across enterprise environments.

**Documentation Resources:**
Extensive documentation is available at [learn.netdata.cloud](https://learn.netdata.cloud), including a comprehensive [Getting Started Guide](/docs/deployment-guides/README.md), step-by-step tutorials, video walkthroughs, and architecture deep-dives. Reference materials include API documentation, configuration guides, troubleshooting resources, and best practices.

**Training Programs:**
Self-service learning options include interactive tutorials, hands-on labs, video courses, and certification paths. Instructor-led training provides virtual workshops, on-site training, custom curriculum development, and train-the-trainer programs.

**Community and Support:**
Active community resources include forums, Discord server, GitHub discussions, and user contributions. Use case examples cover industry solutions, architecture patterns, success stories, and performance benchmarks. Onboarding support includes quick start guides, deployment assistance, initial configuration help, and health checks.

---

*For more information and detailed technical documentation, visit [learn.netdata.cloud](https://learn.netdata.cloud)*
