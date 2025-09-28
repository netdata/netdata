# Real-Time Monitoring: The Netdata Standard

## TL;DR

Netdata defines what real-time monitoring truly means:  **1-second data collection and 1-second latency from collection to visualization**, providing a worst-case latency of less that 2 seconds from event to insight, at any scale. While most monitoring systems operate on 10–60-second intervals, Netdata provides  _true_  sub-2-second visibility without overhead. This difference is critical: most operational anomalies last under 10 seconds, which means traditional monitoring misses them completely.

With Netdata, organizations gain:

-   **Faster MTTR:**  Instant feedback accelerates troubleshooting.
-   **Accurate Detection:**  See true patterns, not averages.
-   **Resilience:**  Catch spikes before they cascade into outages.
-   **Security:**  Detect attacks unfolding in mere seconds.
-   **Operational Efficiency:**  Enable smarter autoscaling and container monitoring.

In short: when every second matters, Netdata is the only monitoring solution proven to deliver real-time performance at global scale.

---

Netdata pioneered the 1-second standard for operational monitoring, establishing what true real-time monitoring means in practice. While other monitoring solutions typically operate on 10-second, 30-second, or even minute-level intervals, Netdata has proven that sustained per-second monitoring at scale is both achievable and essential for modern operations.

This document defines real-time monitoring, explains why it matters, and details how Netdata achieves true real-time performance where others cannot.

## What is Real-Time Monitoring?

### Industry Definitions
The Webster dictionary defines _real-time_ as _“the simultaneous recording of an event with the actual occurrence of that event.”_

In engineering, the meaning extends to systems that **record and respond to events instantly**.

