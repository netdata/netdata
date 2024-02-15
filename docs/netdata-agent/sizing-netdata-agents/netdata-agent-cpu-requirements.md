# CPU Requirements

Netdata's CPU consumption is affected by the following factors:

1. The number of metrics collected
2. The frequency metrics are collected
3. Machine Learning
4. Streaming compression (streaming of metrics to Netdata Parents)
5. Database Mode

## On Production Systems, Netdata Children

On production systems, where Netdata is running with default settings, monitoring the system it is installed at and its containers and applications, CPU utilization should usually be about 1% to 5% of a single CPU core.

This includes 3 database tiers, machine learning, per-second data collection, alerts, and streaming to a Netdata Parent.

## On Metrics Centralization Points, Netdata Parents

On Metrics Centralization Points, Netdata Parents running on modern server hardware, we **estimate CPU utilization per million of samples collected per second**:

|      Feature      |                     Depends On                      |                       Expected Utilization                       |                                Key Reasons                                |
|:-----------------:|:---------------------------------------------------:|:----------------------------------------------------------------:|:-------------------------------------------------------------------------:|
| Metrics Ingestion |        Number of samples received per second        |          2 CPU cores per million of samples per second           |         Decompress and decode received messages, update database.         |
| Metrics re-streaming|         Number of samples resent per second         |          2 CPU cores per million of samples per second           |           Encode and compress messages towards Netdata Parent.            |
| Machine Learning  | Number of unique time-series concurrently collected | 2 CPU cores per million of unique metrics concurrently collected | Train machine learning models, query existing models to detect anomalies. |

We recommend keeping the total CPU utilization below 60% when a Netdata Parent is steadily ingesting metrics, training machine learning models and running health checks. This will leave enough CPU resources available for queries.

## I want to minimize CPU utilization. What should I do?

You can control Netdata's CPU utilization with these parameters:

1. **Data collection frequency**: Going from per-second metrics to every-2-seconds metrics will half the CPU utilization of Netdata.
2. **Number of metrics collected**: Netdata by default collects every metric available on the systems it runs. Review the metrics collected and disable data collection plugins and modules not needed.
3. **Machine Learning**: Disable machine learning to save CPU cycles.
4. **Number of database tiers**: Netdata updates database tiers in parallel, during data collection. This affects both CPU utilization and memory requirements.
5. **Database Mode**: The default database mode is `dbengine`, which compresses and commits data to disk. If you have a Netdata Parent where metrics are aggregated and saved to disk and there is a reliable connection between the Netdata you want to optimize and its Parent, switch to database mode `ram` or `alloc`. This disables saving to disk, so your Netdata will also not use any disk I/O.  

## I see increased CPU consumption when a busy Netdata Parent starts, why?

When a Netdata Parent starts and Netdata children get connected to it, there are several operations that temporarily affect CPU utilization, network bandwidth and disk I/O.

The general flow looks like this:

1. **Back-filling of higher tiers**: Usually this means calculating the aggregates of the last hour of `tier2` and of the last minute of `tier1`, ensuring that higher tiers reflect all the information `tier0` has. If Netdata was stopped abnormally (e.g. due to a system failure or crash), higher tiers may have to be back-filled for longer durations.
2. **Metadata synchronization**: The metadata of all metrics each Netdata Child maintains are negotiated between the Child and the Parent and are synchronized.
3. **Replication**: If the Parent is missing samples the Child has, these samples are transferred to the Parent before transferring new samples.
4. Once all these finish, the normal **streaming of new metric samples** starts.
5. At the same time, **machine learning** initializes, loads saved trained models and prepares anomaly detection.
6. After a few moments the **health engine starts checking metrics** for triggering alerts.

The above process is per metric. So, while one metric back-fills, another replicates and a third one streams.

At the same time:

- the compression algorithm learns the patterns of the data exchanged and optimizes its dictionaries for optimal compression and CPU utilization,
- the database engine adjusts the page size of each metric, so that samples are committed to disk as evenly as possible across time.

So, when looking for the "steady CPU consumption during ingestion" of a busy Netdata Parent, we recommend to let it stabilize for a few hours before checking.

Keep in mind that Netdata has been designed so that even if during the initialization phase and the connection of hundreds of Netdata Children the system lacks CPU resources, the Netdata Parent will complete all the operations and eventually enter a steady CPU consumption during ingestion, without affecting the quality of the metrics stored. So, it is ok if during initialization of a busy Netdata Parent, CPU consumption spikes to 100%.

Important: the above initialization process is not such intense when new nodes get connected to a Netdata Parent for the first time (e.g. ephemeral nodes), since several of the steps involved are not required.

Especially for the cases where children disconnect and reconnect to the Parent due to network related issues (i.e. both the Netdata Child and the Netdata Parent have not been restarted and less than 1 hour has passed since the last disconnection), the re-negotiation phase is minimal and metrics are instantly entering the normal streaming phase.
