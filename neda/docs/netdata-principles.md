# Netdata Principles: Engineering Excellence for Operational Simplicity

## Preamble

These principles guide every technical and product decision at Netdata. They represent our commitment to the world's operations teams, DevOps engineers, SREs, and system administrators who manage increasingly complex infrastructures with lean teams and limited resources.

**Our Core Philosophy**: We invest engineering work to eliminate complexity, not manage it. Netdata profits by providing hard engineering work to simplify operations—not by charging for data movement, storage, or processing volume. Our goal is to reveal and surface the pulse and breath of systems and applications, making infrastructure health visible, understandable, and actionable without requiring deep expertise or specialized skills.

---

## The Netdata Principles

The following are provided for your understanding, as hints to capture Netdata's philosophy.
CRITICAL: You MUST NOT use the exact wording on every page - we don't want every page to look the same. Focus on the subject of the page.

### 1. **Your Infrastructure, Your Data, Your Control**

**The Principle**: Observability data belongs at the edge where it's generated, not in centralized databases controlled by vendors. Organizations must maintain complete sovereignty over their monitoring data.

**Why This Matters to You**:
- **Compliance by design**: GDPR, HIPAA, PCI DSS, and regional data residency requirements are satisfied automatically because your metrics never leave your infrastructure
- **No vendor lock-in**: Your data stays under your control, eliminating dependency on external services
- **Predictable costs**: No surprise bills from data egress charges or volume-based pricing
- **Privacy protection**: Sensitive performance data remains within your security perimeter
- **Operational resilience**: Monitoring continues working even during internet outages or cloud provider issues

**How Netdata Delivers**:
- Distributed architecture keeps all observability data local
- Only metadata (hostnames, chart titles) travels to Netdata Cloud for unified dashboards
- Machine learning models train locally without external data transmission
- On-premises deployment options for complete air-gapped environments
- You choose where data lives: standalone agents, local parents, or your own infrastructure

**The Innovation**: While others centralize your data and charge you for it, we distribute intelligence to your systems and let you keep what's yours.

---

### 2. **Real-Time Means Per-Second, Not "Near Real-Time"**

**The Principle**: True real-time monitoring requires per-second data collection with sub-2-second total latency from event to insight. Anything slower is statistical analysis, not operational monitoring.

**Why This Matters to You**:
- **Catch problems before they cascade**: A 3-second CPU spike can trigger a 30-second outage if not detected immediately
- **See what actually happened**: A query using 100% CPU for 5 seconds every 20 seconds appears as harmless 25% average load in minute-based monitoring—but Netdata shows the damaging 100% spikes
- **Faster troubleshooting**: Engineers see the immediate effects of their changes, enabling rapid iteration during incidents
- **Security threat detection**: Modern attacks happen in seconds—port scans (2-3s), crypto-miners (instant CPU spikes), memory scanning (burst patterns), data exfiltration attempts (unusual outbound connections lasting 4-8 seconds)—invisible to 30-second monitoring
- **Accurate autoscaling**: Cloud autoscalers with 30-second visibility suffer from over-provisioning, under-provisioning, and flapping—per-second data enables informed decisions

**How Netdata Delivers**:
- 1-second data collection across all 800+ integrations
- 1-second visualization latency from collection to display
- Sub-2-second worst-case total latency (data + analysis + action)
- No sampling, no averaging, no statistical approximations
- Gaps in charts reveal system stress (not network issues)—if Netdata can't collect, your applications can't serve

**The Innovation**: We proved that sustained per-second monitoring at scale is achievable and essential. Independent testing shows Netdata uses 37% less CPU, 88% less RAM, and 97% less disk I/O than traditional solutions while providing 40x longer retention and 22x faster queries.

---

### 3. **Zero Configuration, Instant Value**

**The Principle**: Monitoring should work instantly without configuration, manual dashboard building, learning query languages, or tuning thresholds. The time from installation to actionable insight should be measured in seconds, not weeks.

