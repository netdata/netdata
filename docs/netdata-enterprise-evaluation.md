# Netdata at a Glance: Enterprise Evaluation Guide

## Introduction

Netdata is an open-source, real-time infrastructure monitoring platform providing comprehensive, full-stack observability, advanced troubleshooting, and simplified operations, even for lean teams.

### Core Components

Netdata consists of three components:

**1. Netdata Agents**: A highly optimized monitoring agent installed on all IT systems (Linux, Windows, FreeBSD, macOS). Netdata Agents are complete observability solutions packaged in a single application (data collection, storage, query engines, machine learning, alerts, dashboards).

**2. Netdata Parents**: Netdata Parents use the same software as Netdata Agents, configured to accept and maintain data on behalf of other Netdata Agents, acting as observability centralization points.

**3. Netdata Cloud**: A highly scalable control plane, available as both on-premises and SaaS. Its role is to abstract access to Netdata Agents and Parents, centralize alert management, provide user and role management, and deliver infrastructure-level dashboards.

When deploying Netdata, organizations have the flexibility to:

1. Use standalone Agents (fully distributed) or centralize observability data to one or more Netdata Parents
2. Use a single large cluster of Netdata Parents (fully centralized) or multiple independent Netdata Parents (mixed distributed and centralized)
3. Deploy Netdata Cloud on-premises or use it as a SaaS

Most organizations use a mixed approach: independent Netdata Parent clusters per data center, service, department, or geography, unified into one view through Netdata Cloud:

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

## Infrastructure Requirements

Netdata is architected to minimize resource consumption while maximizing observability capabilities:

- **Storage Optimization**: Industry-leading compression achieving ~0.6 bytes per sample on disk for high-resolution data
- **Edge Computing**: Distributed architecture keeps data processing close to its source, reducing bandwidth and central processing requirements - Netdata distributes the code instead of centralizing the data
- **Intelligent ML**: Machine learning runs as low-priority background tasks, automatically yielding resources when needed for data collection. See [AI & ML Features](/docs/category-overview-pages/machine-learning-and-assisted-troubleshooting.md)
- **Built-in Scalability**: Native clustering and high-availability features enable organizations to scale horizontally without architectural changes. See [Observability Centralization Points](/docs/deployment-guides/deployment-with-centralization-points.md).
- **Stable and Predictable Resource Usage**: Agents are optimized to spread work over time and avoid sudden changes in resource consumption

This lets Netdata monitor infrastructure at unprecedented scale with minimal overhead, from IoT devices to high-performance computing clusters.

### Agent Requirements (Per Node)

Even with Machine Learning enabled, Netdata Agents typically consume less than 5% CPU of a single core (see [CPU Requirements](/docs/netdata-agent/sizing-netdata-agents/cpu-requirements.md) for default-settings sizing guidance). Exact figures depend on metrics collected per server.

When connected to a Netdata Parent (the agent is in `child` mode), these requirements can be significantly reduced and, in the case of storage, even eliminated (no data on local disk).

| Type       | Metric/s | CPU                     | Memory        | Network  | Storage                   |
|------------|----------|-------------------------|---------------|----------|---------------------------|
| standalone | 3k - 10k | 4% - 20% of single core | 150 - 500 MiB | none     | varies based on retention |
| child      | 3k - 10k | 2% - 10% of single core | 100 - 300 MiB | \<1 Mbps | none                      |

For more information and ways to further reduce Agent resource utilization, see [Agent Resource Utilization](/docs/netdata-agent/sizing-netdata-agents/README.md).

### Parent Node Requirements (Centralization Points)

Parent nodes aggregate data from child agents, and their resource needs scale with metrics volume and deployment choices (clustering, centralized ML, etc).

