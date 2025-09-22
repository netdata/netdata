# Scalability: Monitoring at Any Scale

## TL;DR

Netdata scales from a single node to 100,000+ nodes without architectural changes, maintaining 1-second granularity and sub-2-second latency at any scale. The distributed edge-native architecture ensures that adding more nodes doesn't degrade performance - each node operates independently while collaborating seamlessly.

## The Problem with Centralization

Traditional observability assumes one thing: push all data to a central database, then query it for dashboards and alerts.

This works. Until it doesn't.

When scale breaks the model, teams face two options - and both are wrong:

### Option 1: Reduce the Workload

Lower granularity. Drop cardinality. Filter. Sample.

**This is a trap and a paradox.** If you knew which data you'd need during a crisis, you could predict the crisis and prevent it. By definition, an unpredictable event is the one that will be invisible in your downsampled dataset. You're betting your incident response on being able to predict the unpredictable.

### Option 2: Scale the Database

Build giant, expensive clusters. Add more cores. More RAM. More everything.

**This is a money pit.** In many organizations today, observability costs more than the services being monitored. We routinely encounter companies where 40-50% of infrastructure budget goes to monitoring pipelines, plus teams of specialists to keep them alive.

## The Netdata Way: Process and Store at the Edge

**Instead of centralizing data, distribute the code.** This is the heart of Netdata's philosophy and design.

Every Netdata Agent is a full observability engine:

- Collects metrics at the edge
- Stores data locally in multi-tier storage
- Runs ML-based anomaly detection in real-time
- Runs health checks and triggers alerts
- Serves dashboards and APIs independently

When you need high availability, persistent storage for ephemeral nodes, reduced load on production systems, or on-premises dashboards, Parents aggregate streams without becoming bottlenecks - because the heavy lifting already happened at the edge.

This distributed architecture delivers results that speak for themselves:

- **No loss of fidelity** - every metric, every second, always visible
- **No blind spots** - no sampling, no cherry-picking
- **No scaling tax** - adding nodes adds observability, not exponential cost curves

Once you pass ~500 nodes, you’re naturally in the multi-million metrics/s range. What looks “heroic” elsewhere is simply normal operating conditions with Netdata.

## Proof: The Numbers Don't Lie

### Independent Validation: University of Amsterdam Study (2023)

**Study:** "An Empirical Evaluation of the Energy and Performance Overhead of Monitoring Tools on Docker-Based Systems"<br/>
**Conference:** ICSOC 2023 (International Conference on Service-Oriented Computing)<br/>
**DOI:** 10.1007/978-3-031-48421-6_13

**Finding:** **Netdata is the most energy-efficient monitoring solution**, with the lowest CPU and memory overhead - even while collecting data every second and running anomaly detection at the edge.

### Head-to-Head: Netdata vs Prometheus (2025)

We tested a single installation Netdata Parent and Prometheus, at **4.6 million metrics per second** - the scale you hit with just 1,000 nodes. This is how the systems compare for ingestion:

| Metric                          | Netdata           | Prometheus       | Impact                                       |
|---------------------------------|-------------------|------------------|----------------------------------------------|
| **CPU Usage**                   | ~9.4 cores        | ~14.8 cores      | 36% less CPU                                 |
| **Memory Usage**                | ~47 GiB           | ~383 GiB         | 88% less RAM                                 |
| **Disk I/O**                    | ~4.7 MiB/s writes | ~147 MiB/s total | 97% less I/O                                 |
| **Per-second retention (1TiB)** | ~1.25 days        | ~2 hours         | 15x longer - 40x retention in lower tiers    |
| **Sample completeness**         | ~100%             | ~93.7%           | Zero data loss                               |
| **Query latency (2hr window)**  | ~0.11s            | ~1.8s            | 16x faster - 22x faster in long term queries |

**Critical insight:** This isn't exotic scale. **Every Netdata deployment with >500 nodes runs at millions of metrics per second.** Our users don't even notice - because the architecture absorbs it.

