# Welcome to Netdata

## The Challenge

As infrastructures become more ephemeral, distributed, and complex, traditional centralized observability pipelines are reaching their limits. Container orchestration, microservices, multi-cloud deployments, and edge computing have created environments where infrastructure changes faster than monitoring can be configured, data volumes exceed what centralized systems can economically process, and the skills gap between what monitoring requires and what teams possess continues to widen.

## What is Netdata?

Netdata is a distributed, real-time observability platform that monitors metrics and logs from systems and applications, built on a foundation designed to seamlessly extend to distributed tracing. It collects data at per-second granularity, stores it at (or as close to) the edge where it's generated, provides automated dashboards, machine learning anomaly detection, and AI-powered analysis without requiring configuration or specialized skills.

Instead of centralizing the data, Netdata distributes the monitoring code to each system, keeping data local while providing unified access. This architecture enables linear scaling to millions of metrics per second and terabytes of logs while offering significantly faster queries.

The platform is designed for operations teams, sysadmins, DevOps engineers, and SREs who need comprehensive real-time, low-latency visibility into their infrastructure and applications. Netdata is opinionated — it collects everything, visualizes everything, runs machine learning anomaly detection on everything, with several innovations that make modern observability accessible to lean teams, without the need for specialized skills.

The system consists of three components:
- **Netdata Agent**: Monitoring software installed on each system
- **Netdata Parents**: Optional centralization points for aggregating data from multiple agents (Netdata Parents are the same software component as Netdata Agents, configured as Parents)
- **Netdata Cloud**: A smart control plane for unifying multiple independent Netdata Agents and Parents, providing horizontal scalability, role based access control, access from anywhere, centralized alerts notifications, team collaboration, AI insights and more.

## Performance at a Glance

| Aspect | Netdata | Industry Standard |
|--------|---------|-------------------|
| **Real-Time Monitoring** | | |
| Data granularity | 1 second | 10-60 seconds |
| Collection to visualization | 1 second | 30+ seconds |
| Time to first dashboard | 10 seconds | Hours to days |
| **Automation** | | |
| Configuration required | Minimal to none | Extensive |
| ML anomaly detection | All metrics automatically | Selected metrics manually |
| Pre-configured alerts | 400+ out of the box | Build from scratch |
| **Efficiency** | | |
| Storage per metric | 0.6 bytes/sample | 2-16 bytes/sample |
| Agent CPU usage | 5% single core | 10-30% single core |
| Scalability | Linear, unlimited | Exponential complexity |
| **Coverage** | | |
| Metrics collected | Everything available | Manually selected |
| Built-in collectors | 800+ integrations | Basic system metrics |
| Hardware monitoring | Comprehensive | Limited or none |
| Live monitoring | processes, network connections, containers, systemd services and units | Limited or none |

## Design Philosophy and Implementation

### Data at the Edge

Observability data is vast, usually orders of magnitude larger than actual business data. Observability solutions struggle to scale, and even when they do scale, their complexity increases drastically and their total cost of ownership becomes unreasonable, in many cases matching or exceeding the actual infrastructure cost.

At the same time, observability data is collected, stored and analyzed, but only a small percentage is actually ever viewed. The vast majority of observability data is there in case they are needed for troubleshooting or post-mortem analysis.

Netdata keeps the observability data at the edge (Netdata Agents), or as close to the edge as possible (Netdata Parents).

**Implementation**: Each Netdata Agent is a complete monitoring system with collection, storage, query engine, visualization, machine learning, and alerting. This isn't just an agent that ships data elsewhere — it's a full observability stack. The distributed architecture provides:

- **Data sovereignty**: Data is always stored on-premises and only leaves the servers when viewed. This ensures compliance with GDPR, HIPAA, and regional data residency requirements.
- **Linear scalability**: Adding more Netdata Agents and Parents does not affect the existing ones.
- **Monitoring in isolation**: Observability works even when internet connectivity faces difficulties.
- **Universal capture**: All observability data exposed by systems and applications are important and are collected, enriching the views and the depth of the possible analysis available.
- **High-fidelity insights**: High-resolution (per-second) data capture the micro world at which our infrastructures operate, surfacing the breadth and pulse of our applications and the sequence of cascading effects.

