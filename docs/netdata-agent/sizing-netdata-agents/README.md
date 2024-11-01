# Sizing Netdata Agents

Netdata is designed to automatically adjust its resource consumption based on the specific workload.

This table shows the specific system resources affected by different Netdata features:

|                 Feature | CPU | RAM | Disk I/O | Disk Space | Network Traffic |
|------------------------:|:---:|:---:|:--------:|:----------:|:---------------:|
|       Collected metrics |  ✓  |  ✓  |    ✓     |     ✓      |        -        |
|        Sample frequency |  ✓  |  -  |    ✓     |     ✓      |        -        |
| Database mode and tiers |  -  |  ✓  |    ✓     |     ✓      |        -        |
|        Machine learning |  ✓  |  ✓  |    -     |     -      |        -        |
|               Streaming |  ✓  |  ✓  |    -     |     -      |        ✓        |

1. **Collected metrics**

    - **Impact**: More metrics mean higher CPU, RAM, disk I/O, and disk space usage.
    - **Optimization**: To reduce resource consumption, consider lowering the number of collected metrics by disabling unnecessary data collectors.

2. **Sample frequency**

    - **Impact**: Netdata collects most metrics with 1-second granularity. This high frequency impacts CPU usage.
    - **Optimization**: Lowering the sampling frequency (e.g., 1-second to 2-second intervals) can halve CPU usage. Balance the need for detailed data with resource efficiency.

3. **Database Mode and Tiers**

    - **Impact**: The number of database tiers directly affects memory consumption. More tiers mean higher memory usage.
    - **Optimization**: The default number of tiers is 3. Choose the appropriate number of tiers based on data retention requirements.

4. **Machine Learning**

    - **Impact**: Machine learning model training is CPU-intensive, affecting overall CPU usage.
    - **Optimization**: Consider disabling machine learning for less critical metrics or adjusting model training frequency.

5. **Streaming Compression**

    - **Impact**: Compression algorithm choice affects CPU usage and network traffic.
    - **Optimization**: Select an algorithm that balances CPU efficiency with network bandwidth requirements (e.g., zstd for a good balance).

## Minimizing the resources used by Netdata Agents

To optimize resource utilization, consider using a **Netdata Parent-Child** setup.

This approach involves centralizing the collection and processing of metrics on Netdata Parent nodes while running lightweight Netdata Child Agents on edge devices.

## Maximizing the scale of Netdata Parents

Netdata Parents dynamically adjust their resource usage based on the volume of metrics received. However, for optimal query performance, you may need to dedicate more RAM.

Check [RAM Requirements](/docs/netdata-agent/sizing-netdata-agents/ram-requirements.md) for more information.

## Innovations Netdata has for optimal performance and scalability

The following are some of the innovations the open-source Netdata Agent has, that contribute to its excellent performance, and scalability.

1. **Minimal disk I/O**

   When Netdata saves data on-disk, it stores them at their final place, eliminating the need to reorganize this data.

   Netdata is organizing its data structures in such a way that samples are committed on disk as evenly as possible across time, without affecting its memory requirements.

   Furthermore, Netdata Agents use direct I/O for saving and loading metric samples. This prevents Netdata from polluting system caches with metric data. Netdata maintains its own caches for this data.

2. **4 bytes per sample uncompressed**

   To achieve optimal memory and disk footprint, Netdata uses a custom 32-bit floating point number. This floating point number is used to store the collected samples, together with their anomaly bit.

   The database of Netdata is fixed-step, so it has predefined slots for every sample, allowing Netdata to store timestamps once every several hundreds of samples, minimizing both its memory requirements and the disk footprint.

   The final disk footprint of Netdata varies due to compression efficiency. It is usually about 0.6 bytes per sample for the high-resolution tier (per-second), 6 bytes per sample for the mid-resolution tier (per-minute) and 18 bytes per sample for the low-resolution tier (per-hour).

3. **Query priorities**

   Alerting, Machine Learning, Streaming and Replication rely on metric queries.

   When multiple queries are running in parallel, Netdata assigns priorities to all of them, favoring interactive queries over background tasks.

   This means that queries do not compete equally for resources. Machine learning or replication may slow down when interactive queries are running and the system starves for resources.

4. **A pointer per label**

   Apart from metric samples, metric labels and their cardinality is the biggest memory consumer, especially in highly ephemeral environments, like kubernetes.

   Netdata uses a single pointer for any label key-value pair that is reused. Keys and values are also deduplicated, providing the best possible memory footprint for metric labels.

5. **Streaming Protocol**

   The streaming protocol of Netdata allows minimizing the resources consumed on production systems by delegating features of to other Netdata Agents (Parents), without compromising monitoring fidelity or responsiveness, enabling the creation of a highly distributed observability platform.
