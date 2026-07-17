# Real-Time Monitoring: The Netdata Standard

## TL;DR

Netdata focuses on low-latency infrastructure monitoring. Core system metrics default to one-second collection, and a local dashboard polling every second adds less than two seconds from tick alignment. Actual end-to-end latency and overhead depend on the collector, monitored source, infrastructure, and access path.

With Netdata, organizations gain:

- **Faster MTTR:**  Instant feedback accelerates troubleshooting.
- **Accurate Detection:**  See true patterns, not averages.
- **Resilience:**  Catch spikes before they cascade into outages.
- **Security:**  Detect attacks unfolding in mere seconds.
- **Operational Efficiency:**  Enable smarter autoscaling and container monitoring.

In short: when every second matters, Netdata is the only monitoring solution proven to deliver real-time performance at global scale.

---

Netdata pioneered the 1-second standard for operational monitoring. While other solutions typically operate on 10-second, 30-second, or minute-level intervals, Netdata has proven that sustained per-second monitoring at scale is both achievable and essential.

This document defines real-time monitoring, explains why it matters, and details how Netdata achieves it where others cannot.

## What is Real-Time Monitoring?

### Industry Definitions

The Webster dictionary defines _real-time_ as _“the simultaneous recording of an event with the actual occurrence of that event.”_

In engineering, the meaning extends to systems that **record and respond to events instantly**.