### Complete Coverage

Observability solutions are usually selective to control cost, complexity and the time and skills required to set up. Organizations are frequently instructed to select only what is important for them, based on their understanding and needs. This creates two fundamental problems: missing just one uncollected metric can obscure the root cause of an issue (leading to frustration and incomplete visibility during crisis), and the observability quality organizations get reflects the skills and experience of their people.

Netdata's design allows it to capture everything exposed by systems and applications — every metric, every log entry, every piece of telemetry available.

The comprehensive approach ensures:

- **No blind spots**: The metric you didn't know to monitor is already collected and visualized.
- **Skill-independent quality**: Junior and senior engineers get the same comprehensive visibility.
- **Crisis-ready coverage**: When incidents occur, all relevant data is available.
- **Full context for AI**: Machine learning and AI assistants have complete data to identify patterns and correlations.

### Real-Time, Low-Latency Visibility

Most observability solutions lower granularity (the frequency data is collected) to control cost and scalability. For most of them 'real-time' is at best every 10 or 15 seconds. And for all of them even this is not strict, it can fluctuate without any direct impact on the analysis. Furthermore, observability pipelines introduce latency in making the data available and it is common to have dozens of seconds, or even minutes, of delay between data collection and visualization. These inherent weaknesses make observability a statistical analysis tool, not able to keep up with the actual pace of the infrastructure, forcing engineers to use console tools when precise, accurate and on-time information is required.

Netdata collects everything per-second and has a fixed one-second data collection to visualization latency. Furthermore, Netdata works on a beat. Every sample needs to be collected on time. Missing a sample on time indicates that the monitored component or application is under stress, and Netdata shows gaps on the charts. This strict real-time approach delivers:

- **True real-time visibility**: See what's happening now, not what happened 30 seconds ago.
- **Console-quality precision**: No need to SSH into servers for real-time data during incidents.
- **Stress detection**: Gaps in charts immediately reveal when systems and applications are under stress.
- **Accurate sequencing**: Understand the exact order of cascading failures across systems.
- **Live troubleshooting**: Watch the immediate impact of your changes as you make them.
- **Tools consolidation**: Use a single uniform and universal dashboard for all systems and applications.

### Data Accessibility

Observability solutions assume users know and understand the data before they collect and visualize them. Many solutions require from users to also know the exact types and kinds of collected data in order to visualize them properly. Most solutions require from users to manually set up charts by learning a query language and configure dashboards, organizing them in a manner that is meaningful. This is usually the biggest obstacle to proper observability. Discipline, skills and a huge amount of work for something that should be there by default.

Netdata's solution comes from the observation that most of our infrastructure components are common: operating systems, databases, web servers, message brokers, containers, storage devices, network devices, and so on. We all use the same finite set of components, plus a few custom applications.

Netdata dashboards are an algorithm, not a configuration. Each Netdata chart is a complete analytical tool that provides a 360 view of the data and its sources, allowing slicing and dicing of any data-set using point and click, optimized to provide a comprehensive view of what is available and where data is coming from. Netdata provides single-node, multi-node, and infrastructure level dashboards automatically. All metrics are organized in a meaningful manner with a universal table of contents that dynamically adapts to the data available, providing instant access to every metric. This approach delivers:

- **Zero learning curve**: No query languages, no manual dashboard building, no configuration.
- **Instant time to value**: Complete visibility from the moment of installation.
- **Universal navigation**: The same logical structure across all organizations and infrastructures.
- **Interactive exploration**: Point-and-click analysis without knowing metric names or data types.
- **Skill democratization**: Everyone from junior to senior engineers gets the same powerful tools.

### Efficient Storage

Centralized observability solutions introduce significant storage requirements in both capacity and I/O throughput, making observability the most important consumer of storage systems in the infrastructure.

