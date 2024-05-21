# ML models and anomaly detection

In observability, machine learning can be used to detect patterns and anomalies in large datasets, enabling users to identify potential issues before they become critical.

At Netdata through understanding what useful insights ML can provide, we created a tool that can improve troubleshooting, reduce mean time to resolution and in many cases prevent issues from escalating. That tool is called the [Anomaly Advisor](/docs/dashboards-and-charts/anomaly-advisor-tab.md), available at our [Netdata dashboard](/docs/dashboards-and-charts/README.md).

> **Note**
>
> If you want to learn how to configure ML on your nodes, check the [ML configuration documentation](/src/ml/ml-configuration.md).

## Design principles

The following are the high level design principles of Machine Learning in Netdata:

1. **Unsupervised**

   Whatever the ML models can do, they should do it by themselves, without any help or assistance from users.

2. **Real-time**

   We understand that Machine Learning will have some impact on resource utilization, especially in CPU utilization, but it shouldn't prevent Netdata from being real-time and high-fidelity.

3. **Integrated**

   Everything achieved with Machine Learning should be tightly integrated to the infrastructure exploration and troubleshooting practices we are used to.

4. **Assist, Advice, Consult**

   If we can't be sure that a decision made by Machine Learning is 100% accurate, we should use this to assist and consult users in their journey.

   In other words, we don't want to wake up someone at 3 AM, just because a model detected something.

Some of the types of anomalies Netdata detects are:

1. **Point Anomalies** or **Strange Points**: Single points that represent very big or very small values, not seen before (in some statistical sense).
2. **Contextual Anomalies** or **Strange Patterns**: Not strange points in their own, but unexpected sequences of points, given the history of the time-series.
3. **Collective Anomalies** or **Strange Multivariate Patterns**: Neither strange points nor strange patterns, but in global sense something looks off.
4. **Concept Drifts** or **Strange Trends**: A slow and steady drift to a new state.
5. **Change Point Detection** or **Strange Step**: A shift occurred and gradually a new normal is established.

### Models

Once ML is enabled, Netdata will begin training a model for each dimension. By default this model is a [k-means clustering](https://en.wikipedia.org/wiki/K-means_clustering) model trained on the most recent 4 hours of data.

Rather than just using the most recent value of each raw metric, the model works on a preprocessed [feature vector](https://en.wikipedia.org/wiki/Feature_(machine_learning)#:~:text=edges%20and%20objects.-,Feature%20vectors,-%5Bedit%5D) of recent smoothed values.

This enables the model to detect a wider range of potentially anomalous patterns in recent observations as opposed to just point-anomalies like big spikes or drops.

Unsupervised models have some noise, random false positives. To remove this noise, Netdata trains multiple machine learning models for each time-series, covering more than the last 2 days in total.

Netdata uses all of its available ML models to detect anomalies. So, all machine learning models of a time-series need to agree that a collected sample is an outlier, for it to be marked as an anomaly.

This process removes 99% of the false positives, offering reliable unsupervised anomaly detection.

The sections below will introduce you to the main concepts.

### Anomaly Bit

Once each model is trained, Netdata will begin producing an **anomaly score** at each time step for each dimension. It **represents a distance measure** to the centers of the model's trained clusters (by default each model has k=2, so two clusters exist for every model).

Anomalous data should have bigger distance from the cluster centers than points of data that are considered normal. If the anomaly score is sufficiently large, it is a sign that the recent raw values of the dimension could potentially be anomalous.

By default, the threshold is that the anomalous data's distance from the center of the cluster should be greater than the 99th percentile distance of the data used in training.

Once this threshold is passed, the anomaly bit corresponding to that dimension is set to `true` to flag it as anomalous, otherwise it would be left as `false` to signal normal data.

#### How the anomaly bit is used

In addition to the raw value of each metric, Netdata also stores the anomaly bit **that is either 100 (anomalous) or 0 (normal)**.

More importantly, this is achieved without additional storage overhead as this bit is embedded into the custom floating point number the Netdata database uses, so it does not introduce any overheads in memory or disk footprint.

The query engine of Netdata uses this bit to compute anomaly rates while it executes normal time-series queries. This eliminates to need for additional queries for anomaly rates, as all `/api/v2` time-series query include anomaly rate information.

### Anomaly Rate

Once all models have been trained, we can think of the Netdata dashboard as a big matrix/table of 0 and 100 values. If we consider this anomaly bit based representation of the state of the node, we can now detect overall node level anomalies.

This figure illustrates the main idea (the x axis represents dimensions and the y axis time):

|         | d1      | d2      | d3      | d4      | d5      | **NAR**               |
|---------|---------|---------|---------|---------|---------|-----------------------|
| t1      | 0       | 0       | 0       | 0       | 0       | **0%**                |
| t2      | 0       | 0       | 0       | 0       | 100     | **20%**               |
| t3      | 0       | 0       | 0       | 0       | 0       | **0%**                |
| t4      | 0       | 100     | 0       | 0       | 0       | **20%**               |
| t5      | 100     | 0       | 0       | 0       | 0       | **20%**               |
| t6      | 0       | 100     | 100     | 0       | 100     | **60%**               |
| t7      | 0       | 100     | 0       | 100     | 0       | **40%**               |
| t8      | 0       | 0       | 0       | 0       | 100     | **20%**               |
| t9      | 0       | 0       | 100     | 100     | 0       | **40%**               |
| t10     | 0       | 0       | 0       | 0       | 0       | **0%**                |
| **DAR** | **10%** | **30%** | **20%** | **20%** | **30%** | **_NAR_t1-10 = 22%_** |

- DAR = Dimension Anomaly Rate
- NAR = Node Anomaly Rate
- NAR_t1-t10 = Node Anomaly Rate over t1 to t10

To calculate an anomaly rate, we can take the average of a row or a column in any direction.

For example, if we were to average along one row then this would be the Node Anomaly Rate, NAR (for all dimensions) at time `t`.

Likewise if we averaged a column then we would have the dimension anomaly rate for each dimension over the time window `t = 1-10`. Extending this idea, we can work out an overall anomaly rate for the whole matrix or any subset of it we might be interested in.

### Anomaly detector, node level anomaly events

An anomaly detector looks at all the anomaly bits of a node. Netdata's anomaly detector produces an anomaly event when the percentage of anomaly bits is high enough for a persistent amount of time.

This anomaly event signals that there was sufficient evidence among all the anomaly bits that some strange behavior might have been detected in a more global sense across the node.

Essentially if the Node Anomaly Rate (NAR) passes a defined threshold and stays above that threshold for a persistent amount of time, a node anomaly event will be triggered.

These anomaly events are currently exposed via the `new_anomaly_event` dimension on the `anomaly_detection.anomaly_detection` chart.

## Charts

Once enabled, the "Anomaly Detection" menu and charts will be available on the dashboard.

- `anomaly_detection.dimensions`: Total count of dimensions considered anomalous or normal.
- `anomaly_detection.anomaly_rate`: Percentage of anomalous dimensions.
- `anomaly_detection.anomaly_detection`: Flags (0 or 1) to show when an anomaly event has been triggered by the detector.
