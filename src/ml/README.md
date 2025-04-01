# Machine Learning Models and Anomaly Detection in Netdata

## Overview

Machine learning helps detect patterns and anomalies in large datasets, enabling early issue identification before they escalate.

At Netdata, we developed **Anomaly Advisor**, a tool designed to improve troubleshooting, reduce mean time to resolution, and prevent issues from escalating. You can access it through the [Netdata dashboard](/docs/dashboards-and-charts/README.md).

> **Note**
>
> To configure ML on your nodes, check the [ML configuration documentation](/src/ml/ml-configuration.md).

---

## Design Principles

Netdata’s machine learning models follow these key principles:

1. **Unsupervised Learning**
   - Models operate independently without requiring user input.

2. **Real-time Performance**
   - While ML impacts CPU usage, it won't compromise Netdata’s high-fidelity, real-time monitoring.

3. **Seamless Integration**
   - ML-based insights are fully embedded into Netdata’s existing infrastructure monitoring and troubleshooting.

4. **Assistance Over Alerts**
   - ML helps users investigate potential issues rather than triggering unnecessary alerts. It won’t wake you up at 3 AM for minor anomalies.

---

## Types of Anomalies Detected

Netdata identifies several anomaly types:

- **Point Anomalies**: Unusually high or low values compared to historical data.
- **Contextual Anomalies**: Sequences of values that deviate from expected patterns.
- **Collective Anomalies**: Multivariate anomalies where a combination of metrics appears off.
- **Concept Drifts**: Gradual shifts leading to a new baseline.
- **Change Points**: Sudden shifts resulting in a new normal state.

---

## How Netdata’s ML Models Work

### Training & Detection

Once ML is enabled, Netdata trains an unsupervised model for each metric. By default, this model is a [k-means clustering](https://en.wikipedia.org/wiki/K-means_clustering) algorithm trained on the last 4 hours of data. Instead of just analyzing raw values, the model works with preprocessed feature vectors to improve detection accuracy.

To reduce false positives, Netdata trains multiple models per time-series, covering over two days of data. An anomaly is flagged only if **all** models agree on it, eliminating 99% of false positives.

### Anomaly Bit

Each trained model assigns an **anomaly score** at every time step based on how far the data deviates from learned clusters. If the score exceeds the 99th percentile of training data, the **anomaly bit** is set to `true` (100); otherwise, it remains `false` (0).

**Key benefits:**
- No additional storage overhead since the anomaly bit is embedded in Netdata’s floating point number format.
- The query engine automatically computes anomaly rates without requiring extra queries.

### Anomaly Rate

Netdata calculates **Node Anomaly Rate (NAR)** and **Dimension Anomaly Rate (DAR)** based on anomaly bits. Here’s an example matrix:

| Time | d1  | d2  | d3  | d4  | d5  | **NAR** |
|------|-----|-----|-----|-----|-----|---------|
| t1   | 0   | 0   | 0   | 0   | 0   | **0%**  |
| t2   | 0   | 0   | 0   | 0   | 100 | **20%** |
| t3   | 0   | 0   | 0   | 0   | 0   | **0%**  |
| t4   | 0   | 100 | 0   | 0   | 0   | **20%** |
| t5   | 100 | 0   | 0   | 0   | 0   | **20%** |
| t6   | 0   | 100 | 100 | 0   | 100 | **60%** |
| t7   | 0   | 100 | 0   | 100 | 0   | **40%** |
| t8   | 0   | 0   | 0   | 0   | 100 | **20%** |
| t9   | 0   | 0   | 100 | 100 | 0   | **40%** |
| t10  | 0   | 0   | 0   | 0   | 0   | **0%**  |
| **DAR** | **10%** | **30%** | **20%** | **20%** | **30%** | **_NAR_t1-10 = 22%_** |

- **DAR (Dimension Anomaly Rate):** Average anomalies for a specific metric over time.
- **NAR (Node Anomaly Rate):** Average anomalies across all metrics at a given time.
- **Overall anomaly rate:** Computed across the entire dataset for deeper insights.

### Node-Level Anomaly Detection

Netdata tracks the percentage of anomaly bits over time. When the **Node Anomaly Rate (NAR)** exceeds a set threshold and remains high for a period, a **node anomaly event** is triggered. These events are recorded in the `new_anomaly_event` dimension on the `anomaly_detection.anomaly_detection` chart.

---

## Viewing Anomaly Data in Netdata

Once ML is enabled, Netdata provides an **Anomaly Detection** menu with key charts:

- **`anomaly_detection.dimensions`**: Number of dimensions flagged as anomalous.
- **`anomaly_detection.anomaly_rate`**: Percentage of anomalous dimensions.
- **`anomaly_detection.anomaly_detection`**: Flags (0 or 1) indicating when an anomaly event occurs.

These insights help you quickly assess potential issues and take action before they escalate.

---

## Summary

Netdata’s machine learning models provide reliable, real-time anomaly detection with minimal false positives. By embedding ML within existing observability workflows, Netdata enhances troubleshooting and ensures proactive monitoring without unnecessary alerts.

For more details, check out:
- [Anomaly Advisor](/docs/dashboards-and-charts/anomaly-advisor-tab.md)
- [ML Configuration Guide](/src/ml/ml-configuration.md)