**Why This Matters to You**:
- **Eliminate setup time**: No weeks/months spent configuring collectors, building dashboards, or learning PromQL
- **Reduce operational burden**: Infrastructure changes don't require monitoring updates—new containers appear automatically, deleted resources vanish cleanly
- **Democratize observability**: Junior engineers get the same powerful tools as senior SREs without specialized training
- **Focus on problems, not tools**: Spend time fixing issues, not maintaining monitoring infrastructure
- **Accelerate incident response**: During crises, you need answers immediately—not time spent building queries or dashboards

**How Netdata Delivers**:
- **Auto-discovery**: Automatically detects systems, containers, applications, and services (800+ integrations)
- **Algorithmic dashboards**: Charts generate automatically based on data's semantic meaning—not manual configuration
- **Universal navigation**: Same logical structure across all infrastructures—learn once, use everywhere
- **Point-and-click analysis**: Slice and dice any dataset without knowing metric names or query languages
- **Pre-configured alerts**: 400+ health checks work out-of-the-box with ML-based anomaly detection requiring zero tuning

**The Innovation**: Dashboards are an algorithm, not a configuration. Each chart is a complete analytical tool equivalent to 20+ Grafana charts, providing 360° views with simple point-and-click.

---

### 4. **Intelligence at the Edge, Not in the Cloud**

**The Principle**: Processing, analysis, and machine learning should happen where data is generated—on the monitored node—not in centralized cloud services. Edge intelligence eliminates bandwidth costs, reduces latency, enables offline operation, and scales linearly.

**Why This Matters to You**:
- **Linear scalability**: Adding 100 nodes costs and performs the same as the first 100 nodes—no architectural rewrites as you grow
- **No central bottlenecks**: Each agent operates independently; parent failures don't affect agent operation
- **Faster insights**: No round-trip to cloud for analysis—anomaly detection happens during data collection
- **Lower costs**: No bandwidth charges for streaming terabytes of metrics to centralized databases
- **Resilient operation**: Monitoring continues working during network partitions or parent downtime
- **Privacy by design**: Sensitive data never leaves your infrastructure for ML processing

**How Netdata Delivers**:
- **Complete observability engine per agent**: Each agent collects, stores, analyzes, detects anomalies, checks alerts, and serves dashboards independently
- **Edge-based machine learning**: 18 unsupervised k-means models per metric train locally, achieving 99% false positive reduction through consensus
- **Distributed correlation**: Real-time anomaly correlation across all metrics powers root cause analysis
- **Parent-child streaming**: Lightweight children stream to powerful parents with automatic failover and data replication
- **Horizontal scaling**: Deploy more parents to handle more agents—no complex sharding or rebalancing

**The Innovation**: Each Netdata Agent is a full monitoring system, not just a data forwarder. This distributed architecture processes 4.5+ billion metrics/second globally while maintaining sub-2-second latency.

---

### 5. **Monitor Everything, Miss Nothing**

**The Principle**: Organizations must be free to collect every metric from every system without artificial limits, sampling, or cost barriers. You cannot predict which "insignificant" metric will hold the key to solving tomorrow's crisis.

**Why This Matters to You**:
- **Eliminate blind spots**: The metric you didn't think to collect often holds the answer during incidents
- **Resolve the prediction paradox**: If you could predict what will break, you'd prevent it—complete coverage eliminates guessing
- **Enable ML effectiveness**: Algorithms find correlations across "unrelated" metrics that humans miss
- **Reduce MTTD/MTTR**: All data is available immediately during investigations—no "we weren't collecting that" moments
- **Support skill independence**: Junior engineers get the same comprehensive visibility as senior staff

**How Netdata Delivers**:
- **800+ integrations**: Systems, containers, VMs, hardware sensors, databases, web servers, message brokers, cloud services
- **Universal protocols**: OpenTelemetry, OpenMetrics, StatsD, logs, synthetic checks, SNMP
- **Automatic privileges**: Uses Linux capabilities and secure elevation for comprehensive system access
- **Hierarchical organization**: 50 disk metrics stay in disk section, 100 container metrics in container view—no analysis paralysis
- **Unlimited cardinality**: High-cardinality data (ephemeral containers, microservices) handled without performance degradation