## Architecture: Built for Planet Scale

### Core Components

| Component  | Role                              | Resources (Standalone)                  | Resources (Offloaded)                  | Scale Factor             |
|------------|-----------------------------------|-----------------------------------------|----------------------------------------|--------------------------|
| **Agent**  | Edge collector                    | &lt;5% CPU, &lt;200 MiB RAM, disk I/O   | &lt;2% CPU, &lt;150 MiB RAM, zero disk | 3,000-20,000 metrics/sec |
| **Parent** | Workload distributor & aggregator | 10 cores, 40 GiB RAM per 1M metrics/sec | Same + ML training if enabled          | Linear scaling           |
| **Cloud**  | Control plane & federation        | Minimal                                 | Minimal                                | Unlimited Parents        |

:::note

Agents can offload ML, alerting, dashboards, and retention to Parents - typically cutting agent CPU ≈50%, RAM ≈25%, and eliminating disk I/O entirely.

:::

### The Edge Advantage

Each Agent is autonomous:

- **Collects** 3,000-20,000 metrics per second per node
- **Stores** data in tiered storage (raw + aggregated)
- **Detects** anomalies using unsupervised ML
- **Triggers** alerts in real-time
- **Serves** local dashboards and APIs
- **Streams** to Parents for aggregation

This means:

- No data loss if Parent is down (Agents buffer locally)
- No performance degradation as you scale (work stays distributed)
- No architectural changes from 1 to 100,000 nodes

## The Parent Advantage: Intelligent Workload Distribution

### Why Parents Should Be Your Default

Parents aren't just centralization points - they're intelligent workload distributors that can reduce the resource footprint on production systems.

With Parents, Agents can offload:

- **ML Training** - Parents train models, Agents just collect (50% CPU reduction)
- **Health Checks** - Parents run all alerts, Agents focus on collection
- **Persistent Storage** - Agents run in RAM-only mode with zero disk I/O
- **Dashboard Serving** - Parents handle all queries and visualizations

A fully offloaded Agent uses &lt;2% CPU, &lt;150 MiB RAM, and zero disk I/O - a fraction of a standalone Agent.

### ML Intelligence: Train Where It Makes Sense

Netdata's ML models flow with the metrics stream, giving you complete flexibility:

**Option 1: ML at the Edge (default)**

- Agents train their own models locally
- Models stream to Parents along with metrics
- Parents receive pre-computed ML results
- Best for: Systems with available CPU, need for immediate local anomaly detection

**Option 2: ML at Parents**

- Agents disable ML training (50% CPU savings)
- First Parent trains models for all Agents
- Models shared with other Parents in cluster
- Best for: Resource-constrained production systems, centralized ML management

The architecture adapts to your needs - train where you have resources, use everywhere.

### When You Need Parents

**We recommend Parents by default:**

- Future-proof your architecture (same setup works at 10 or 100,000 nodes)
- Reduce production system load even at small scale
- Provide unified dashboards and centralized alerting
- Enable high availability and disaster recovery
- Cost less than the resources they save on production systems

**Parents are essential when you have:**

- **Ephemeral systems** - Kubernetes pods, auto-scaling VMs that disappear
- **Resource constraints** - Systems where every CPU cycle matters
- **On-premises requirements** - Multi-node view without Cloud connectivity
- **Network restrictions** - Agents can't reach Cloud due to firewalls/policies

### Parent Sizing Guidelines

| Nodes per Parent | Metrics/sec | Resources            |
|:----------------:|:-----------:|:---------------------|
|    ~100 nodes    |  ~0.5M/sec  | 5 cores, 20 GiB RAM  |
|    ~250 nodes    |   ~1M/sec   | 10 cores, 40 GiB RAM |
|    ~500 nodes    |   ~2M/sec   | 20 cores, 80 GiB RAM |

:::tip

Scale horizontally with more Parents, not vertically with bigger Parents. Beyond 500 nodes per Parent, resource usage grows non-linearly.

:::

### Parent Placement Strategy

