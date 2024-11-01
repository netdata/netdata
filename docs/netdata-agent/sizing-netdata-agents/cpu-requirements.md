# CPU

Netdata utilizes CPU based on what features are enabled, read more at our [resource utilization](/docs/netdata-agent/sizing-netdata-agents/README.md) page.

## Children

On Children Agents, where Netdata is running with default settings, CPU utilization should usually be about 1% to 5% of a single CPU core.

This includes 3 database tiers, Machine Learning, per-second data collection, Alerts, and [Streaming to a Parent Agent](/docs/observability-centralization-points/metrics-centralization-points/README.md).

## Parents

On Metrics Centralization Points -we call them Parent Agents- running on modern server hardware, we estimate CPU utilization per million of samples collected per second:

|       Feature        |                     Depends On                      |                       Expected Utilization                       |                               Key Reasons                                |
|:--------------------:|:---------------------------------------------------:|:----------------------------------------------------------------:|:------------------------------------------------------------------------:|
|    Metrics Ingest    |        Number of samples received per second        |          2 CPU cores per million of samples per second           |         Decompress and decode received messages, update database         |
| Metrics re-streaming |         Number of samples resent per second         |          2 CPU cores per million of samples per second           |       Encode and compress messages towards another Parent        |
|   Machine Learning   | Number of unique time-series concurrently collected | 2 CPU cores per million of unique metrics concurrently collected | Train machine learning models, query existing models to detect anomalies |

We recommend keeping the total CPU utilization below 60% when a Parent is steadily ingesting metrics, training machine learning models and running health checks. This will leave enough available CPU resources for queries.

## Increased CPU consumption on Parent startup

When a Parent starts, Children connect to it. There are several operations that temporarily affect CPU utilization, network bandwidth and disk I/O.

The general flow looks like this:

1. **Back-filling of higher tiers**: This means calculating the aggregates of the last hour of `tier2` and of the last minute of `tier1`, ensuring that higher tiers reflect all the information `tier0` has. If Netdata was stopped abnormally (e.g. due to a system failure or crash), higher tiers may have to be back-filled for longer durations.
2. **Metadata synchronization**: The metadata of all metrics that each Child maintains are negotiated between the Child and the Parent and are synchronized.
3. **Replication**: If the Parent is missing samples the Child has, they are transferred to the Parent before transferring new samples.
4. Only then the normal **streaming of new metric samples** starts.
5. At the same time, **Machine Learning** initializes, loads saved trained models and prepares Anomaly Detection.
6. After a few moments the **Health engine starts checking metrics** for triggering Alerts.

The above process is per metric.

At the same time:

- The compression algorithm learns the patterns of the data exchanged and optimizes its dictionaries for optimal compression and CPU utilization
- The database engine adjusts the page size of each metric, so that samples are committed to disk as evenly as possible across time.
