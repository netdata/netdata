# Welcome to Netdata

## Who we are

Netdata is a distributed, real-time observability platform that monitors metrics and logs from systems and applications, built on a foundation designed to seamlessly extend to distributed tracing. It collects data at per-second granularity, stores it at (or as close to) the edge where it's generated, provides automated dashboards, machine learning anomaly detection, and AI-powered analysis without requiring configuration or specialized skills.

Instead of centralizing the data, Netdata distributes the monitoring code to each system, keeping data local while providing unified access. This architecture enables linear scaling to millions of metrics per second and terabytes of logs while offering significantly faster queries.

We have designed this platform for operations teams, sysadmins, DevOps engineers, and SREs who need comprehensive real-time, low-latency visibility into their infrastructure and applications. Netdata is opinionated — it collects everything, visualizes everything, runs machine learning anomaly detection on everything, with several innovations that make modern observability accessible to lean teams, without the need for specialized skills.

The system consists of three components:
- [**Netdata Agent**](/docs/deployment-guides/standalone-deployment.md): Monitoring software installed on each system
- [**Netdata Parents**](https://github.com/netdata/netdata/blob/master/docs/deployment-guides/deployment-with-centralization-points.md): Optional centralization points for aggregating data from multiple agents (Netdata Parents are the same software component as Netdata Agents, configured as Parents)
- [**Netdata Cloud**](https://github.com/netdata/netdata/blob/master/docs/netdata-cloud/README.md): A smart control plane for unifying multiple independent Netdata Agents and Parents, providing horizontal scalability, role based access control, access from anywhere, centralized alerts notifications, team collaboration, AI insights and more.

## Performance at a Glance

| Aspect | Netdata | Industry Standard |
|-------:|:-------:|:-----------------:|
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
| Live monitoring | processes, network connections, and more | Limited or none |

## Design Philosophy and Implementation

### Data at the Edge

:::note

Netdata keeps the observability data at the edge (Netdata Agents), or as close to the edge as possible (Netdata Parents).

:::

:::tip

Keeping data at the edge eliminates egress charges, ensures compliance by default, and transforms observability from an unpredictable cost center into a fixed operational expense while delivering sub-second query performance.

:::

**Implementation**: Each Netdata Agent is a complete monitoring system with collection, storage, query engine, visualization, machine learning, and alerting. This isn't just an agent that ships data elsewhere — it's a full observability stack. The distributed architecture provides:

- **Data sovereignty**: Data is always stored on-premises and only leaves the servers when viewed. This ensures compliance with GDPR, HIPAA, and regional data residency requirements.
- **Linear scalability**: Adding more Netdata Agents and Parents does not affect the existing ones.
- **Monitoring in isolation**: Observability works even when internet connectivity faces difficulties.
- **Universal capture**: All observability data exposed by systems and applications are important and are collected, enriching the views and the depth of the possible analysis available.
- **High-fidelity insights**: High-resolution (per-second) data capture the micro world at which our infrastructures operate, surfacing the breadth and pulse of our applications and the sequence of cascading effects.

### Complete Coverage

:::note

Most observability solutions are usually selective to control cost, complexity and the time and skills required to set up. Organizations are frequently instructed to select only what is important for them, based on their understanding and needs.

This creates two fundamental problems:
- missing just one uncollected metric can obscure the root cause of an issue, leading to frustration and incomplete visibility during crisis
- the observability quality organizations get reflects the skills and experience of their people.

:::

Netdata's design allows it to capture everything exposed by systems and applications — every metric, every log entry, every piece of telemetry available.

The comprehensive approach ensures:

- **No blind spots**: The metric you didn't know to monitor is already collected and visualized.
- **Skill-independent quality**: Junior and senior engineers get the same comprehensive visibility.
- **Crisis-ready coverage**: When incidents occur, all relevant data is available.
- **Full context for AI**: Machine learning and AI assistants have complete data to identify patterns and correlations.

### Real-Time, Low-Latency Visibility

:::note

Most observability solutions collect data every 10-60 seconds with additional pipeline delays of seconds to minutes, making them statistical analysis tools rather than real-time monitoring. This forces engineers to SSH into servers for accurate, timely data during incidents.

:::

Netdata collects everything per-second and has a fixed one-second data collection to visualization latency. Netdata works on a beat. Every sample needs to be collected on time. Delays in data collection indicate that the monitored component or application is under stress, and Netdata shows gaps on the charts. This strict real-time approach delivers:

- **True real-time visibility**: See what's happening now, not what happened 30 seconds ago.
- **Console-quality precision**: No need to SSH into servers for real-time data during incidents.
- **Stress detection**: Gaps in charts immediately reveal when systems and applications are under stress.
- **Accurate sequencing**: Understand the exact order of cascading failures across systems.
- **Live troubleshooting**: Watch the immediate impact of your changes as you make them.
- **Tools consolidation**: Use a single uniform and universal dashboard for all systems and applications.

### Data Accessibility

:::note

Most observability solutions require users to learn query languages, manually build dashboards, and understand metric types before they can visualize data. This prerequisite knowledge and configuration work becomes the biggest barrier to effective monitoring.

:::

Most of our infrastructure components are common: operating systems, databases, web servers, message brokers, containers, storage devices, network devices, and so on. We all use the same finite set of components, plus a few custom applications.

Netdata dashboards are an algorithm, not a configuration. Each Netdata chart is a complete analytical tool that provides a 360 view of the data and its sources, allowing slicing and dicing of any data-set using point and click, optimized to provide a comprehensive view of what is available and where data is coming from. Netdata provides single-node, multi-node, and infrastructure level dashboards automatically. All metrics are organized in a meaningful manner with a universal table of contents that dynamically adapts to the data available, providing instant access to every metric. This approach delivers:

- **Zero learning curve**: No query languages, no manual dashboard building, no configuration.
- **Instant time to value**: Complete visibility from the moment of installation.
- **Universal navigation**: The same logical structure across all organizations and infrastructures.
- **Interactive exploration**: Point-and-click analysis without knowing metric names or data types.
- **Skill democratization**: Everyone from junior to senior engineers gets the same powerful tools.

### Efficient Storage

Netdata, contrary to most observability solutions, is optimized for lightweight storage operations. Three storage tiers are updated in parallel (per-second, per-minute, per-hour). The high-resolution tier needs 0.6 bytes per sample on disk (Gorilla compression + ZSTD). The lower resolution tiers need 6-bytes and 18-bytes per sample respectively and maintain the ability to provide the same min, max, average and anomaly rate the high-resolution tier provides. Data are written in append-only files and are never reorganized on disk (Write Once Read Many - WORM). Writes are spread evenly over time. Netdata Agents write at 5 KiB/s, Netdata Parents aggregating 1M metrics/s write at 1MiB/s across all tiers.

Netdata implements a custom time-series database optimized for the specific patterns of system metrics:

- **Write-once design**: Append-only architecture for maximum performance
- **Multi-tier storage**: Three storage tiers of different resolution, updated in parallel
- **Zero maintenance**: No recompaction or database maintenance windows

This efficient storage architecture delivers years of data in gigabytes rather than terabytes, with predictable I/O patterns and linear scaling of storage requirements with infrastructure size.

### Logs Management

:::info

Log management has become one of the largest cost drivers in observability, with organizations spending millions on storage and processing infrastructure. Many resort to aggressive filtering and sampling just to make costs manageable, inevitably losing critical information when they need it most.

:::

Netdata takes a fundamentally different approach by leveraging the systemd journal format, the native logs format on Linux systems. This edge-based approach provides enterprise-grade capabilities without the enterprise costs:

- **Direct file access**: No query servers needed — clients open journal files directly, leveraging OS disk cache for fast performance
- **Comprehensive indexing**: Every field in every log entry is automatically indexed, enabling instant queries across millions of entries
- **Flexible schema**: Each log entry can have its own unique set of fields and values, all fully indexed and searchable
- **Efficient storage**: Journal files typically match uncompressed text log sizes while providing full indexing — a balance between space efficiency and query performance
- **Native tooling**: Built-in support for centralization, filtering, exporting, and integration with existing pipelines
- **Security built-in**: Write Once Read Many (WORM) and Forward Secure Sealing (FSS) ensures log integrity and tamper detection
- **Logs transformation**: The platform includes `log2journal` for converting any text, JSON, or logfmt logs into structured journal entries

Where traditional solutions sample 5,000 log entries to generate field statistics on their dashboards, Netdata starts sampling at 1 million entries, providing 200x more accurate insights into log patterns. 
The result is enterprise-grade log management capabilities — field statistics, histogram breakdowns, full-text search, time-based filtering — all while keeping logs at the edge where they're generated, eliminating the massive costs of centralized log infrastructure.

:::note

On Windows Netdata queries Windows Event Logs (WEL), Event Tracing for Windows (ETW) and TraceLogging (TL) via the Event Log.

:::

### AI and Machine Learning

ML is the simplest way to model the behavior of our systems and applications. When done properly, ML can reliably detect anomalies, surface correlations between components and applications, provide valuable information about cascading effects under crisis, identify the blast radius of issues and even detect infrastructure level issues independently of the configured alerts.

Netdata democratizes ML and AI by making it automatic and universal (no configuration is required). The system trains 18 k-means models per metric using different time windows, requiring unanimous agreement before flagging anomalies. This achieves a false positive rate of 10^-36 (1% per model ^ 18 models) while remaining sensitive to real issues:

- **Continuous training**: Models train automatically as data arrives
- **Real-time detection**: Anomaly detection runs instantly, not in batches
- **Efficient storage**: Results store in just 1 bit per metric per second
- **Correlation analysis**: Engine identifies related anomalies across metrics
- **Unbiased detection**: Anomaly detection is not influenced by future events

Note: Netdata's ML focuses on detecting behavioral anomalies in metrics using their last 2 days of data. It is optimized for reliability rather than sensitivity and may miss slow (over days/weeks) infrastructure degradation or certain types of long-term anomalies (weekly, monthly, etc). However, it typically detects most types of abnormal behavior that break services.

For more information see [Netdata's ML Accuracy, Reliability and Sensitivity](https://github.com/netdata/netdata/blob/master/docs/ml-ai/ml-anomaly-detection/ml-anomaly-detection.md).

### Troubleshooting

Netdata introduces a significant shift to the troubleshooting process utilizing its unsupervised and real-time anomaly detection system. The "Anomaly Advisor" transforms troubleshooting:

- **Automatic scoring**: Ranks all metrics by anomaly severity within any time window
- **Root cause prioritization**: Surfaces the most likely culprits in the first 30-50 metrics
- **Sequence analysis**: Reveals the order of cascading failures across systems
- **Blast radius mapping**: Determines the full impact scope of incidents
- **AI-ready insights**: Provides structured data that AI assistants use to narrow investigations

This approach still requires interpretation skills but dramatically simplifies the investigation process compared to traditional methods (the aha! moment is within the first 30-50 results).

### Alerts

:::note

Most monitoring solutions focus on aggregate metrics and business-level alerts, often missing component failures until they cascade into service outages. This approach leads to alert fatigue from false positives and missed issues from incomplete coverage.

:::

Netdata takes a fundamentally different approach: templated alerts that monitor individual component and application instances. Each alert watches a single instance, building a comprehensive safety net where every component has its own watchdog. This granular approach ensures:

- **Complete coverage**: Every database, web server, container, and service instance has dedicated monitoring
- **Early detection**: Component failures are caught before they cascade into service-wide issues
- **Clear accountability**: Alerts identify exactly which instance is failing, not just that "something is wrong"
- **Scalable alerting**: Templates automatically apply to new instances as infrastructure grows
- **Synthetic checks**: Lightweight integration tests that validate connectivity and behavior between applications complements component monitoring

:::tip

Netdata ships with hundreds of pre-configured alerts, many intentionally silent by default. These silent alerts monitor important but non-critical conditions that should be reviewed but shouldn't wake engineers at 3am. This pragmatic approach balances comprehensive monitoring with operational sanity.

:::

### Scalability

For Netdata scalability is inherent to the architecture, not an add-on. Designed to be fully distributed, Netdata achieves linear scalability through:

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

1. **Install Netdata Agents** on all Linux, Windows, FreeBSD and MacOS physical servers and VMs
2. Optionally: dedicate resources (VMs, storage) for Netdata Parents, providing high-availability and longer retention to observability data
3. Optionally: configure logs transformation with `log2journal` and centralization using typical systemd-journald methodologies
4. **Configure collectors** that need credentials to access protected applications (databases, message brokers, etc), data collection for custom applications, enable SNMP discovery and data collection, install Netdata with auto-discovery in Kubernetes clusters
5. **Review alerts** (Netdata ships with preconfigured alerts) and set up alert **notification channels**
6. **Invite colleagues** (enterprise SSO via IODC, Okta and SCIMv2 supported), assign roles and permissions

Netdata will automatically provide:

1. **Complete coverage** of hardware, operating system and application metrics
2. Real-time, low-latency **Metrics and Logs Dashboards**
3. Live and interactive exploration of running **processes**, **network connections**, **systemd units**, **systemd services**, **IMPI sensors**, and more
4. Unsupervised **machine-learning based anomaly detection** for all metrics
5. Hundreds of **pre-configured alerts** for systems and applications
6. **AI insights** (reports) and **AI-assistant** (chat) connections via MCP

:::tip

Custom dashboards are supported but are optional. Netdata provides single-node, multi-node and **infrastructure level dashboards** automatically.

:::

Netdata configurations are infrastructure-as-code friendly, and provisioning systems can be used to automate deployment on large infrastructures.
A complete Netdata deployment is usually achieved within a few days.

## Resource Requirements

Netdata is committed to having best-in-class resource utilization. Wasted resources are considered bugs and are addressed with high priority.

Based on extensive real-world deployments and independent academic validation, Netdata maintains minimal resource footprint:

| Resource               | Standalone 5k metrics/s | Child 5k metrics/s  | Parent 1M metrics/s |
|------------------------|:-----------------------:|:-------------------:|:-------------------:|
| **CPU**                | 5% of a single core     | 3% of a single core | ~10 cores total     |
| **Memory**             | 200 MB                  | 150 MB              | ~40 GB              |
| **Network**            | None                    | \<1 Mbps to Parent   | ~100 Mbps inbound   |
| **Storage Capacity**   | 3 GiB (configurable)    | None                | as needed           |
| **Storage Throughput** | 5 KiB/s write           | None                | 1 MiB/s write       |
| **Retention**          | 1 year (configurable)   | None                | as needed           |

:::note

- Parent resources include both ingestion and query workload
- Storage rates are for all tiers combined; actual disk usage depends on retention configuration
- The recommended topology is having a cluster of Netdata Parents every 500 monitored nodes (2M metrics/s)

:::

:::info

The [University of Amsterdam study](https://twitter.com/IMalavolta/status/1734208439096676680) found Netdata to be the most energy-efficient monitoring solution, with the lowest CPU overhead, memory usage, and execution time impact among compared tools.

For more information see [Netdata's impact on resources](https://github.com/netdata/netdata/blob/master/docs/netdata-agent/sizing-netdata-agents/README.md).

:::

## Practical Implications

Please also see [Netdata Enterprise Evaluation Guide](https://github.com/netdata/netdata/blob/master/docs/netdata-enterprise-evaluation-corrected.md) and [Netdata's Security and Privacy Design](https://github.com/netdata/netdata/blob/master/docs/security-and-privacy-design/README.md).

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

## Frequently Asked Questions on Design Philosophy

<details>
<summary><strong>Doesn't edge architecture create a management nightmare?</strong></summary><br/>

**The opposite is true — edge architecture eliminates most management overhead.**

Traditional centralized systems require:
- Database administration (backups, compaction, tuning)
- Capacity planning for central storage
- Pipeline management and scaling
- Schema migration coordination
- Downtime windows for maintenance

Netdata's edge approach provides:
- **Zero maintenance**: Agents and Parents run autonomously without administration
- **Automatic updates**: Built-in update mechanisms or integration with provisioning tools
- **Strong compatibility**: Backwards compatibility ensures upgrades don't break things
- **Forward compatibility**: Can often downgrade without data loss (we implement forward-read capability before activating new schemas)
- **No coordination needed**: Each agent updates independently — no "big bang" migrations
- **Fixed relationships**: Parent-Child connections are one-time configuration with alerts for disconnections
- **Cardinality protection**: Automated protections prevent runaway metrics from affecting the entire system
- **Built-in high availability**: Streaming and replication provide data redundancy without complex setup

The architecture also delivers operational benefits:
- **Minimal disk I/O**: Data commits only every 17 minutes per metric (spread over time), while real-time streaming maintains data safety
- **No backup complexity**: Observability data is ephemeral (rotated) and write-once-read-many (WORM), eliminating traditional backup requirements via replication
- **Isolated failures**: Issues affect only parts of the ecosystem (e.g., a single parent), not the entire monitoring foundation

**Why not use existing databases?**

Existing time-series databases couldn't meet the requirements for edge deployment:
- **Process independence**: No separate database processes to manage
- **Write-once-read-many (WORM)**: Corruption-resistant with graceful degradation
- **Zero maintenance**: No tuning, compaction, or optimization required
- **Minimal footprint**: Small memory usage with extreme compression (0.6 bytes/sample)
- **Optimized I/O**: Low disk writes spread over time to minimize impact
- **Embedded ML**: Anomaly detection without additional storage overhead
- **Partial resilience**: Continues operating even with partial disk corruption

The "thousands of databases" concern misunderstands the architecture. These aren't databases you manage — they're autonomous components that manage themselves. It's like worrying about managing thousands of log files when you use syslog — the system handles it.

In practice, organizations using Netdata routinely achieve multi-million samples/second, highly-available observability infrastructure without even noticing the complexity this would normally imply. The complexity isn't moved — it's eliminated through design.
</details>

<details>
<summary><strong>Isn't collecting 'everything' fundamentally wasteful?</strong></summary><br/>

**The opposite is true — Netdata is the most energy-efficient monitoring solution available.**

The University of Amsterdam study confirmed Netdata uses significantly fewer resources than selective monitoring solutions. Despite collecting everything and per-second, our optimized design and streamlined code make Netdata more efficient, not less.

The real question is: **What's the business impact when critical troubleshooting data isn't available during a crisis?**

Consider:
- **Crisis happens when things break unexpectedly** — if they were expected, you'd have mitigations in place
- **The very fact systems are in crisis** means the failure mode wasn't predicted
- **Engineers can't predict what data they'll need** for problems they didn't anticipate

The business case for complete coverage:
- **Reduced MTTD/MTTR**: All data is available immediately when investigating issues
- **No blind spots**: The metric you didn't think to collect often holds the key
- **ML/AI effectiveness**: Algorithms can find correlations in "insignificant" metrics that humans miss
- **Lower environmental impact**: More efficient than selective solutions despite broader coverage

Selective monitoring creates a paradox: you must predict what will break to know what to monitor, but if you could predict it, you'd prevent it. Complete coverage eliminates this guessing game while actually reducing resource consumption through better engineering.
</details>

<details>
<summary><strong>Does complete coverage create analysis paralysis?</strong></summary><br/>

**Structure prevents paralysis — Netdata organizes data hierarchically, not as an unstructured pool.**

Unlike monitoring solutions that present metrics as a flat list, Netdata uses intelligent hierarchical organization:
- 50 disk metrics stay within the disk section
- 100 container metrics remain in the container view
- Database metrics don't interfere with network analysis

This means:
- **No performance impact**: Finding database issues isn't slower because you have more network metrics
- **No confusion**: Each subsystem's metrics are logically grouped and accessible
- **Negligible cost**: One more metric adds just 18KB memory and 0.6 bytes/sample on disk

**The real insight: Comprehensive data empowers different engineering approaches**

Some engineers thrive with complete visibility — they can trace issues across subsystems, understand cascading failures, and prevent future problems. Others prefer simpler "is it working?" dashboards. Netdata supports both:

- **For troubleshooters**: Full depth to understand root causes and prevent recurrence
- **For quick fixes**: High-level dashboards and clear alerts for immediate action
- **For everyone**: ML-driven Anomaly Advisor surfaces what matters without manual searching

The philosophy isn't "more data is better" — it's "the right data should always be available." Hierarchical organization ensures engineers can work at their preferred depth without being overwhelmed by information they don't currently need.

Organizations report that engineers who initially felt overwhelmed quickly adapt once they experience finding that one critical metric that solved a major incident — the metric they wouldn't have thought to collect in advance.
</details>

<details>
<summary><strong>Is per-second granularity actually useful or just marketing?</strong></summary><br/>

**Per-second is for engineers, not business metrics — it matches the speed at which systems actually operate.**

Consider the reality of modern systems:
- CPUs execute **billions of instructions per second**
- A single second contains enough time for entire cascading failures
- In one minute, a system can process millions of requests, experience multiple garbage collections, or suffer intermittent network issues

**Per-second is the standard for engineering tools**

When engineers debug with console tools, they never use 10-second or minute averages. Why? Because averaging hides critical details:
- Stress spikes that trigger failures
- Micro-bursts that overwhelm queues
- Brief stalls that compound into user-facing latency

**Netdata was designed as a unified console replacement**

Think of Netdata as the evolution of `top`, `iostat`, `netstat`, and hundreds of other console tools — but with:
- The same per-second granularity engineers expect
- Complete coverage across all subsystems
- Historical data to trace issues backward
- Visual representation of complex relationships
- Machine Learning analyzing everything

This is true tools consolidation: instead of jumping between dozens of console commands during an incident, engineers have one unified view at the resolution that matters. When a service degrades, you need to see the exact second it started, not a minute-average that obscures the trigger.

**Immediate feedback is crucial for effective operations**

When engineers make infrastructure changes, they need to see the impact immediately:
- **During crisis**: Every second counts — you can't wait for minute-averages to confirm if your fix is working
- **Configuration changes**: See instantly whether that parameter helped or made things worse
- **Scaling operations**: Watch resource utilization respond in real-time as you add capacity
- **Performance tuning**: Observe the immediate effect of cache size adjustments or thread pool changes

This instant feedback loop dramatically accelerates problem resolution. Engineers can rapidly iterate through potential fixes, seeing results within seconds rather than waiting for averaged data that might hide whether the intervention actually helped.

For business metrics, minute or hourly aggregations make sense. But for infrastructure monitoring and tuning, per-second granularity is the foundation of effective troubleshooting.
</details>

<details>
<summary><strong>What about the observer effect? How do you guarantee per-second collection isn't impacting application performance?</strong></summary><br/>

**Netdata's default collection frequencies are carefully configured to avoid impacting monitored applications.**

The goal is simple: collect all metrics at the maximum possible frequency without affecting performance. This means:

**Thoughtfully configured defaults:**
- **Most metrics**: Collected per-second when source data updates frequently
- **Slower metrics**: Collected every 5-10 seconds when source data changes less frequently  
- **Expensive metrics**: Disabled by default with optional configuration flags for specialized use cases

**Performance-first defaults:**
- Collection frequency is tuned based on the cost of data gathering
- "Expensive" metrics (those affecting performance) have lower default frequencies
- Some specialized metrics are completely disabled by default but can be enabled when their value justifies the overhead

**User control:**
- All frequencies are configurable — users can increase collection frequency if they need higher resolution for specific metrics
- Can disable any collector that proves problematic in their specific environment
- Can enable expensive collectors when their specialized value outweighs the performance cost

This isn't about blindly collecting everything every second regardless of impact. It's about being intelligent enough to collect each metric at the optimal frequency for that specific data source and use case, defaulting to configurations that have been proven safe across thousands of production deployments.

The University of Amsterdam study confirmed this approach works: despite comprehensive collection, Netdata has the lowest performance impact on the monitored applications among monitoring solutions.
</details>

<details>
<summary><strong>Why systemd-journal instead of industry standards like Elasticsearch/Splunk?</strong></summary><br/>

**systemd-journal IS the industry standard — it's already installed and running on every Linux system.**

The question misframes the choice. systemd-journal isn't competing with Elasticsearch/Splunk — it's the native log format they all read from. The real question is: why move data when you can query it directly?

**Understanding the trade-offs:**

| Approach | Storage Footprint | Query Performance | Indexing Strategy |
|----------|------------------|-------------------|-------------------|
| **Loki** | 1/4 to 1/2 of original logs | Slow (brute force scan after metadata filtering) | Limited metadata indexing |
| **Elasticsearch/Splunk** | 2-5x larger than original logs | Fast full-text search | Word-level reverse indexing |
| **systemd-journal** | ~Equal to original logs | Fast field-value queries | Forward indexing of all field values |

**systemd-journal provides a balanced approach:**
- **Open schema**: Each log entry can have unique fields, all automatically indexed
- **Storage efficient**: Roughly same size as original logs
- **Query optimized**: Fast lookups for any field value as a whole
- **Universal compatibility**: Already the source for all other log systems

**But systemd-journal is actually superior in critical ways:**
- **Security features**: Forward Secure Sealing (FSS) for tamper detection — not even available in most commercial solutions
- **Native access control**: Uses filesystem permissions for isolation — no additional security layer to breach
- **Extreme performance**: Outperforms everything else in single-node ingestion throughput while being lightweight
- **No query server**: All queries run in parallel, lockless, directly on files — infinitely scalable read performance
- **OS-level optimization**: Naturally cached by the kernel, providing blazing-fast repeated queries
- **Built-in distribution**: Native tools for log centralization within infrastructure, no additional software needed
- **Edge-native**: Distributed by design, perfectly aligned with Netdata's architecture

Furthermore, direct file access isn't a security risk — it's a security advantage. Access control is enforced by the operating system itself through native filesystem permissions. There's no query server to hack, no additional authentication layer to misconfigure, and no database permissions to manage. Multi-tenancy and log isolation work through the same filesystem permission model that has provided reliable security for decades.

**What Netdata adds:**
systemd-journal is powerful but lacks the visualization and analysis layer. Netdata provides:
- Rich query interface and dashboards
- Field statistics and histograms
- Integration with metrics and anomaly detection
- Web-based exploration tools

The insight: instead of copying logs to expensive centralized systems, why not build better tools on the robust foundation already present in every Linux system? This eliminates data movement, reduces infrastructure costs, provides superior security, and delivers faster queries through native file access — all while maintaining the distributed architecture that makes modern infrastructure manageable.
</details>

## Summary

Netdata represents a fundamental rethink of monitoring architecture. By processing data at the edge, automating configuration, maintaining real-time resolution, applying ML universally, and making data accessible to everyone, it solves core monitoring challenges that have persisted for decades.

The result is a monitoring system that deploys in minutes instead of months, scales efficiently to any size, adapts automatically to changes, and provides insights that would be impossible with traditional approaches — all while remaining open source and community driven.

Whether you're monitoring a single server or a global infrastructure, Netdata's design philosophy creates a monitoring system that works with you rather than demanding constant attention.
