# CPU Utilization

Netdata's CPU usage depends on the features you enable. For details, see [resource utilization](/docs/netdata-agent/sizing-netdata-agents/README.md).

## Children

With default settings on Children, CPU utilization typically falls within the range of 1% to 5% of a single core. This includes the combined resource usage of:

- Three Database Tiers for storage
- ML for Anomaly Detection
- Per-second data collection
- Alerts
- Streaming to a [Parent Agent](/docs/observability-centralization-points/metrics-centralization-points/README.md)

## Parents

For Parents, we estimate the following CPU utilization:

|       Feature        |                     Depends On                      | Expected Utilization (CPU cores per million) |                               Key Reasons                                |
|:--------------------:|:---------------------------------------------------:|:--------------------------------------------:|:------------------------------------------------------------------------:|
|    Metrics Ingest    |        Number of samples received per second        |                      2                       |         Decompress and decode received messages, update database         |
| Metrics re-streaming |         Number of samples resent per second         |                      2                       |           Encode and compress messages towards another Parent            |
|   Machine Learning   | Number of unique time-series concurrently collected |                      2                       | Train machine learning models, query existing models to detect anomalies |

To ensure optimal performance, keep total CPU utilization below 60% when the Parent is actively processing metrics, training models, and running health checks.

## Increased CPU consumption on Parent startup

When a Parent starts up, it undergoes a series of initialization tasks that can temporarily increase CPU, network, and disk I/O usage:

1. **Backfilling Higher Tiers**: The Parent calculates aggregated metrics for missing data points, ensuring consistency across different time resolutions.
2. **Metadata Synchronization**: The Parent and Children exchange metadata information about collected metrics.
3. **Data Replication**: Missing data is transferred from Children to the Parent.
4. **Normal Streaming**: Regular streaming of new metrics begins.
5. **Machine Learning Initialization**: ML models are loaded and prepared for Anomaly Detection.
6. **Health Check Initialization**: The health engine starts monitoring metrics and triggering alerts.

Additional considerations:

- **Compression Optimization**: The compression algorithm learns data patterns to optimize compression ratios.
- **Database Optimization**: The Database engine adjusts page sizes for efficient disk I/O.

These initial tasks can temporarily increase resource usage, but the impact typically diminishes as the Parent stabilizes and enters a steady-state operation.
