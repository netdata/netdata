# Anomaly Advisor

The Anomaly Advisor (the "Anomalies" tab on Netdata dashboards) is a troubleshooting assistant that correlates anomalies across your entire infrastructure and presents them as a ranked list of metrics, sorted by anomaly severity.

Built on three components: per-metric anomaly detection using k-means clustering (18 models per metric), pre-computed Node Anomaly Rate (NAR) correlation charts, and a specialized scoring engine that evaluates thousands of metrics simultaneously. When you highlight an incident timeframe, the scoring engine analyzes all metrics and returns an ordered list - typically placing root causes within the top 30-50 results.

The system works by detecting anomaly clusters. A single event (like an SSH login, a backup job, or a service restart) typically triggers anomalies across dozens of related metrics - CPU, memory, network, disk I/O, and application-specific counters. The Anomaly Advisor captures these correlations and ranks them by severity, effectively showing you what changed and how those changes cascaded through your infrastructure.

This approach inverts traditional troubleshooting. Instead of forming hypotheses and validating them one by one, you start with data - a ranked list of what actually deviated from normal patterns. The tool works without requiring system-specific knowledge, though interpreting results still requires engineering expertise.

Limitations: Works best for sudden changes and patterns not seen in the last 54 hours. Cannot detect anomalies in stopped services (no data = no anomalies). May miss gradually evolving issues where each increment appears normal.

## System Characteristics

| Aspect | Implementation | Operational Benefit |
|--------|----------------|-------------------|
| **Data Source** | Per-metric anomaly detection using k-means (k=2) with 18-model consensus | Comprehensive coverage, no blind spots |
| **Correlation Engine** | Pre-computed Node Anomaly Rate (NAR) charts updated in real-time | Instant blast radius visualization |
| **Query Engine** | Specialized scoring engine evaluating thousands of metrics simultaneously | Returns ranked list, not time-series data |
| **Ranking Algorithm** | Anomaly severity scoring across selected time window | Root cause typically in top 30-50 results |
| **Infrastructure View** | Dual charts: % anomalous and absolute count per node | Distinguishes small node spikes from large node issues |
| **Time to Insight** | Highlight timeframe → ranked results in seconds |Minutes to root cause vs hours of hypothesis testing |
| **Expertise Required** | No system-specific knowledge needed to identify anomalies | Minimal expertise to interpret results |
| **Dependency Discovery** | Correlated anomalies reveal component relationships | Exposes hidden infrastructure dependencies |
| **Best Use Cases** | Sudden changes, cascading failures, multi-node incidents | Excellent for "what just happened?" scenarios |
| **Limitations** | Cannot detect stopped services, gradual degradation | Not a replacement for all monitoring |

:::warning Limitations

- **Stopped services**: No data = no anomalies detected
- **Gradual degradation**: Changes within 54-hour training window may appear normal
- **Pattern fragments**: If anomaly patterns existed separately in training data, consensus may not trigger
:::

## Visualizing Cascading Infrastructure Level Effects

The Anomaly Advisor provides two views of node-level anomalies, revealing how issues propagate across infrastructure. Each node-level chart aggregates the underlying anomaly bits calculated per metric (as described in [Machine Learning Anomaly Detection](/docs/ml-ai/ml-anomaly-detection/ml-anomaly-detection.md)):

![Anomaly cascading effects visualization](https://github.com/user-attachments/assets/a69b1461-c559-4b22-bb02-045670d84168)

This visualization shows two distinct anomaly clusters:

**First cluster (left):**

- Shows clear propagation: one node spikes first, followed by three more nodes in sequence
- Each subsequent node shows anomalies shortly after the previous one
- Classic cascading pattern where an issue on one node impacts dependent nodes

**Second cluster (right):**

- Multiple nodes become anomalous simultaneously
- The final node shows the largest spike (200+ anomalous metrics in absolute count)
- Indicates either a shared resource issue or the final node being the aggregation point

The two charts provide different perspectives:

1. **Top chart** - Percentage of anomalous metrics per node (spikes up to 10%)
2. **Bottom chart** - Absolute count of anomalous metrics per node (spikes up to 200+)

This dual view helps distinguish between:

- **Small nodes with high anomaly rates** (high percentage, low count)
- **Large nodes with many anomalies** (lower percentage, high count)

**This visualization provides the infrastructure-level blast radius of any incident.** At a glance, you can see:

- Which nodes were affected
- When each node was impacted
- The severity of impact on each node
- Whether the issue propagated sequentially or hit multiple nodes simultaneously

By highlighting any spike or cluster (click and drag on the timeline), the scoring engine analyzes all metrics from all affected nodes during that period. The engine ranks metrics by their anomaly severity within the selected timeframe, returning an ordered list that typically reveals the root cause within the top 30-50 results.

## How to Use It

1. **Click the Anomalies tab** in any dashboard
2. **Highlight the incident time window** (click and drag on any chart)
3. **Review the ranked list** of anomalous metrics
4. **Root cause usually surfaces** in top 30-50 metrics

## Learn More

For detailed information about using the Anomalies tab, see:

- [Anomalies Tab Documentation](/docs/dashboards-and-charts/anomaly-advisor-tab.md)
- [Machine Learning Anomaly Detection](/docs/ml-ai/ml-anomaly-detection/ml-anomaly-detection.md) - The foundation powering the Anomaly Advisor
