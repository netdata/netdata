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

For more information see [Deployment Strategies](https://github.com/netdata/netdata/blob/master/docs/deployment-guides/deployment-with-centralization-points.md).

## Infrastructure Requirements

Netdata has been architected to minimize resource consumption while maximizing observability capabilities. This efficiency is achieved through several key design decisions:

- **Storage Optimization**: Industry-leading compression achieving ~0.5 bytes per sample on disk for high-resolution data
- **Edge Computing**: Distributed architecture keeps data processing close to its source, reducing bandwidth and central processing requirements - Netdata distributes the code instead of centralizing the data
- **Intelligent ML**: Machine learning runs as low-priority background tasks, automatically yielding resources when needed for data collection. See [AI & ML Features](https://github.com/netdata/netdata/blob/master/docs/category-overview-pages/machine-learning-and-assisted-troubleshooting.md)
- **Built-in Scalability**: Native clustering and high-availability features enable organizations to scale horizontally without architectural changes. See [Observability Centralization Points]().
- **Stable and Predictable Resource Usage**: Agents are optimized to spread work over time and avoid sudden changes in resource consumption

This design philosophy enables Netdata to monitor infrastructure at unprecedented scale with minimal overhead, making it suitable for everything from IoT devices to high-performance computing clusters.

### Agent Requirements (Per Node)

Netdata Agent is designed to be lightweight, making it suitable for deployment across entire infrastructures. Even with Machine Learning enabled, Netdata Agents typically consume less than 5% CPU of a single core. The exact figures depend on the number of metrics collected per server.

When connected to a Netdata Parent (the agent is in `child` mode), these requirements can be significantly reduced and, in the case of storage, even eliminated (no data on local disk).

| Type       | Metric/s | CPU | Memory | Network | Storage |
|------------|---------------|-----------|---------|---------|---------|
| standalone | 3k - 10k | 4% - 20%<br/>of single core | 150 - 500 MiB | none | varies based on retention |
| child      | 3k - 10k | 2% - 10%<br/>of single core | 100 - 300 MiB | <1 Mbps | none |

For more information and ways to further reduce Agent resource utilization, see [Agent Resource Utilization](hhttps://github.com/netdata/netdata/blob/master/docs/netdata-agent/sizing-netdata-agents/README.md).

### Parent Node Requirements (Centralization Points)

Parent nodes aggregate data from multiple child agents and require resources that scale with the volume of metrics collected and other deployment decisions (clustered parents, centralized machine learning, etc).

The best practice is to have a cluster of Netdata Parents for every approximately 500 monitored nodes (2M metrics/second), like this:

| Monitored Nodes | Metric/s | CPU Cores | Memory | Network | Storage |
|-----------------|----------|-----------|---------|---------|---------|
| 500 nodes       | 2 million | 20 cores  | 80 GB | 200 Mbps | varies based on retention |

These figures include ingestion and query resources.

For more information see [Agent Resource Utilization](https://github.com/netdata/netdata/blob/master/docs/netdata-agent/sizing-netdata-agents/README.md).

For high availability configurations, see [Clustering and High Availability of Netdata Parents](https://github.com/netdata/netdata/blob/master/docs/observability-centralization-points/metrics-centralization-points/clustering-and-high-availability-of-netdata-parents.md).

### Netdata Cloud On-Prem

For organizations requiring complete control over their monitoring infrastructure, Netdata Cloud On-Prem provides the full control plane within the datacenter (air-gapped).

Netdata Cloud is a Kubernetes cluster (Kubernetes 1.23+):

| Monitored Nodes | CPU Cores | Memory | Storage | Notes |
|-----------------|-----------|---------|---------|--------|
| 2,000 nodes     | 20 cores  | 45 GB   | 500 GB  | Minimal K8s cluster |
| 10,000 nodes    | 100 cores | 200 GB  | 2 TB    | Multi-node K8s cluster |

For more information see [Netdata Cloud On-Prem](https://github.com/netdata/netdata-cloud-onprem/blob/master/docs/learn.netdata.cloud/README.md) and [Installation Guide](https://github.com/netdata/netdata-cloud-onprem/blob/master/docs/learn.netdata.cloud/installation.md).

## Rollout / Lifecycle Management

Netdata users have multiple options to deploy Netdata on their systems. Netdata Agent configuration uses simple text files that can be provisioned with configuration management tools, versioned in git repositories, and maintained centrally for large infrastructures.

### Agent and Parent Deployment Options

**1. Configuration Management Tools**: Use Ansible, Puppet, Chef, Salt, Terraform
**2. Container Deployment**: Use official Docker images
**3. Native Package Management**: Use native GPG-signed packages for Debian, Ubuntu, Red Hat, CentOS, SUSE, etc., with the ability to mirror Netdata repositories internally for air-gapped environments
**4. One-Line Installation with Auto-Updates**: Single command installation that configures automatic updates, with a choice of release channels: stable (recommended) or nightly

**Note**: Netdata Parents use the same software as Agents with different configuration. All deployment methods apply to both.

For more information about Netdata Agents & Parents installation methods see [Netdata Agent Installation](https://github.com/netdata/netdata/blob/master/packaging/installer/README.md).

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

Netdata Agents and Parents are authorized to connect to Netdata Cloud via a process called [claiming](https://github.com/netdata/netdata/blob/master/src/claim/README.md). This process uses cryptographic certificates to authorize the agents to connect to Netdata Cloud and can be controlled via configuration management/provisioning tools, or manually.

To authorize users, Netdata Cloud supports email authentication, Google OAuth, GitHub OAuth, Okta SSO, OpenID Connect (OIDC). For more information see [Enterprise SSO Authentication](https://github.com/netdata/netdata/blob/master/docs/netdata-cloud/authentication-and-authorization/enterprise-sso-authentication.md).

Netdata supports automatic mapping of LDAP/AD groups into Netdata roles, using the System for Cross-domain Identity Management (SCIM) v2. For more information see [SCIM Integration](https://github.com/netdata/netdata/blob/master/integrations/cloud-authentication/metadata.yaml).

To control access to observability data, Netdata Cloud uses spaces and rooms to segment the infrastructure (by geography, type, or any grouping the organization decides). Users gain access to spaces and rooms and, depending on their role, they may have different access to different types of observability data. For more information see [Role-Based Access model (RBAC)](https://github.com/netdata/netdata/blob/master/docs/netdata-cloud/authentication-and-authorization/role-based-access-model.md).

## Scalability

Netdata scales horizontally. There is no limit on how large a Netdata monitored infrastructure can scale, although there are several practical usability limits when it comes to using the observability platform.

For data ingestion and storage, there is no limit, theoretical or practical. When using standalone agents, each agent is independent of the others. When using parent-child agents, each parent is sized independently of the others and the system scales by adding more parents.

Practical limits exist when querying observability data, mainly due to the amount of information web browsers and aggregation points can reasonably handle at once. The system becomes slower depending on the amount of information queried at once (i.e. how many nodes, instances, time-series appear on the same chart or view).

Due to the distributed nature of the architecture, Netdata responsiveness is usually significantly better compared to other monitoring solutions. Still, it is influenced by the amount of data queried at once.

## Impact on the Monitored Infrastructure

Netdata takes the impact on monitored infrastructure extremely seriously. Netdata is committed to being a friendly and polite citizen alongside other applications and services running on the monitored systems. This philosophy drives every architectural decision.

**Netdata's Commitment:**
- Any negative impact on monitored applications is considered a severe bug that must be fixed
- Continuous optimization to minimize resource consumption while maximizing observability value
- The solution is battle-tested by a large community across diverse environments
- Years of engineering effort have been invested to ensure Netdata performs exceptionally in resource efficiency

### Independent Validation

According to the [University of Amsterdam study](https://www.ivanomalavolta.com/files/papers/ICSOC_2023.pdf), examining the energy efficiency and impact of monitoring tools on Docker-based systems, Netdata was found to be the most energy-efficient tool for monitoring Docker-based systems. The study demonstrates that Netdata excels in:

- **CPU usage** - Lowest overhead among compared solutions
- **RAM usage** - Most memory-efficient monitoring approach  
- **Execution time impact** - Minimal interference with application performance
- **Energy consumption** - Best-in-class energy efficiency

## Data Retention and Storage

For metrics, Netdata maintains ingested samples as close to the edge as possible. Netdata Parents receive these data and each parent in a cluster maintains its own copy of them. All metrics are stored in write-once files which are rotated according to configuration (size and/or time based). Each Netdata Agent or Parent maintains by default 3 storage tiers of the same data (high resolution - per second, medium resolution - per minute, low resolution - per hour). All tiers are updated in parallel during ingestion. Users can configure size and time retention per tier. High availability of the data is achieved via streaming (parent-child relationships) and parents clustering.

For detailed information see [Disk Requirements and Retention](https://github.com/netdata/netdata/blob/master/docs/netdata-agent/sizing-netdata-agents/disk-requirements-and-retention.md).

For logs, Netdata uses standard systemd-journal files (readable with journalctl). Standard systemd-journald practices apply (archiving, backup, centralization, exporting, etc) and Forward Secure Sealing (FSS) is supported.

## Integration

Netdata is an open platform.

It can ingest metrics in commonly used open standards, including OpenMetrics, StatsD, JSON, etc. OpenTelemetry support is scheduled for Q3 2025. Netdata also has approximately 800+ data collection modules and plugins to directly collect data from applications.

Netdata can export metrics to Prometheus, InfluxDB, Graphite, OpenTSDB, TimescaleDB, and more.

For logs, standard systemd-journal practices apply.

For alert notifications, Netdata supports PagerDuty, Slack, Teams, email (SMTP), Discord, Telegram, Jira, ServiceNow, and custom webhooks.

For AI and Large Language Models, Netdata supports Model Context Protocol (MCP), supports AI DevOps/SRE Copilots like Claude Code and Gemini CLI, and provides an AI Chat application (access to Google, OpenAI, Anthropic LLMs is required).

## Compliance and Security

Netdata provides enterprise-grade security and compliance capabilities designed to meet the requirements of regulated industries and security-conscious organizations.

**Compliance Standards:**
Netdata maintains SOC 2 Type 1 certification and is pursuing Type 2 certification, with ISO 27001 on the roadmap. The platform aligns with GDPR, CCPA, HIPAA (with BAA available), and PCI DSS requirements, making it suitable for financial services, healthcare, and government deployments.

**Data Sovereignty Architecture:**
The platform's architecture ensures complete data sovereignty - all metrics and logs remain on customer infrastructure. Only metadata (node names, chart titles, alert configurations) is synchronized to Netdata Cloud, with all data transmission protected by TLS encryption. Air-gapped deployment options are available for maximum security isolation.

**Enterprise Authentication:**
Comprehensive SSO integration supports LDAP/AD group mapping through SCIM 2.0, enabling automatic role assignment based on organizational structure. Multi-factor authentication, IP whitelisting, and role-based access control provide granular security controls with comprehensive audit logging.

**Security Operations:**
Regular third-party security audits, penetration testing, and an active bug bounty program ensure continuous security improvement. The platform supports configurable data anonymization and network segmentation for sensitive environments.

For comprehensive security information see [Security and Privacy Design](https://github.com/netdata/netdata/blob/master/docs/security-and-privacy-design/README.md).

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
The platform provides drag-and-drop dashboard creation with custom visualizations, shared dashboards, and template libraries. Over 30 chart types are available with custom color schemes, responsive layouts, and export capabilities.

**Alert Management:**
Comprehensive alert customization includes custom thresholds, complex conditions, ML-based alerts, and schedule-based rules. Pre-built and custom alert templates support bulk configuration with version control integration.

**Extensibility:**
The plugin architecture supports custom collectors in multiple languages (Go recommended, Python, Bash, Node.js). Custom exporters offer format customization, filtering rules, transformation options, and batch processing. API extensions enable webhook development, custom endpoints, data processing, and event handling.

## Cost and Licensing

Netdata offers flexible licensing models designed to accommodate organizations of all sizes, from open-source deployments to enterprise on-premises installations.

**Licensing Options:**
The open-source version provides full agent functionality with unlimited metrics and community support. Netdata Cloud plans range from a free Community plan (5 nodes, 1 user) to Business plans (unlimited nodes, enterprise SSO) and Enterprise On-Prem (self-hosted control plane with custom contracts).

**Total Cost of Ownership:**
Minimal infrastructure requirements, low bandwidth usage, efficient storage, and energy efficiency contribute to reduced infrastructure costs. Operational benefits include automated deployment, self-maintaining systems, minimal administration requirements, and low training needs.

**Return on Investment:**
Organizations typically realize immediate benefits through instant visibility, zero configuration overhead, automatic discovery, and real-time insights. Long-term value includes predictive capabilities, capacity optimization, performance improvements, operational efficiency gains, faster MTTR, prevented outages, and reduced tool sprawl.

## Training and Documentation

Netdata provides comprehensive training and documentation resources designed to support successful deployment and ongoing operations across enterprise environments.

**Documentation Resources:**
Extensive documentation is available at [learn.netdata.cloud](https://learn.netdata.cloud), including a comprehensive [Getting Started Guide](https://github.com/netdata/netdata/blob/master/docs/deployment-guides/README.md), step-by-step tutorials, video walkthroughs, and architecture deep-dives. Reference materials include API documentation, configuration guides, troubleshooting resources, and best practices.

**Training Programs:**
Self-service learning options include interactive tutorials, hands-on labs, video courses, and certification paths. Instructor-led training provides virtual workshops, on-site training, custom curriculum development, and train-the-trainer programs.

**Community and Support:**
Active community resources include forums, Discord server, GitHub discussions, and user contributions. Use case examples cover industry solutions, architecture patterns, success stories, and performance benchmarks. Onboarding support includes quick start guides, deployment assistance, initial configuration help, and health checks.

---

*For more information and detailed technical documentation, visit [learn.netdata.cloud](https://learn.netdata.cloud)*