As Richard Hackathorn wrote in _[The BI Watch: Real-Time to Real-Value](https://www.researchgate.net/publication/228498840_The_BI_watch_real-time_to_real-value)_:

> _The key concept behind “real time” is that our artificial representation of the world must be in sync with the real world, so that we can respond to events in an effective manner._

Wikipedia defines **[real-time business intelligence](https://en.wikipedia.org/wiki/Real-time_business_intelligence)** as a range from _milliseconds to ≤ 5 seconds_ after an event has occurred, and identifies three types of latency involved:

- **Data latency** - time to collect and store data
- **Analysis latency** - time to process data
- **Action latency** - time to act on the data (e.g., visualize or alert in observability)

For a system to qualify as _real-time_, the **sum of all three latencies** must remain within the real-time window of ≤ 5 seconds.

### The Netdata Standard for Real-Time

Based on these definitions, Netdata establishes this practical taxonomy:

|     Classification | Total Latency | Netdata's Position               |
|-------------------:|:-------------:|:---------------------------------|
|      **Real-time** |  ≤ 5 seconds  | ✅ Netdata: 1-2 seconds total     |
| **Near real-time** | 5-30 seconds  | ❌ Most "modern" monitoring tools |
|  **Not real-time** | > 30 seconds  | ❌ Traditional monitoring systems |

**Netdata is designed to keep the sum of all three latencies under 2 seconds**, making it one of the few monitoring systems that qualifies as truly real-time at scale, under rigorous definitions.

## Why Real-Time Monitoring is Non-Negotiable

### The Pillars of Operational Excellence

1. **Faster MTTR**<br/>
  Engineers see the immediate effect of a change: alter an index, instantly see queries speed up. Without real-time monitoring, this iterative troubleshooting takes 10-30x longer.
2. **Accurate Incident Detection**<br/>
  100% CPU for 2 seconds then 0% for 8 seconds is fundamentally different from steady 20% CPU, but averaged metrics hide that pattern, delaying root cause identification.
3. **Preventing Cascading Failures**<br/>
  A 3-second resource spike can trigger a 30-second cascade if not caught immediately. Real-time monitoring catches the spark before it becomes a fire.
4. **Security Threat Detection**<br/>
  Modern attacks happen in seconds: port scans, crypto-miner activation, memory scanning bursts, all invisible to systems monitoring at 30-second intervals.

### Use Cases That Demand Real-Time

#### Autoscaling Decisions

Cloud autoscalers with 30-second visibility over-provision (delayed spike detection triggers unnecessary scaling), under-provision (missed micro-bursts degrade user experience), or flap (slow feedback loops oscillate scale up/down). Netdata's 1-second monitoring shows autoscalers the actual load pattern.

#### Database Performance Tuning

A query that runs 3 seconds every 10 seconds shows as a misleading 30% constant load with 10-second monitoring, versus its actual 100% spike pattern with Netdata, letting DBAs immediately see the effect of index or query plan changes.

#### Container and Kubernetes Monitoring

Container lifespans can be seconds: pod scheduling, startup, and shutdown often complete in under 5 seconds, invisible to traditional monitoring. Netdata captures the full lifecycle.

#### Typical System Administration

**Scenario 1: Disk throughput**<br/>
An application reads disk at 500 MB/s for 5 seconds, then idles for 10. A 15-second average shows 167MB/s: "contact the developers, disks can go faster." Netdata shows 5 seconds of saturation then idle: "already maxing the disks, install faster ones."

**Scenario 2: Network saturation**<br/>
A transactional database stalls for 10 seconds every 5 minutes. A 1-minute average shows network usage rising slightly (200→220 Mb/s), looking like noise. Netdata reveals a cron job saturating the network for those 10 seconds, starving the database.

Coarse averages let every team point at someone else. Netdata shows the whole signal at per-second fidelity, shifting the debate from "who's at fault?" to "how do we fix it?"

## The Anatomy of Netdata’s Latency

For a metric collected every second and a local dashboard refreshing every second, tick alignment usually contributes about one second of latency and at most just under two seconds:

1. **Data latency:** up to 1 second from event
2. **Analysis latency:** microseconds (negligible, CPU-speed dependent)
3. **Action latency:** up to 1 second from collection

This simplified model means:

- **Best case:** the event is collected and displayed almost immediately when the ticks align
- **Worst case:** tick alignment alone contributes just under 2 seconds

Collector execution time, network transit, query execution, and browser rendering can add latency.

Graphically:

```mermaid
flowchart LR
    event["Event starts just after a collection tick"]
    collected["Next collection tick<br/>Event collected"]
    missed["UI refresh narrowly misses the new result"]
    displayed["Next UI refresh<br/>Event displayed"]

    event -->|"less than 1 second"| collected
    collected --> missed
    missed -->|"less than 1 second"| displayed
```

The two waits show the worst-case alignment of 1-second collection and visualization ticks. Together they contribute less than 2 seconds; the complete end-to-end latency also depends on the collector and access path.

### Why One Second is the Ideal Standard

- **The Universal Baseline:** 1-second is the native rhythm of console tools like `top`, `vmstat`, and `iostat`.
- **The Performance Sweet Spot:** Sub-second intervals increase collection overhead; one second is a practical default for many dynamic system metrics.
- **Sufficient Resolution:** Most operational anomalies last multiple seconds, so 1-second granularity captures them without loss of fidelity.
- **Negligible Overhead:** A few thousand metrics per second is a trivial fraction of a single CPU core's billions of cycles.

### Intentional Deviation from Per-Second Collection

Netdata intentionally uses a longer collection interval _only_ in two specific scenarios:

1. The underlying metric changes slowly (e.g., a temperature sensor).
2. Per-second polling would place undue stress on the monitored application (e.g., a delicate legacy system).

In these cases, Netdata chooses a responsible interval that balances fidelity with non-intrusiveness.

### Beyond Latency: The Meaning of Gaps

Netdata operates with the precision of a heartbeat: metrics must be collected at their configured frequency, unlike solutions that treat missed collections as a normal network condition.

A gap in a Netdata chart represents samples that are unavailable for that time range, not a visualization trick. Collection failures, Agent downtime, or an interruption that exceeds the available replication retention can all create gaps, so investigate the collector and data path before attributing the cause.

## Netdata's Real-Time Performance at Scale

Netdata is a distributed-by-design platform that scales horizontally by adding Agents and streaming Parents. Collection and storage remain distributed, so adding nodes does not inherently lengthen collection intervals on existing nodes. End-to-end latency still depends on each collector, the infrastructure, and the query path.

## How Netdata Compares to Other Monitoring Solutions

While most tools can be _configured_ for faster polling, their core architecture is not optimized for sustained, pervasive, per-second collection without excessive overhead or cost. This table reflects their typical, real-world deployment:

|      Monitoring Solution |   Collection Interval   |  Real Latency  | Why It's Not Real-Time                    |
|-------------------------:|:-----------------------:|:--------------:|:------------------------------------------|
|              **Netdata** |        1 second         |  1-2 seconds   | ✅ **Defines the true real-time standard** |
| **Prometheus + Grafana** | 10-30 seconds (typical) | 15-40 seconds  | Pull-based scraping limits frequency      |
|              **Datadog** |      15-60 seconds      | 20-90 seconds  | Agent batching + cloud processing         |
|            **New Relic** |  60 seconds (default)   | 60-120 seconds | Minute-level aggregation model            |
|        **Grafana Cloud** |      10-30 seconds      | 20-60 seconds  | Prometheus-based limitations              |
|           **CloudWatch** |     60-300 seconds      | 60-360 seconds | AWS API polling constraints               |
|               **Zabbix** |      30-60 seconds      | 30-90 seconds  | Server polling architecture               |
|               **Nagios** |     60-300 seconds      | 60-600 seconds | Check-based paradigm                      |
|        **Elastic Stack** |      10-30 seconds      | 30-300 seconds | Ingest → Index → Query pipeline           |
|            **Dynatrace** |      10-60 seconds      | 15-90 seconds  | Cloud analysis latency                    |
|          **AppDynamics** |       60 seconds        | 60-180 seconds | Minute-level business focus               |
|               **Splunk** |     30-300 seconds      | 60-600 seconds | Log indexing architecture                 |

:::note

Competitor figures reflect typical default configurations and publicly documented polling models, not vendor-published latency benchmarks; actual behavior varies by plan, configuration, and deployment. The "Real Latency" column is an architectural estimate (collection interval plus typical pipeline/processing overhead), not a measured or vendor-confirmed figure.

:::

### The Prometheus + Grafana Reality

Designed around 10-30 second scrape intervals; 1-second scraping is an anti-pattern that overloads targets. The pull model and Grafana both add query latency on top. **Verdict**: excellent for historical trending, but architecturally blind to micro-events. Not real-time.

**Could Prometheus be real-time?** Technically yes, practically no: matching Netdata's data density would need an order of magnitude more servers, would overwhelm the monitored applications with scraping, and the Prometheus TSDB isn't built for this volume of per-second data.

Our [stress test](https://www.netdata.cloud/blog/netdata-vs-prometheus-2025/) against a Prometheus stack at 4.6M metrics/s proved this: Netdata used **-37% CPU, -88% RAM, -13% bandwidth, and -97% disk I/O** while providing **40x longer retention** and **22x faster queries**.

### The Datadog Reality

The agent defaults to 15-second intervals, batches data before sending to the cloud, and cloud processing adds another 5-30 seconds. **Verdict**: powerful analytics, but architecturally not real-time.

**Q: Datadog claims "real-time" features. How is Netdata different?**<br/>
A: Datadog's "real-time" usually means live-tail for logs or near-live infrastructure views, with 20-90 second metric pipeline latency. Netdata collects core system metrics every second by default and provides low-latency local visualization; actual latency varies by collector and access path.

**Q: Couldn't Datadog just collect faster?**<br/>
A: Their business and architectural model prevents it: 1-second collection would increase metric volume 15-60x (cost-prohibitive under their pricing), the agent-to-cloud-to-UI journey adds immutable latency, and bandwidth costs would be enormous. Netdata's edge-native architecture avoids these bottlenecks by collecting, storing, and serving locally.

### The New Relic Reality

Focused on application performance (APM) with 1-minute default reporting, relying on sampling for high-volume events. **Verdict**: powerful for code-level insights, but minute-level granularity misses system-level patterns. Not real-time.

**Q: New Relic has 1-minute metrics. Isn't that enough?**<br/>
A: Absolutely not. They hide the most critical performance patterns. A 5-second query running every 20 seconds appears as a benign 25% load average in New Relic but is immediately obvious as a damaging spike pattern in Netdata.

## The Netdata Real-Time Monitoring Manifesto

We believe real-time monitoring is a right, not a premium feature. Our principles are:

1. **Every Second Matters:** Problems lasting 3 seconds are primary incidents, not statistical noise.
2. **Gaps Stay Visible:** Missing samples are preserved as gaps and should be investigated, not silently interpolated.
3. **Fidelity with Transparency:** Netdata preserves source-resolution data where practical and documents when a collector estimates, samples, or aggregates values.
4. **Edge-Native Collection:** Intelligence and storage belong at the source of the data.
5. **Push-Based Streaming:** Data flows outward as it happens; systems shouldn't wait to be polled.
6. **Truly Horizontal Scaling:** Adding monitoring capacity must not degrade existing performance.
7. **Production-Safe Efficiency:** Real-time cannot come at the cost of operational stability.

## Netdata by the Numbers: The Proof is in the Performance

### Per-Node Performance

- **Granularity:** Core system metrics default to 1-second collection; collectors for slower or polling-sensitive sources can use longer configurable intervals.
- **Volume:** 3,000-20,000+ metrics collected per second per node.
- **CPU Overhead:** Less than 5% of a single core, typical for 3,000 metrics/s.
- **RAM Footprint:** Less than 200 MB, typical for 3,000 metrics/s.
- **Fault Tolerance:** Retained tier-0 samples can be replayed after network interruptions; sufficient local retention is required to avoid gaps.
- **Storage Efficiency:** ~0.6 bytes per sample on disk, enabling years of retention for gigabytes, not terabytes.

For default-settings sizing guidance (CPU, RAM, disk, and bandwidth) across different workloads, see [Resource utilization](/docs/netdata-agent/sizing-netdata-agents/README.md).

### Global Scale

- **Billions of Metrics:** Processes over 4.5 billion metrics per second across all installations.
- **100% Sampling Rate:** No statistical sampling - every data point is captured.
- **Metric-Volume Licensing:** The open-source Agent does not impose a metric-volume license limit; consult the [pricing page](https://www.netdata.cloud/pricing/) for current Cloud entitlements.
- **Proven at Scale:** Monitors infrastructures with 100,000+ nodes seamlessly.

### Enterprise-Grade Reliability

- **No Single Point of Failure:** Fully distributed, peer-to-peer streaming architecture.
- **Automatic Failover:** Resilient streaming connections with self-healing.
- **Self-Managing:** Requires minimal administration.
- **Zero-Downtime Updates:** Supports rolling updates without interrupting monitoring.

It is common for large Netdata deployments to process millions of metrics per second across a highly available, distributed fleet, all while remaining virtually invisible from an administrative overhead perspective. This operational elegance is Netdata's ultimate success.

## Real-Time Monitoring FAQ

**Q: Is Netdata truly real-time?**<br/>
A: For metrics collected every second, Netdata provides per-second granularity and low-latency visualization. Actual end-to-end latency depends on the collector and access path.

**Q: Why is 1-second resolution critical compared to 10 or 30-second?**<br/>
A: Most operational anomalies last 2-10 seconds. 30-second monitoring is blind to over 90% of incidents; 10-second monitoring still misses roughly 50%.

**Q: Doesn't per-second monitoring create unsustainable overhead?**<br/>
A: No. Netdata typically uses less than 5% of a single CPU core (see [CPU requirements](/docs/netdata-agent/sizing-netdata-agents/cpu-requirements.md)), and the [University of Amsterdam study](https://www.ivanomalavolta.com/files/papers/ICSOC_2023.pdf) found it the most energy-efficient tool tested for Docker-based systems.

**Q: How does Netdata handle network outages?**<br/>
A: Agents retain tier-0 metrics locally and replay available samples after reconnection. Gaps can occur when an outage exceeds local retention or collection stops while a node is unavailable; each Agent or Parent also has its own dashboard for troubleshooting during the outage.

**Q: Can Netdata handle cloud and container environments?**<br/>
A: Yes, natively. Automatic discovery and per-second monitoring for Kubernetes, Docker, and major cloud platforms, collecting cgroups metrics directly from the kernel.

**Q: What is the core architectural difference between Netdata and Prometheus/Datadog?**<br/>
A: Netdata is distributed and edge-native; Prometheus is a pull-based centralized scraper; Datadog is a cloud SaaS platform. Those models impose latency that Netdata's architecture avoids.

**Q: Is real-time necessary for every single metric?**<br/>
A: No. Netdata adjusts collection frequency down for slow-changing or polling-sensitive metrics, while keeping 1-second collection for dynamic system and application metrics.

**Q: Does querying more time-series slow down the dashboard?**<br/>
A: Rendering thousands of lines is a browser limitation, not Netdata's. High-cardinality views get instant aggregated results with real-time drill-down.

**Q: What are the trade-offs for being real-time?**<br/>
A: Architectural complexity. Distributing intelligence to the edge and synchronizing data without central bottlenecks is a harder engineering problem, one Netdata solves so you don't have to trade real-time visibility for operational cost.

## Summary

Real-time monitoring is not a luxury - it is a fundamental requirement for operating modern, dynamic, and complex infrastructure. The difference between 1-second and 30-second visibility is the difference between preventing outages and merely documenting them.

Netdata doesn't just claim to be real-time; it defines the category through engineering excellence:

- **Low Latency:** Per-second collectors and local dashboards minimize collection-to-visualization delay; actual latency depends on the collector and access path.
- **High Granularity:** Core system metrics default to one-second collection, with configurable intervals for other sources.
- **Visible Data Quality:** Missing samples remain gaps, and collector-specific estimates or aggregations are documented.
- **Distributed Scale:** Agents and Parents distribute collection, storage, and query work across the infrastructure.
- **Production-Safe:** Configurable collection intervals balance fidelity with source and host overhead.

When you need to see what's actually happening right now, not a smoothed-over report of a minute ago, this is real-time monitoring. This is The Netdata Standard.