As Richard Hackathorn wrote in _[The BI Watch: Real-Time to Real-Value](https://www.researchgate.net/publication/228498840_The_BI_watch_real-time_to_real-value)_:

> _The key concept behind “real time” is that our artificial representation of the world must be in sync with the real world, so that we can respond to events in an effective manner._

Wikipedia defines **[real-time business intelligence](https://en.wikipedia.org/wiki/Real-time_business_intelligence)** as a range from _milliseconds to ≤ 5 seconds_ after an event has occurred, and identifies three types of latency involved:

-   **Data latency** - time to collect and store data
-   **Analysis latency** - time to process data
-   **Action latency** - time to act on the data (e.g., visualize or alert in observability)

For a system to qualify as _real-time_, the **sum of all three latencies** must remain within the real-time window of ≤ 5 seconds.

### The Netdata Standard for Real-Time
Based on these definitions, Netdata establishes this practical taxonomy:

|     Classification | Total Latency | Netdata's Position               |
| -----------------: | :-----------: | :------------------------------- |
|      **Real-time** |  ≤ 5 seconds  | ✅ Netdata: 1-2 seconds total     |
| **Near real-time** | 5-30 seconds  | ❌ Most "modern" monitoring tools |
|  **Not real-time** | > 30 seconds  | ❌ Traditional monitoring systems |

**Netdata is designed to keep the sum of all three latencies under 2 seconds**, making it one of the few monitoring systems that qualifies as truly real-time at scale, under rigorous definitions.

## Why Real-Time Monitoring is Non-Negotiable

### The Pillars of Operational Excellence

1. **Faster Mean Time to Resolution (MTTR)**<br/>
  Engineers see the immediate effects of their actions. When a database is slow, they alter an index and instantly observe queries running faster and resource relief. Without real-time monitoring, this iterative troubleshooting process takes 10-30x longer.
2. **Accurate Incident Detection**<br/>
  Real-time monitoring reveals the true behavior of systems. An application using 100% CPU for 2 seconds then 0% for 8 seconds is fundamentally different from one using steady 20% CPU. Averaged metrics hide critical patterns, confuse operations teams, and delay root cause identification.
3. **Preventing Cascading Failures**<br/>
  Problems compound exponentially. A 3-second resource spike can trigger a 30-second cascade if not caught immediately. Real-time monitoring catches the spark before it becomes a fire.
4. **Security Threat Detection**<br/>
  Modern attacks happen in seconds: port scans (2-3 seconds), crypto-miner activation (instant CPU spikes), memory scanning attempts (burst patterns). These are invisible to systems monitoring at 30-second intervals.

### Use Cases That Demand Real-Time

#### Autoscaling Decisions
Cloud autoscalers with 30-second visibility suffer from:

-   **Over-provisioning**: Delayed spike detection triggers unnecessary scaling
-   **Under-provisioning**: Missed micro-bursts cause user-facing degradation
-   **Flapping**: Slow feedback loops create oscillating scale up/down cycles

With Netdata's 1-second monitoring, autoscalers see actual load patterns and make informed decisions.

#### Database Performance Tuning
A problematic query that runs for 3 seconds every 10 seconds shows as:

-   **30% constant load** with 10-second monitoring (misleading average)
-   **100% spike pattern** with Netdata (actual behavior)

DBAs using Netdata can immediately see the effects of index changes, query plan modifications, and connection pool adjustments.

#### Container and Kubernetes Monitoring
Container lifespans can be seconds. Pod scheduling, startup, and shutdown events often complete in under 5 seconds. Traditional monitoring completely misses these critical events. Netdata captures the full lifecycle.

#### Typical System Administration
**Scenario 1: Disk throughput**<br/>
An application performs disk reads at **500 MB/s for 5 seconds**, then is idle for 10 seconds. Can the application be made faster?

- **Other monitoring solutions** show 15-second averages at 167MB/s.  SREs: "The application can be made faster, the disks can provide up to 500MB/s. Contact the developers".
- **Netdata** shows saturation for 5 seconds, then idle. SREs: "The application is already maxing the disks. Install faster disks."

**Scenario 2: Network saturation**<br/>
A sensitive transactional database stalls for 10 seconds every 5 minutes.

-   **Other monitoring solutions** (1-minute averages) show network usage rising slightly, from **200 Mb/s → 220 Mb/s** once every 5 minutes. This looks harmless, almost noise.
-   **Netdata** reveals the truth: a cron job runs every 5 minutes, transferring several GiB to a backup server. For those 10 seconds, the network is fully saturated, starving the database of bandwidth.

With coarse averages, every team sees evidence that _someone else_ is at fault. Netdata stops the blame game by showing the **whole signal at per-second fidelity**. The debate shifts from _“who do we blame?”_ to _“how do we fix it?”_.

## The Anatomy of Netdata’s Latency

Netdata is designed to keep the **sum of all three latencies at about 1 second **. with the worst case scenario at 2 seconds:

1.  **Data latency:** up to 1 second from event
2.  **Analysis latency:** microseconds (negligible, CPU-speed dependent)
3.  **Action latency:** up to 1 second from collection

This means:

-   **Best case:** ~1 millisecond from event to visualization
-   **Worst case:** ~1999 milliseconds if collection and visualization ticks are maximally misaligned

Graphically:

```
Data Collection Pace

   the interesting event started
   ↓         event collected
   ↓         ↓
──┬██████████┬──────────┬──────────┬──────────┬   ← data collection pace
  │ ── 1s ── │ ── 1s ── │ ── 1s ── │ ── 1s ── │

Visualization Pace (a few ms misaligned to collection - worst case)

                      UI fetches everything collected
                      ↓
┬──────────┬──────────┬██████████┬──────────┬──   ← visualization pace
│ ── 1s ── │ ── 1s ── │ ── 1s ── │ ── 1s ── │
                      ↑
                      event visualized
```

The shaded boxes show the slices where an event may fall. Because both collection and visualization run on 1-second ticks, the event is guaranteed to be visible within 2 seconds.

### Why One Second is the Ideal Standard

-   **The Universal Baseline:** 1-second is the native rhythm of universal console tools (`top`, `vmstat`, `iostat`).
-   **The Performance Sweet Spot:** Moving to sub-second intervals (e.g., 500ms) often doubles overhead for diminishing returns. One second is highly efficient and universally safe.
-   **Sufficient Resolution:** The vast majority of operational anomalies last multiple seconds; 1-second granularity captures them without loss of fidelity.
-   **Negligible Overhead:** Modern systems handle per-second sampling with ease; collecting a few thousand metrics per second consumes a trivial fraction of a single CPU core's billions of cycles.

### Intentional Deviation from Per-Second Collection
Netdata intentionally uses a longer collection interval _only_ in two specific scenarios:

1.  The underlying metric changes slowly (e.g., a temperature sensor).
2.  Per-second polling would place undue stress on the monitored application (e.g., a delicate legacy system).

In these cases, Netdata chooses a responsible interval that balances fidelity with non-intrusiveness.

### Beyond Latency: The Meaning of Gaps
Netdata operates with the precision of a heartbeat. Unlike other solutions that treat missed collections as a normal network condition, Netdata is engineered as a watchdog: **metrics must be collected at their configured frequency**.

A gap in the Netdata dashboard is not a visualization trick or a "network blip." It is a **persisted gap in the database** written to disk. It is a definitive signal that the system itself was under such severe stress that it could not service Netdata's lightweight collection process. If Netdata is missing samples, your application is almost certainly missing service-level objectives.

This philosophy makes Netdata not just a monitoring tool, but a canary for system health itself.

## Netdata's Real-Time Performance at Scale

Netdata is a distributed-by-design platform. It scales horizontally by adding more Agents and streaming Parents. This architecture is key to its real-time capability: **adding more nodes does not increase the latency or impair the performance of existing ones.** Each node operates independently at its own 1-second rhythm, collaborating in real-time. A fleet of 10 nodes and a fleet of 10,000 nodes exhibit the same per-node, sub-2-second latency.

## How Netdata Compares to Other Monitoring Solutions
While most tools can be _configured_ for faster polling, their core architecture is not optimized for sustained, pervasive, per-second collection without excessive overhead or cost. This table reflects their typical, real-world deployment:

|      Monitoring Solution |   Collection Interval   |  Real Latency  | Why It's Not Real-Time                    |
| -----------------------: | :---------------------: | :------------: | :---------------------------------------- |
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

### The Prometheus + Grafana Reality
- Designed around 10-30 second scrape intervals; 1-second scraping is anti-pattern that overloads targets.
- Pull model adds network latency to every collection
- Grafana adds additional query latency on top
- **Verdict**: Excellent for historical trending and long-term data analysis, but architecturally blind to micro-events. Not real-time.

**Could Prometheus Be Real-Time?**
Technically, yes, but practically, no. To match Netdata's data density:

1.  **Scalability Collapse:** Would require an order of magnitude more Prometheus servers, dramatically increasing cost and complexity.
2.  **Target Assault:** Frequent scraping would overwhelm the very applications it's meant to monitor.
3.  **Storage Inefficiency:** The Prometheus TSDB isn't optimized for this volume of per-second data.

Our [stress test](https://www.netdata.cloud/blog/netdata-vs-prometheus-2025/) against a Prometheus stack at 4.6M metrics/s proved this: Netdata used **-37% CPU, -88% RAM, -13% bandwidth, and -97% disk I/O** while providing **40x longer retention** and **22x faster queries**.

### The Datadog Reality
-   The agent defaults to 15-second intervals for most metrics.
-   It batches data for seconds before sending to the cloud.
-   Cloud processing and indexing add another 5-30 seconds of latency.
-   **Verdict**: A powerful analytics platform, but its architecture imposes fundamental latency constraints. It is not real-time.

**Q: Datadog claims "real-time" features. How is Netdata different?**<br/>
A: Datadog's "real-time" typically refers to live-tail for logs or near-live infrastructure views. Their metric pipeline latency is 20-90 seconds. Netdata provides true 1-2 second latency for all metrics, everywhere.

**Q: Couldn't Datadog just collect faster?**<br/>
A: No. Their business and architectural model prevents it:

-   **Cost Prohibitive:** 1-second collection increases metric volume 15-60x, making their pricing model untenable for customers.
-   **Cloud Bottlenecks:** The journey from agent to cloud to UI introduces immutable latency.
-   **Bandwidth:** The data transfer costs would be enormous.

Netdata's edge-native architecture eliminates these bottlenecks by collecting, storing, and serving metrics locally.

### The New Relic Reality
-   Focused on application performance (APM) with 1-minute default reporting.
-   Relies on sampling for high-volume events, sacrificing fidelity.
-   **Verdict**: Powerful for code-level application insights, but its minute-level granularity misses crucial system-level patterns. Not real-time.

**Q: New Relic has 1-minute metrics. Isn't that enough?**<br/>
A: Absolutely not. They hide the most critical performance patterns. A 5-second query running every 20 seconds appears as a benign 25% load average in New Relic but is immediately obvious as a damaging spike pattern in Netdata.

## The Netdata Real-Time Monitoring Manifesto

We believe real-time monitoring is a right, not a premium feature. Our principles are:

1.  **Every Second Matters:** Problems lasting 3 seconds are primary incidents, not statistical noise.
2.  **Gaps Are Failures:** Missing data is a symptom of system distress, not an acceptable network condition.
3.  **Fidelity Over Approximation:** No sampling, no averaging, no estimation - only ground truth.
4.  **Edge-Native Collection:** Intelligence and storage belong at the source of the data.
5.  **Push-Based Streaming:** Data flows outward as it happens; systems shouldn't wait to be polled.
6.  **Truly Horizontal Scaling:** Adding monitoring capacity must not degrade existing performance.
7.  **Production-Safe Efficiency:** Real-time cannot come at the cost of operational stability.

## Netdata by the Numbers: The Proof is in the Performance

### Per-Node Performance

-   **Granularity:** 1-second for all metrics, without exception.
-   **Volume:** 3,000-20,000+ metrics collected per second per node.
-   **CPU Overhead:** Less than 5% of a single core, typical for 3,000 metrics/s.
-   **RAM Footprint:** Less than 200 MB, typical for 3,000 metrics/s.
-   **Fault Tolerance:** Zero data loss during network issues (local buffering + replay).
-   **Storage Efficiency:** ~0.6 bytes per sample on disk, enabling years of retention for gigabytes, not terabytes.

### Global Scale

-   **Billions of Metrics:** Processes over 4.5 billion metrics per second across all installations.
-   **100% Sampling Rate:** No statistical sampling - every data point is captured.
-   **Unlimited Metrics:** No artificial limits or pricing tiers based on volume.
-   **Proven at Scale:** Monitors infrastructures with 100,000+ nodes seamlessly.

### Enterprise-Grade Reliability

-   **No Single Point of Failure:** Fully distributed, peer-to-peer streaming architecture.
-   **Automatic Failover:** Resilient streaming connections with self-healing.
-   **Self-Managing:** Requires minimal administration.
-   **Zero-Downtime Updates:** Supports rolling updates without interrupting monitoring.

It is common for large Netdata deployments to process millions of metrics per second across a highly available, distributed fleet, all while remaining virtually invisible from an administrative overhead perspective. This operational elegance is Netdata's ultimate success.

## Real-Time Monitoring FAQ

**Q: Is Netdata truly real-time?**<br/>
A: Yes. Netdata provides 1-second granularity monitoring with a total latency of 1-2 seconds from event to visualization, making it the fastest and only true real-time monitoring solution proven at scale. This is 10-60x faster than the typical "near real-time" solutions.

**Q: Why is 1-second resolution critical compared to 10 or 30-second?**<br/>
A: Most operational anomalies have a duration of 2-10 seconds. With 30-second monitoring, you are blind to over 90% of incidents. With 10-second monitoring, you still miss roughly 50%. Netdata's 1-second monitoring captures the full spectrum of system behavior.

**Q: Doesn't per-second monitoring create unsustainable overhead?**<br/>
A: No. This is a common misconception. Netdata is engineered for extreme efficiency, typically using less than 5% of a single CPU core. According to the  [University of Amsterdam study](https://www.ivanomalavolta.com/files/papers/ICSOC_2023.pdf), Netdata is the most energy-efficient tool for monitoring Docker-based systems. The study also shows Netdata excels in CPU usage, RAM usage, and execution time compared to other monitoring solutions.

**Q: How does Netdata handle network outages?**<br/>
A: Netdata Agents buffer metrics locally on disk during network partitions and automatically replay the buffered data once the connection is restored. This ensures zero data loss. Gaps only appear if the local system is too stressed to even collect data, which is itself a critical alert. Also, each Netdata Agent and Parent provide their own dashboards allowing continue troubleshooting at extreme conditions.

**Q: Can Netdata handle cloud and container environments?**<br/>
A: Yes, natively. Netdata provides automatic discovery and per-second monitoring for Kubernetes, Docker, and all major cloud platforms. It collects cgroups metrics directly from the kernel. The short lifespans of containers are perfectly aligned with Netdata's real-time model.

**Q: What is the core architectural difference between Netdata and Prometheus/Datadog?**<br/>
A: Netdata is a distributed, edge-native system designed for real-time data. Prometheus is a pull-based centralized scrapers, and Datadog is a cloud-based SaaS platform. These fundamental models impose inherent latency that Netdata's architecture avoids entirely.

**Q: Is real-time necessary for every single metric?**<br/>
A: Netdata uses intelligence, not dogma. It automatically adjusts collection frequency for slow-changing metrics (like static configuration details) or for applications that are sensitive to frequent polling, while maintaining 1-second collection for all dynamic system and application metrics.

**Q: Does querying more time-series slow down the dashboard?**<br/>
A: Rendering thousands of lines on a chart is a browser limitation, not a Netdata limitation. For these high-cardinality views, Netdata provides instant aggregated views with the ability to drill down to specific metrics in real-time.

**Q: What are the trade-offs for being real-time?**<br/>
A: The trade-off is architectural complexity. Distributing intelligence to the edge, synchronizing data in real-time without central bottlenecks, and managing a fleet implicitly rather than explicitly is a significantly harder engineering problem. We solved this so you don't have to choose between real-time visibility and operational cost.

## Summary

Real-time monitoring is not a luxury - it is a fundamental requirement for operating modern, dynamic, and complex infrastructure. The difference between 1-second and 30-second visibility is the difference between preventing outages and merely documenting them.

Netdata doesn't just claim to be real-time; it defines the category through engineering excellence:

-   **Bounded Latency:** Guaranteed under 2 seconds from event to insight.
-   **Universal 1-Second Granularity:** For all metrics, across your entire stack.
-   **100% Fidelity:** No averages, no samples, no approximations - only truth.
-   **Distributed Scale:** Provides real-time performance at any scale, from one node to one million.
-   **Production-Safe:** Engineered for efficiency that ensures it never becomes the problem it is designed to solve.

When you need to see what is actually happening in your infrastructure right now - not a smoothed-over report of what happened a minute ago - you need Netdata. This is real-time monitoring. This is The Netdata Standard.