- **Keep Parents close** to their Agents (same datacenter, region, or cloud zone)
- **Minimize network hops** to reduce latency and bandwidth costs
- **Deploy per region** in multi-region architectures
- **Use multiple Parents** rather than one giant Parent

### High Availability & Intelligent Clustering

Parents work together intelligently to eliminate duplicate work:

- **Active-active Parents** with automatic work distribution
- **ML model sharing** - First Parent trains, others receive models
- **Automatic failover** - Agents reconnect to available Parents
- **Local buffering** - Agents retain 1+ hour of data during Parent downtime
- **Streaming replication** between Parents for complete redundancy
- **Federated queries** across all Parents via Netdata Cloud

**Key insight:** Clustering without double-spend: In an active-active cluster, the first Parent that sees a child trains the model; peers reuse it. You get HA without multiplying heavy work.

### Alerts: Automation vs Monitoring

Netdata separates automation from monitoring, letting you optimize both:

**Agents: Local Automation**

- Keep only alerts that trigger local scripts
- Example: "If CPU > 90%, scale this service"
- Immediate response, no network dependency
- Minimal overhead when selective

**Parents: Human Monitoring**

- Run comprehensive health checks for all Agents
- Send notifications to teams via Cloud or integrations
- Correlate issues across multiple systems
- Rich context for troubleshooting

This dual approach means production systems only run automation-critical alerts while Parents handle the hundreds of monitoring alerts that humans need to see.

## Storage: Efficient Multi-Tier Architecture

### Three-Tier Storage System

| Tier       | Resolution | Compression                | Retention       | Use Case          |
|------------|------------|----------------------------|-----------------|-------------------|
| **Tier 0** | Per-second | Minimal (0.6 bytes/sample) | Days to Weeks   | Troubleshooting   |
| **Tier 1** | Per-minute | High                       | Weeks to Months | Trending          |
| **Tier 2** | Per-hour   | Maximum                    | Months to Years | Capacity planning |

All tiers update in parallel - no post-processing or compaction jobs needed.

### Storage Efficiency

- **0.6 bytes per sample** - industry's most efficient
- **Gorilla + ZSTD compression** for optimal size/speed
- **WORM design** - append-only, no expensive compaction

## Why This Architecture Wins

### For Operations Teams

- **No blind spots** during incidents - all data available
- **No architectural rewrites** as you scale
- **No sampling lottery** - the metric you need is always there
- **No specialized skills** required - it just works

### For Finance

- **Predictable costs** - linear scaling, no surprises
- **Lower TCO** - fewer resources for same visibility
- **Energy efficient** - independently validated lowest overhead
- **Reduced team size** - less complexity to manage

### For Developers

- **Per-second granularity** - see what actually happened
- **Real-time anomaly detection** - catch issues immediately
- **Local dashboards** - debug without central bottlenecks
- **Full cardinality** - every dimension tracked

## The Bottom Line

Through intelligent workload distribution between Parents and Agents:

- **ML trains where you have resources** (edge or Parents, your choice)
- **Alerts run where they matter** (automation locally, monitoring centrally)
- **Storage happens where it's cheap** (Parents, not production)
- **Millions of metrics per second is normal** (not heroic)
- **HA doesn't multiply overhead** (intelligent clustering)

This isn't just optimization. It's a fundamentally different architecture that recognizes observability shouldn't compete with your applications for resources.

**Welcome to observability that makes your infrastructure better, not heavier.**

## FAQ

<details>
<summary><strong>How many nodes can a single Netdata Parent handle?</strong></summary><br/>

We recommend running Parents with up to 500 Agents (1.5M metrics/s). We have customers running larger Parents, but resources increase and performance decreases non-linearly.

</details>

<details>
<summary><strong>What happens if a Parent goes down?</strong></summary><br/>

If the Parent was clustered, agents will connect to the other Parent and replicate to it any metrics collected during the transition. If there is no other Parent to connect to, Agents keep collecting and storing data locally, which will be replicated to the Parent when it becomes available. Note that the replication of past metrics uses only tier-0 (high-res data), so Agents must have enough retention in tier-0 to avoid gaps in the charts.