Netdata is optimized for lightweight storage operations. Three storage tiers are updated in parallel (per-second, per-minute, per-hour). The high-resolution tier needs 0.6 bytes per sample on disk (Gorilla compression + ZSTD). The lower resolution tiers need 6-bytes and 18-bytes per sample respectively and maintain the ability to provide the same min, max, average and anomaly rate the high-resolution tier provides. Data are written in append-only files and are never reorganized on disk (Write Once Read Many - WORM). Writes are spread evenly over time. Netdata Agents write at 5 KiB/s, Netdata Parents aggregating 1M metrics/s write at 1MiB/s across all tiers.

Netdata implements a custom time-series database optimized for the specific patterns of system metrics:

- **Write-once design**: Append-only architecture for maximum performance
- **Multi-tier storage**: Three storage tiers of different resolution, updated in parallel
- **Zero maintenance**: No recompaction or database maintenance windows

This efficient storage architecture delivers years of data in gigabytes rather than terabytes, with predictable I/O patterns and linear scaling of storage requirements with infrastructure size.

### Logs Management

Log management has become one of the largest cost drivers in observability, with organizations spending millions on storage and processing infrastructure. Many resort to aggressive filtering and sampling just to make costs manageable, inevitably losing critical information when they need it most.

Netdata takes a fundamentally different approach by leveraging the systemd journal format, native to Linux systems. This edge-based approach provides enterprise-grade capabilities without the enterprise costs:

- **Direct file access**: No query servers needed — clients open journal files directly, leveraging OS disk cache for blazing-fast performance
- **Comprehensive indexing**: Every field in every log entry is automatically indexed, enabling instant queries across millions of entries
- **Flexible schema**: Each log entry can have its own unique set of fields and values, all fully indexed and searchable
- **Efficient storage**: Journal files match uncompressed text log sizes while providing full indexing — a balance between space efficiency and query performance
- **Native tooling**: Built-in support for centralization, filtering, exporting, and integration with existing pipelines
- **Security built-in**: Write Once Read Many (WORM) and Forward Secure Sealing (FSS) ensures log integrity and tamper detection
- **Logs transformation**: The platform includes `log2journal` for converting any text, JSON, or logfmt logs into structured journal entries

Where traditional solutions sample 5,000 log entries to generate field statistics on their dashboards, Netdata starts sampling at 1 million entries, providing 200x more accurate insights into log patterns. 
The result is enterprise-grade log management capabilities — field statistics, histogram breakdowns, full-text search, time-based filtering — all while keeping logs at the edge where they're generated, eliminating the massive costs of centralized log infrastructure.

Note: On Windows Netdata queries Windows Event Logs (WEL), Event Tracing for Windows (ETW) and TraceLogging (TL) via the Event Log.

### AI and Machine Learning

Machine Learning (ML) in monitoring requires data scientists, training periods, and careful model management. This makes it accessible only to a few organizations with specialized teams, and even then is used selectively with limited scope.

However, ML is the simplest way to model the behavior of our systems and applications. When done properly, ML can reliably detect anomalies, surface correlations between components and applications, provide valuable information about cascading effects under crisis, identify the blast radius of issues and even detect infrastructure level issues independently of the configured alerts.

Netdata democratizes ML and AI by making it automatic and universal (no configuration is required). The system trains 18 k-means models per metric using different time windows, requiring unanimous agreement before flagging anomalies. This achieves a false positive rate of 10^-36 (1% per model ^ 18 models) while remaining sensitive to real issues:

- **Continuous training**: Models train automatically as data arrives
- **Real-time detection**: Anomaly detection runs instantly, not in batches
- **Efficient storage**: Results store in just 1 bit per metric per second
- **Correlation analysis**: Engine identifies related anomalies across metrics
- **Unbiased detection**: Anomaly detection is not influenced by future events

Note: Netdata's ML focuses on detecting behavioral anomalies in metrics using their last 2 days of data. It is optimized for reliability rather than sensitivity and may miss slow (over days/weeks) infrastructure degradation or certain types of long-term anomalies (weekly, monthly, etc). However, it typically detects most types of abnormal behavior that break services.

### Troubleshooting

During a crisis, engineers typically need to make assumptions about possible root causes and validate or drop these assumptions. This is a painful process requiring expertise, deep understanding of the monitored infrastructure and dependencies that usually leads to days or weeks of investigation.