**The Innovation**: We proved comprehensive monitoring is more efficient than selective monitoring. University of Amsterdam study confirmed Netdata is the most energy-efficient monitoring solution despite collecting everything, every second.

---

### 6. **Efficiency Through Engineering, Not Through Compromise**

**The Principle**: Resource efficiency should come from superior engineering—optimized algorithms, efficient storage, intelligent processing—not from sampling data, reducing granularity, or limiting coverage.

**Why This Matters to You**:
- **Lower infrastructure costs**: Minimal CPU (<5%), RAM (150MB), and disk I/O per agent
- **Sustainable at scale**: 500 nodes with 2M metrics/second requires only 20 cores and 80GB RAM for parent cluster
- **No performance tax**: Monitoring doesn't compete with your applications for resources
- **Environmental responsibility**: Most energy-efficient monitoring solution (peer-reviewed validation)
- **Predictable resource usage**: Stable consumption patterns without sudden spikes

**How Netdata Delivers**:
- **Industry-leading compression**: 0.6 bytes per sample (Gorilla + ZSTD) enables years of retention in gigabytes
- **Write-once storage**: Append-only files with no compaction or maintenance windows
- **Multi-tier architecture**: Three resolution tiers (per-second, per-minute, per-hour) updated in parallel
- **Spread workload**: Data commits every 17 minutes, spread evenly over time to avoid I/O spikes
- **Efficient ML**: Unsupervised k-means runs as low-priority background tasks, yielding to data collection

**The Innovation**: 37% less CPU, 88% less RAM, 13% less bandwidth, and 97% less disk I/O than traditional solutions—while providing 40x longer retention and 22x faster queries.

---

### 7. **Transparent Economics: Nodes, Not Data Volume**

**The Principle**: Pricing should be predictable and based on infrastructure size (nodes), not data volume, cardinality, or feature access. Organizations should never face the choice between visibility and budget.

**Why This Matters to You**:
- **No surprise bills**: Costs scale linearly with infrastructure, not with data volume spikes
- **Collect without anxiety**: No penalties for high-resolution metrics, custom metrics, or deep cardinality
- **Budget predictability**: Simple per-node pricing enables accurate financial planning
- **No feature gating**: ML, AI, alerting, and dashboards included—not premium add-ons
- **Align incentives**: We profit by making monitoring better, not by charging more for your data

**How Netdata Delivers**:
- **Flat per-node pricing**: $6/node/month base price with volume discounts (compare to commercial platforms charging $15-30/node/month PLUS per-metric fees)
- **Unlimited metrics**: No limits on custom metrics, dimensions, or cardinality
- **Unlimited users**: Team collaboration without per-seat charges
- **All features included**: ML anomaly detection, AI insights, alerting, dashboards—no premium tiers
- **Homelab pricing**: $90/year unlimited nodes for non-commercial use

**The Innovation**: While others charge based on data volume (forcing you to choose between visibility and cost), we charge based on infrastructure size—aligning our success with your operational needs.

---

### 8. **Open Standards, No Lock-In**

**The Principle**: Observability platforms should embrace open standards and enable interoperability, not create vendor lock-in through proprietary formats, custom query languages, or closed ecosystems.

**Why This Matters to You**:
- **Freedom to choose**: Integrate with existing tools or switch solutions without data migration nightmares
- **Protect investments**: Existing instrumentation (Prometheus, OpenMetrics, StatsD) works without changes
- **Avoid dependency**: No proprietary query languages or data formats that trap you
- **Community innovation**: Open-source foundation enables customization and community contributions
- **Future-proof architecture**: Standards-based approach adapts to evolving technology landscape

**How Netdata Delivers**:
- **Open-source core**: GPLv3+ licensed agent with 76,000+ GitHub stars and 1.5M daily downloads
- **Standards support**: Ingests Prometheus, OpenMetrics, StatsD, OpenTelemetry (Q3 2025)
- **Export flexibility**: Exports to Prometheus, InfluxDB, Graphite, OpenTSDB, TimescaleDB
- **Grafana integration**: Native datasource plugin for existing workflows
- **API access**: REST APIs for custom integrations and automation
- **Logs interoperability**: Uses systemd-journal (Linux) and Windows Event Log—no proprietary formats