</details>

<details>
<summary><strong>Do I always need Parents?</strong></summary><br/>

No. Agents alone may be enough. Parents are usually required when you have ephemeral nodes.

</details>

<details>
<summary><strong>How much overhead does Netdata introduce on my systems?</strong></summary><br/>

Less than 5% CPU and ~200 MiB RAM per agent in standalone mode. Offloaded agents (streaming to a Parent) drop to less than 2% CPU and ~150 MiB RAM with zero disk I/O. Netdata is designed to be "polite citizen" to production workloads, so it spreads its workload across time and avoids all kinds of sudden and intense spikes.

</details>

<details>
<summary><strong>How efficient is Netdata's storage?</strong></summary><br/>

Tier 0 (per-second) is ~0.6 bytes/sample - the industry's most efficient. Tiers 1 & 2 keep per-minute and per-hour aggregates, letting you retain months or years of history cheaply.

</details>

<details>
<summary><strong>How do I deploy Parents in multi-region or multi-cloud setups?</strong></summary><br/>

Place Parents close to the agents they serve (same DC/region/AZ). Deploy multiple Parents per region for HA. Use Netdata Cloud to unify dashboards and queries across Parents.

</details>

<details>
<summary><strong>What's the difference between monitoring and automation alerts?</strong></summary><br/>

Since Netdata evaluates alerts at the edge, it allows you to specify scripts to be executed when an alert triggers. This enables automation, e.g. "restart service if API endpoint is not responding".

</details>

<details>
<summary><strong>Is Netdata really energy-efficient?</strong></summary><br/>

Yes. A peer-reviewed 2023 study (ICSOC, University of Amsterdam) found Netdata to be the most energy-efficient tool among the ones tested, with the lowest CPU and RAM overhead even at 1-second collection.

</details>

<details>
<summary><strong>Is 100,000+ nodes single installation real?</strong></summary><br/>

Yes. Even Netdata Cloud SaaS itself (our commercial service) is such a single installation that serves way more than 100k reachable nodes.

</details>

<details>
<summary><strong>Do you promote per-second collection and unlimited metrics because your revenue depends on volume?</strong></summary><br/>

No. Our commercial offerings are priced per node, with volume discounts (smaller price as the number of nodes increases). Our revenue is not related to the number of metrics or the volume of observability data collected or viewed. We designed Netdata for maximum performance at scale and volume for your benefit. Not ours.

</details>

<details>
<summary><strong>If I have multiple Parents, how does Netdata Cloud provide unified dashboards?</strong></summary><br/>

Think of Netdata Cloud as the headend of a distributed database. Each Netdata Parent and Agent dynamically becomes part of that database. So, Netdata Cloud queries them all in parallel, to provide the unified view required.

</details>

<details>
<summary><strong>Is querying 100 remote systems in parallel slower than querying a bigger one locally?</strong></summary><br/>

There is some extra network latency involved, but this is usually small (a few ms), because the data transferred are tiny (your web browser will receive 500-1000 points max, even if the query is 10 days of per-second data). However, the aggregate horse power and parallelism of 100 totally independent systems is orders of magnitude more, compared to any single local system. The queries are actually quite faster.

</details>

## Next Steps

- **[Deploy your first Agent](/docs/deployment-guides/standalone-deployment.md)** - Start monitoring in 60 seconds
- **[Configure Parents](/docs/deployment-guides/deployment-with-centralization-points.md)** - Scale to hundreds of nodes
- **[Design for Enterprise](/docs/netdata-enterprise-evaluation-corrected.md)** - Architect for thousands
- **[Try Netdata Cloud](/docs/netdata-cloud/README.md)** - Unified visibility across everything

*Based on real production deployments, independent research (University of Amsterdam, ICSOC 2023), and comparative testing (2025). All metrics and resource usage figures represent typical production scenarios.*