Netdata introduces a significant shift to this process utilizing its unsupervised and real-time anomaly detection system. The "Anomaly Advisor" transforms troubleshooting:

- **Automatic scoring**: Ranks all metrics by anomaly severity within any time window
- **Root cause prioritization**: Surfaces the most likely culprits in the first 30-50 metrics
- **Sequence analysis**: Reveals the order of cascading failures across systems
- **Blast radius mapping**: Determines the full impact scope of incidents
- **AI-ready insights**: Provides structured data that AI assistants use to narrow investigations

This approach still requires interpretation skills but dramatically simplifies the investigation process compared to traditional methods (the aha! moment is within the first 30-50 results).

### Alerts

Most monitoring solutions focus on aggregate metrics and business-level alerts, often missing component failures until they cascade into service outages. This approach leads to alert fatigue from false positives and missed issues from incomplete coverage.

Netdata takes a fundamentally different approach: templated alerts that monitor individual component and application instances. Each alert watches a single instance, building a comprehensive safety net where every component has its own watchdog. This granular approach ensures:

- **Complete coverage**: Every database, web server, container, and service instance has dedicated monitoring
- **Early detection**: Component failures are caught before they cascade into service-wide issues
- **Clear accountability**: Alerts identify exactly which instance is failing, not just that "something is wrong"
- **Scalable alerting**: Templates automatically apply to new instances as infrastructure grows
- **Synthetic checks**: Lightweight integration tests that validate connectivity and behavior between applications complements component monitoring

Netdata ships with hundreds of pre-configured alerts, many intentionally silent by default. These silent alerts monitor important but non-critical conditions that should be reviewed but shouldn't wake engineers at 3am. This pragmatic approach balances comprehensive monitoring with operational sanity.

### Scalability

Centralized monitoring architectures hit bottlenecks — ingestion pipelines overflow, storage systems struggle, query engines slow down. Adding more infrastructure makes the monitoring system itself harder to manage.

Netdata's philosophy is that scalability must be inherent to the architecture, not an expensive add-on. Designed to be fully distributed, Netdata achieves linear scalability through:

- **Independent operation**: Each Agent and Parent operates autonomously without affecting others.
- **Horizontal scaling**: Add more Parents to handle more Agents without redesigning architecture.
- **Consistent performance**: Query response times remain the same whether you have 10 or 10,000 nodes.
- **Resource predictability**: Resource usage scales linearly with infrastructure size.
- **High availability**: Streaming and replication provide high-availability to Netdata deployments.
- **Clustering**: Netdata Parents can be clustered to replicate all their data localy, or cross region for disaster recovery.
- **Fail-over**: Netdata Cloud dynamically routes queries to Netdata Parents and Agents based on their availability.

### Open Ecosystem

Netdata thrives as part of a vibrant open-source community with 1.5 million downloads per day. The platform integrates seamlessly with existing tools and standards:

- **Metrics collection**: Ingests metrics via all open standards (OpenTelemetry in final release stage)
- **Metrics export**: Exports metrics to all open standards and commonly used time-series databases (Prometheus, Graphite, InfluxDB, OpenTSDB, and more)
- **Logs**: Uses battle tested systemd journal files for storing logs, providing maximum interoperability
- **Alert routing**: Delivers notifications to PagerDuty, Slack, email, webhooks, and 20+ platforms
- **AI integration**: Supports AI assistants via Model Context Protocol (MCP)
- **Visualization**: Works with Grafana through native datasource plugin
- **Container orchestration**: Integrates with Kubernetes, Docker Swarm, and Nomad

Netdata can operate independently or alongside your existing observability stack. Whether you use Prometheus, Grafana, OpenTelemetry, or centralized log aggregators, Netdata enhances visibility without disrupting existing workflows.

## Working with Netdata

Typically, organizations deploying Netdata need to:

1. Install Netdata Agents on all Linux, Windows, FreeBSD and MacOS physical servers and VMs
2. Optionally: dedicate resources (VMs, storage) for Netdata Parents, providing high-availability and longer retention to observability data
3. Optionally: configure logs transformation with `log2journal` and centralization using typical systemd-journald methodologies
4. Configure collectors that need credentials to access protected applications (databases, message brokers, etc), data collection for custom applications, enable SNMP discovery and data collection, install Netdata with auto-discovery in Kubernetes clusters
5. Review alerts (Netdata ships with preconfigured alerts) and set up alert notification channels
6. Invite colleagues (enterprise SSO via IODC, Okta and SCIMv2 supported), assign roles and permissions

Netdata will automatically provide:

1. Complete coverage of hardware, operating system and application metrics
2. Real-time, low-latency Metrics and Logs Dashboards
3. Live and interactive exploration of running processes, network connections, systemd units, systemd services, IMPI sensors, and more
4. Unsupervised machine-learning based anomaly detection for all metrics
5. Hundreds of pre-configured alerts for systems and applications
6. AI insights (reports) and AI-assistant (chat) connections via MCP

Custom dashboards are supported but are optional. Netdata provides single-node, multi-node and infrastructure level dashboards automatically.

Netdata configurations are infrastructure-as-code friendly, and provisioning systems can be used to automate deployment on large infrastructures.
A complete Netdata deployment is usually achieved within a few days.

## Resource Requirements

Netdata is committed to having best-in-class resource utilization. Wasted resources are considered bugs and are addressed with high priority.

Based on extensive real-world deployments and independent academic validation, Netdata maintains minimal resource footprint:

| Resource               | Standalone 5k metrics/s | Child 5k metrics/s  | Parent 1M metrics/s |
|------------------------|:-----------------------:|:-------------------:|:-------------------:|
| **CPU**                | 5% of a single core     | 3% of a single core | ~10 cores total     |
| **Memory**             | 200 MB                  | 150 MB              | ~40 GB              |
| **Network**            | None                    | <1 Mbps to Parent   | ~100 Mbps inbound   |
| **Storage Capacity**   | 3 GiB (configurable)    | None                | as needed           |
| **Storage Throughput** | 5 KiB/s write           | None                | 1 MiB/s write       |
| **Retention**          | 1 year (configurable)   | None                | as needed           |

Notes:
- Parent resources include both ingestion and query workload
- Storage rates are for all tiers combined; actual disk usage depends on retention configuration
- The recommended topology is having a cluster of Netdata Parents every 500 monitored nodes (2M metrics/s)

The University of Amsterdam study found Netdata to be the most energy-efficient monitoring solution, with the lowest CPU overhead, memory usage, and execution time impact among compared tools.

## Practical Implications

### For Small Teams

Without dedicated monitoring staff, teams need systems that work without constant attention. Netdata's automatic operation enables teams to:

- Eliminate configuration maintenance as infrastructure changes
- Access instant dashboards during incidents without building them
- Remove the guesswork of threshold tuning as patterns evolve
- Achieve complete visibility with zero learning curve

### For Large Organizations

At scale, traditional monitoring becomes expensive and complex. Netdata's architecture enables organizations to:

- Gain predictable costs based on node count, not data volume
- Ensure consistent performance from 10 to 10,000 systems
- Match monitoring architecture to organizational structure
- Satisfy data locality requirements (GDPR, HIPAA) by design

### For Dynamic Environments

Modern infrastructure changes constantly. Netdata enables teams to:

- See new containers in dashboards immediately upon creation
- Maintain clean views as resources are deleted automatically
- Track relationships that update dynamically as services scale
- Benefit from ML models that continuously adapt to new patterns

## Summary

Netdata represents a fundamental rethink of monitoring architecture. By processing data at the edge, automating configuration, maintaining real-time resolution, applying ML universally, and making data accessible to everyone, it solves core monitoring challenges that have persisted for decades.

The result is a monitoring system that deploys in minutes instead of months, scales efficiently to any size, adapts automatically to changes, and provides insights that would be impossible with traditional approaches — all while remaining open source and community driven.

Whether you're monitoring a single server or a global infrastructure, Netdata's design philosophy creates a monitoring system that works with you rather than demanding constant attention.