**The Innovation**: We built the most advanced monitoring platform on open standards and open-source foundations, proving you don't need vendor lock-in to deliver exceptional value.

---

### 9. **Troubleshooting First, Dashboards Second**

**The Principle**: Monitoring must be a real-time watchdog that enables rapid troubleshooting—not a historian that generates reports after incidents. The system should surface problems immediately with enough context to fix them.

**Why This Matters to You**:
- **Proactive detection**: Learn about problems from Netdata before customers complain
- **Faster resolution**: 80% reduction in MTTR through immediate visibility and ML-powered root cause analysis
- **Console replacement**: Debug without SSH—all console tools (top, iostat, netstat) unified in browser with history
- **Eliminate alert fatigue**: ML consensus (18 models) reduces false positives by 99%
- **Contextual insights**: Anomaly Advisor surfaces the 30-50 metrics that matter during incidents

**How Netdata Delivers**:
- **Real-time anomaly detection**: ML runs during data collection, not in batch jobs
- **Correlation engine**: Automatically identifies related anomalies across metrics
- **Blast radius detection**: Visualizes impact spread in real-time
- **Cascading failure analysis**: Shows exact sequence of which domino fell first and why
- **AI troubleshooting**: One-click "Ask AI" on every alert for instant context and recommended actions
- **Live system state**: Processes, network connections, systemd units—all with history and ML

**The Innovation**: We transformed monitoring from "what happened?" to "what's wrong and how do I fix it?"—with ML and AI providing answers in seconds, not hours.

---

### 10. **Logs Without Pipelines, Queries Without Servers**

**The Principle**: Log management should leverage native formats (systemd-journal, Windows Event Log) for efficient, comprehensive log analysis at the edge—eliminating expensive centralized pipelines and query servers.

**Why This Matters to You**:
- **90% cost reduction**: Eliminate Elasticsearch/Splunk infrastructure, ingestion fees, and storage multiplication
- **No data loss**: Keep all logs without sampling or filtering to manage costs
- **200x more accurate**: Analyzes 1M entries before sampling vs 5K for traditional tools
- **Instant correlation**: Logs and metrics from same source—no timestamp matching or separate systems
- **Compliance built-in**: Forward Secure Sealing (FSS) for tamper detection, data stays at edge for GDPR/sovereignty

**How Netdata Delivers**:
- **Direct file access**: Clients open journal files directly—no query servers needed
- **Comprehensive indexing**: Every field in every log entry automatically indexed by journald
- **Flexible schema**: Each entry can have unique fields, all fully searchable
- **Native tooling**: Built-in centralization, filtering, exporting via systemd-journal-upload
- **Log transformation**: `log2journal` converts text/JSON/logfmt into structured, indexed entries
- **Windows support**: Full Windows Event Logs, ETW, and TraceLogging—unified logging across platforms

**The Innovation**: Instead of copying logs to expensive centralized systems, we built better tools on the robust foundation already present in every system—eliminating data movement while delivering superior capabilities.

---

### 11. **AI That Understands Your Infrastructure**

**The Principle**: AI and machine learning should be embedded throughout the platform—not bolted on as afterthoughts—providing contextual insights based on your actual infrastructure behavior, not generic advice.

**Why This Matters to You**:
- **Democratize expertise**: Junior engineers operate at senior-level effectiveness with AI guidance
- **Accelerate resolution**: AI explains alerts in plain language with recommended actions
- **Reduce cognitive load**: AI Insights generates professional reports in 2-3 minutes instead of hours of manual analysis
- **Natural language queries**: Ask questions about your infrastructure in your language—no query languages required
- **Continuous learning**: ML models adapt to your infrastructure's unique patterns automatically

**How Netdata Delivers**:
- **Embedded ML**: 18 unsupervised models per metric train automatically from day one
- **Anomaly Advisor**: Cuts through thousands of metrics to show the 10 that matter right now
- **AI Insights reports**: Infrastructure summary, capacity planning, performance optimization, anomaly analysis—all automated
- **AI Chat via MCP**: Ask questions using Claude, ChatGPT, Gemini, or other LLMs
- **Alert AI Assistant**: One-click explanations for any alert with context and recommendations
- **Root cause analysis**: Correlation engine identifies cascading failures and blast radius automatically

