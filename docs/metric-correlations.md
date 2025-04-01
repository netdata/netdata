# Metric Correlations

The Metric Correlations feature helps you quickly identify metrics and charts relevant to a specific time window of interest, allowing for faster root cause analysis.

By filtering the standard Netdata dashboard to display only the most relevant charts, Metric Correlations makes it easier to pinpoint anomalies and investigate issues.

Since it leverages every available metric in your infrastructure with up to 1-second granularity, Metric Correlations provides highly accurate insights.

## Using Metric Correlations

When viewing the [Metrics tab or a single-node dashboard](/docs/dashboards-and-charts/metrics-tab-and-single-node-tabs.md), you'll find the **Metric Correlations** button in the top-right corner.

To start:

1. Click **Metric Correlations**.
2. [Highlight a selection of metrics](/docs/dashboards-and-charts/netdata-charts.md#highlight) on a single chart. The selected timeframe must be at least 15 seconds.
3. The menu displays details about the selected area and reference baseline. Metric Correlations compares the highlighted window to a reference baseline, which is four times its length and precedes it immediately.
4. Click **Find Correlations**. This button is only active if a valid timeframe is selected.
5. The process evaluates all available metrics and returns a filtered Netdata dashboard showing only the most changed metrics between the baseline and the highlighted window.
6. If needed, select another window and press **Find Correlations** again to refine your analysis.

## Metric Correlations Options

Metric Correlations offers adjustable parameters for deeper data exploration. Since different data types and incidents require different approaches, these settings allow for flexible analysis.

### Method

Two algorithms are available for scoring metrics based on changes between the baseline and highlight windows:

- **`KS2` (Kolmogorov-Smirnov Test)**: A statistical method comparing distributions between the highlighted and baseline windows to detect significant changes. [Implementation details](https://github.com/netdata/netdata/blob/d917f9831c0a1638ef4a56580f321eb6c9a88037/database/metric_correlations.c#L212).
- **`Volume`**: A heuristic approach based on percentage change in averages, designed to handle edge cases. [Implementation details](https://github.com/netdata/netdata/blob/d917f9831c0a1638ef4a56580f321eb6c9a88037/database/metric_correlations.c#L516).

### Aggregation

To accommodate different window lengths, Netdata aggregates raw data as needed. The default aggregation method is `Average`, but you can also choose `Median`, `Min`, `Max`, or `Stddev`.

### Data Type

Netdata assigns an [Anomaly Bit](https://github.com/netdata/netdata/tree/master/src/ml#anomaly-bit) to each metric in real-time, flagging whether it deviates significantly from normal behavior. You can analyze either raw data or anomaly rates:

- **`Metrics`**: Runs Metric Correlations on raw metric values.
- **`Anomaly Rate`**: Runs Metric Correlations on anomaly rates for each metric.

## Metric Correlations on the Agent

Metric Correlations (MC) requests to Netdata Cloud are handled in two ways:

1. If MC is enabled on any node, the request is routed to the highest-level node (a Parent node or the node itself).
2. If MC is not enabled on any node, Netdata Cloud processes the request by collecting data from nodes and computing correlations on its backend.

## Usage Tips

- When running Metric Correlations from the [Metrics tab](/docs/dashboards-and-charts/metrics-tab-and-single-node-tabs.md) across multiple nodes, refine your results by grouping by node:
  1. Run MC on all nodes if you're unsure which ones are relevant.
  2. Group the most interesting charts by node to determine whether changes affect all nodes or just a subset.
  3. If a subset of nodes stands out, filter for those nodes and rerun MC to get more precise results.

- Choose the **`Volume`** algorithm for sparse metrics (e.g., request latency with few requests). Otherwise, use **`KS2`**.
  - **`KS2`** is ideal for detecting complex distribution changes, such as shifts in variance.
  - **`Volume`** is better for detecting metrics that were inactive and then spiked (or vice versa).

  **Example:**
  - `Volume` can highlight network traffic suddenly turning on.
  - `KS2` can detect entropy distribution changes missed by `Volume`.

- Combine **`Volume`** and **`Anomaly Rate`** to identify the most anomalous metrics within a timeframe. Expand the anomaly rate chart to visualize results more clearly.