Best practice: a cluster of Parents for every ~500 monitored nodes (2M metrics/second). See [Parent Sizing Guidelines](/docs/scalability.md#parent-sizing-guidelines) for the full breakdown:

| Monitored Nodes | Metric/s  | CPU Cores | Memory | Network  | Storage                   |
|-----------------|-----------|-----------|--------|----------|---------------------------|
| 500 nodes       | 2 million | 20 cores  | 80 GB  | 200 Mbps | varies based on retention |

These figures include ingestion and query resources.

For more information see [Agent Resource Utilization](/docs/netdata-agent/sizing-netdata-agents/README.md).

For high availability configurations, see [Clustering and High Availability of Netdata Parents](/docs/observability-centralization-points/metrics-centralization-points/clustering-and-high-availability-of-netdata-parents.md).

### Netdata Cloud On-Prem

For organizations requiring complete control over their monitoring infrastructure, Netdata Cloud On-Prem provides the full control plane within the datacenter (air-gapped).

Netdata Cloud is a Kubernetes cluster (Kubernetes 1.23+):

| Monitored Nodes | CPU Cores | Memory | Storage | Notes                  |
|-----------------|-----------|--------|---------|------------------------|
| 2,000 nodes     | 20 cores  | 45 GB  | 500 GB  | Minimal K8s cluster    |
| 10,000 nodes    | 100 cores | 200 GB | 2 TB    | Multi-node K8s cluster |

For more information see [Netdata Cloud On-Prem](https://github.com/netdata/netdata-cloud-onprem/blob/master/docs/learn.netdata.cloud/README.md) and [Installation Guide](https://github.com/netdata/netdata-cloud-onprem/blob/master/docs/learn.netdata.cloud/installation.md).

## Rollout / Lifecycle Management

Netdata Agent configuration uses simple text files that can be provisioned with configuration management tools, versioned in git, and maintained centrally at scale.

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

Agents and Parents authorize to Netdata Cloud via [claiming](/src/claim/README.md), using cryptographic certificates that can be controlled via provisioning tools or manually.

Users authenticate via email, Google OAuth, GitHub OAuth, Okta SSO, or OpenID Connect. See [Enterprise SSO Authentication](/docs/netdata-cloud/authentication-and-authorization/enterprise-sso-authentication.md). LDAP/AD groups can map automatically to Netdata roles via SCIM v2. See [SCIM Integration](/integrations/cloud-authentication/metadata.yaml).

Netdata Cloud segments infrastructure into spaces and rooms, with role-based access to each. See [Role-Based Access model (RBAC)](/docs/netdata-cloud/authentication-and-authorization/role-based-access-model.md).

## Scalability

Netdata scales horizontally with no theoretical or practical limit on ingestion and storage: standalone agents are independent of each other, and parent-child deployments scale by adding more parents.

The only practical limits are at query time: how much a browser or aggregation point can reasonably render at once, a constraint shared by all observability platforms. Netdata's distributed architecture (parallel, independent smaller queries) usually keeps this significantly better than other solutions, though it's still influenced by how much data is queried at once.

## Impact on the Monitored Infrastructure

Netdata is committed to being a polite citizen alongside other applications on monitored systems, a philosophy that drives every architectural decision:

- Any negative impact on monitored applications is treated as a severe bug
- Continuous optimization minimizes resource consumption while maximizing observability value
- Battle-tested by a large community across diverse environments

### Independent Validation

According to the [University of Amsterdam study](https://www.ivanomalavolta.com/files/papers/ICSOC_2023.pdf), examining the energy efficiency and impact of monitoring tools on Docker-based systems, Netdata was found to be the most energy-efficient tool for monitoring Docker-based systems. The study demonstrates that Netdata excels in:

- **CPU usage** - Lowest overhead among compared solutions
- **RAM usage** - Most memory-efficient monitoring approach  
- **Execution time impact** - Minimal interference with application performance
- **Energy consumption** - Best-in-class energy efficiency

## Data Retention and Storage

Metrics stay as close to the edge as possible, stored in write-once files rotated by size or time. Each Agent or Parent maintains 3 storage tiers by default (per-second, per-minute, per-hour), updated in parallel, with configurable retention per tier. High availability comes from streaming and parent clustering. See [Disk Requirements and Retention](/docs/netdata-agent/sizing-netdata-agents/disk-requirements-and-retention.md).

Logs use standard systemd-journal files (readable with `journalctl`), with standard systemd-journald practices and Forward Secure Sealing (FSS) support.

## Integration

Netdata is an open platform. It ingests metrics via common open standards (OpenMetrics, StatsD, OpenTelemetry, JSON, and more) and has approximately 800+ data collection modules and plugins.

Netdata can export metrics to Prometheus, InfluxDB, Graphite, OpenTSDB, TimescaleDB, and more.

For logs, standard systemd-journal practices apply.

For alert notifications, Netdata supports PagerDuty, Slack, Teams, email (SMTP), Discord, Telegram, Jira, ServiceNow, and custom webhooks.

For AI and Large Language Models, Netdata supports Model Context Protocol (MCP), available via Netdata Cloud for infrastructure-wide access (Paid plan) and on every Agent/Parent for direct local access (free, open-source). Netdata supports AI DevOps/SRE Copilots like Claude Code and Gemini CLI, and provides an AI Chat application (access to Google, OpenAI, Anthropic LLMs is required).

## Compliance and Security

**Compliance:** Netdata maintains SOC 2 Type 2 certification and complies with GDPR and CCPA. It is not officially PCI DSS or HIPAA certified, but aligns with their security practices and offers BAAs for healthcare organizations. See [Security and Privacy Design](/docs/security-and-privacy-design/README.md) for the full picture.

**Data Sovereignty:** All metrics and logs stay on customer infrastructure; only metadata (node names, chart titles, alert configs) syncs to Netdata Cloud, over TLS. Air-gapped deployment is available for maximum isolation.

**Enterprise Authentication:** SSO with LDAP/AD group mapping via SCIM 2.0, plus multi-factor authentication, IP whitelisting, role-based access control, and audit logging.

**Security Operations:** Regular third-party audits, penetration testing, an active bug bounty program, and configurable data anonymization and network segmentation.

## Support and Maintenance

Netdata offers tiered support, from community assistance to dedicated enterprise SLAs.

**Support Tiers:** Community support via GitHub Discussions and Discord. Paid plans add business-hours email/ticket support with SLA guarantees; enterprise agreements add 24/7 support and phone support.

**Professional Services:** Implementation services (architecture design, deployment, migration, integration) and training programs (administrator/user training, workshops, certification).

**Maintenance Services:** Proactive health checks, performance reviews, and capacity planning; update management with scheduled updates, emergency patches, and rollback procedures.

## Customization

Netdata's dashboards, alerts, and collectors are all customizable.

**Dashboard and Visualization:** Drag-and-drop dashboard creation, custom visualizations, shared dashboards, and template libraries, with dozens of chart types and export capabilities.

**Alert Management:** Custom thresholds, complex conditions, ML-based alerts, and schedule-based rules, with bulk configuration and version control integration.

**Extensibility:** The plugin architecture supports custom collectors written in Go (recommended for built-in modules), or in any language as an external plugin via the PLUGINSD protocol (commonly Python, Bash, or Node.js). Custom exporters support format customization, filtering, and transformation; API extensions enable webhooks and custom endpoints.

## Cost and Licensing

The open-source Agent provides full functionality with unlimited metrics. Netdata Cloud ranges from a free Community plan to paid plans with enterprise SSO and a self-hosted Enterprise On-Prem option. See [Netdata Cloud Pricing](https://www.netdata.cloud/pricing/) for current plan details.

Minimal infrastructure requirements, low bandwidth, efficient storage, and energy efficiency keep total cost of ownership down, alongside automated, self-maintaining deployment. Organizations typically see immediate ROI from zero-configuration visibility and automatic discovery, with long-term value from faster MTTR, prevented outages, and reduced tool sprawl.

## Training and Documentation

Full documentation is available at [learn.netdata.cloud](https://learn.netdata.cloud), including a [Getting Started Guide](/docs/deployment-guides/README.md), tutorials, and architecture deep-dives. Community resources include GitHub Discussions and Discord.

---

*For more information and detailed technical documentation, visit [learn.netdata.cloud](https://learn.netdata.cloud)*