**The Innovation**: We embedded AI throughout the platform—from data collection (ML anomaly detection) to troubleshooting (AI Chat) to reporting (AI Insights)—making every engineer more effective regardless of experience level.

---

### 12. **Production-Ready Security by Design**

**The Principle**: Security cannot be an afterthought. TLS by default, zero-trust networking, encrypted communication, data sovereignty, RBAC, SSO, and audit logs must be built-in from day one.

**Why This Matters to You**:
- **Compliance confidence**: SOC 2 Type 1 certified, GDPR/HIPAA/PCI DSS aligned by design
- **Reduced risk**: Security best practices embedded in architecture, not optional add-ons
- **Audit readiness**: Comprehensive audit logs and access controls satisfy regulatory requirements
- **Enterprise authentication**: SSO via LDAP/AD, Okta, OIDC with automatic role mapping via SCIM
- **Data protection**: Metrics never leave infrastructure; only metadata travels to Cloud

**How Netdata Delivers**:
- **TLS everywhere**: Encrypted agent-cloud communication by default
- **Zero-trust architecture**: No implicit trust between components
- **RBAC**: Role-based access control with granular permissions
- **SSO integration**: Enterprise authentication with LDAP/AD group mapping
- **Audit logging**: Comprehensive activity tracking for compliance
- **Data sovereignty**: All observability data stays on-premises under your control
- **OSSF best practices**: Follows Open Source Security Foundation guidelines

**The Innovation**: We built security into the foundation—not as premium features—ensuring every deployment meets enterprise security standards from day one.

---

## How These Principles Work Together

These principles are not isolated features—they form a cohesive philosophy that transforms observability:

1. **Edge architecture** (Principles 1, 4) enables **data sovereignty** and **linear scalability**
2. **Per-second granularity** (Principle 2) makes **real-time troubleshooting** (Principle 9) possible
3. **Zero configuration** (Principle 3) and **comprehensive collection** (Principle 5) eliminate the **prediction paradox**
4. **Efficiency through engineering** (Principle 6) enables **monitoring everything** without compromise
5. **Transparent economics** (Principle 7) aligns our incentives with your operational needs
6. **Open standards** (Principle 8) protect your investment and prevent lock-in
7. **Logs without pipelines** (Principle 10) extends edge efficiency to log management
8. **Embedded AI** (Principle 11) democratizes expertise and accelerates resolution
9. **Security by design** (Principle 12) ensures enterprise readiness from day one

**The Result**: Observability that is faster, simpler, more comprehensive, more efficient, and more cost-effective than traditional approaches—while putting you in complete control.

---

## Our Commitment to You

These principles represent our commitment to the world's operations teams:

- **We invest engineering effort** to eliminate complexity, not manage it
- **We profit by making monitoring better**, not by charging more for your data
- **We respect your data sovereignty** and keep metrics under your control
- **We democratize observability** so teams of any size can manage complex infrastructures
- **We innovate relentlessly** to break industry rules and deliver significantly better results

When you choose Netdata, you're not just selecting a monitoring tool—you're partnering with a team committed to making infrastructure management fundamentally simpler, more effective, and more accessible.

**Welcome to the future of observability. Welcome to Netdata.**

---

## Validation

These principles are not aspirational—they're validated by:

- **76,000+ GitHub stars** and **1.5M daily downloads**
- **Independent research**: University of Amsterdam peer-reviewed study confirming Netdata as most energy-efficient monitoring solution
- **Performance testing**: 37% less CPU, 88% less RAM, 97% less disk I/O vs. traditional solutions
- **Real-world deployments**: 100,000+ node installations processing 4.5+ billion metrics/second globally
- **Customer outcomes**: 80% MTTR reduction, 90% cost savings, immediate productivity gains

---

*These principles guide every decision at Netdata. When we face technical choices, product decisions, or business tradeoffs, we ask: "Does this serve our users' operational needs? Does this simplify their work? Does this respect their data and their control?" If the answer is no, we don't do it.* 
